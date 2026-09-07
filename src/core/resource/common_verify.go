/*
Copyright 2019 The Cloud-Barista Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resource

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloud-barista/cb-tumblebug/src/core/common"
	"github.com/cloud-barista/cb-tumblebug/src/core/common/apierr"
	clientManager "github.com/cloud-barista/cb-tumblebug/src/core/common/client"
	"github.com/cloud-barista/cb-tumblebug/src/core/model"
	"github.com/cloud-barista/cb-tumblebug/src/kvstore/kvstore"
	"github.com/rs/zerolog/log"
	"github.com/tidwall/gjson"
)

// sharedResourceVerifyTTL bounds CSP API load: a verified-alive resource is not re-checked until expiry.
const sharedResourceVerifyTTL = 5 * time.Minute

// verifiedAliveCache maps "ns/type/id" -> last successful verification time (positive results only).
var verifiedAliveCache sync.Map

// InvalidateVerifyCache drops the cached verification for a resource.
func InvalidateVerifyCache(nsId, resType, resourceId string) {
	verifiedAliveCache.Delete(nsId + "/" + resType + "/" + resourceId)
}

func isCspNotFoundMsg(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "does not exist") ||
		strings.Contains(m, "not found") ||
		strings.Contains(m, "notfound")
}

// VerifySharedResourceOnCsp checks via Spider that a recorded resource still exists on the CSP.
// Returns (exists, indeterminate): (false, nil) means definitive drift; a non-nil error means
// the check could not conclude and must not be treated as drift.
func VerifySharedResourceOnCsp(nsId string, resType string, resourceId string) (bool, error) {
	cacheKey := nsId + "/" + resType + "/" + resourceId
	if v, ok := verifiedAliveCache.Load(cacheKey); ok {
		if t, ok := v.(time.Time); ok && time.Since(t) < sharedResourceVerifyTTL {
			return true, nil
		}
		verifiedAliveCache.Delete(cacheKey)
	}

	key := common.GenResourceKey(nsId, resType, resourceId)
	keyValue, exists, err := kvstore.GetKv(key)
	if err != nil || !exists {
		return false, fmt.Errorf("cannot read resource record %s: %v", key, err)
	}

	var cspResourceName, connectionName, spiderPath string
	switch resType {
	case model.StrSSHKey:
		temp := model.SshKeyInfo{}
		if err := json.Unmarshal([]byte(keyValue.Value), &temp); err != nil {
			return false, err
		}
		cspResourceName, connectionName, spiderPath = temp.CspResourceName, temp.ConnectionName, "/keypair/"
	case model.StrVNet:
		temp := model.VNetInfo{}
		if err := json.Unmarshal([]byte(keyValue.Value), &temp); err != nil {
			return false, err
		}
		cspResourceName, connectionName, spiderPath = temp.CspResourceName, temp.ConnectionName, "/vpc/"
	case model.StrSecurityGroup:
		temp := model.SecurityGroupInfo{}
		if err := json.Unmarshal([]byte(keyValue.Value), &temp); err != nil {
			return false, err
		}
		cspResourceName, connectionName, spiderPath = temp.CspResourceName, temp.ConnectionName, "/securitygroup/"
	default:
		return false, fmt.Errorf("unsupported resource type for CSP verification: %s", resType)
	}
	if cspResourceName == "" {
		return false, nil
	}

	type connReq struct {
		ConnectionName string
	}
	requestBody := connReq{ConnectionName: connectionName}
	var callResult interface{}
	client := clientManager.NewHttpClient()
	client.SetTimeout(60 * time.Second)

	_, err = clientManager.ExecuteHttpRequest(
		client,
		"GET",
		model.SpiderRestUrl+spiderPath+cspResourceName,
		nil,
		clientManager.SetUseBody(requestBody),
		&requestBody,
		&callResult,
		clientManager.ShortDuration,
	)
	if err == nil {
		verifiedAliveCache.Store(cacheKey, time.Now())
		return true, nil
	}
	if isCspNotFoundMsg(err.Error()) {
		log.Warn().Msgf("%s '%s' (csp: %s) recorded in Tumblebug but missing behind it: %v",
			resType, resourceId, cspResourceName, err)
		return false, nil
	}
	return false, err
}


// spiderAllListPath maps a type to Spider's CSP-enumeration endpoint
var spiderAllListPath = map[string]string{
	model.StrDataDisk:      "/alldisk",
	model.StrSSHKey:        "/allkeypair",
	model.StrSecurityGroup: "/allsecuritygroup",
	model.StrCustomImage:   "/allmyimage",
	model.StrNLB:           "/allnlb",
	model.StrVNet:          "/allvpc",
	model.StrObjectStorage: "/alls3",
	model.StrRDBMS:         "/allrdbms",
}

// ResourcePresentOnCsp reports whether the resource still exists on the CSP, via
// Spider's /all* enumeration. Matches SystemId or NameId so provider ID quirks read
// as "present"; absence must be unambiguous before a delete purges the local record.
func ResourcePresentOnCsp(connName, resourceType, cspResourceId, cspResourceName string) (bool, error) {
	// A purge decision must never act on a response cached from before the DELETE
	clientManager.InvalidateGetCache(model.SpiderRestUrl+spiderAllListPath[resourceType],
		model.CspResourceStatusRequest{ConnectionName: connName})
	resp, err := GetCspResourceStatus(connName, resourceType)
	if err != nil {
		return false, err
	}
	match := func(list []model.SpiderNameIdSystemId) bool {
		for _, v := range list {
			if (v.SystemId != "" && v.SystemId == cspResourceId) ||
				(v.NameId != "" && v.NameId == cspResourceName) {
				return true
			}
		}
		return false
	}
	return match(resp.AllList.MappedList) || match(resp.AllList.OnlyCSPList), nil
}

// spiderRegisterPath maps a type to Spider's register-existing-resource endpoint
var spiderRegisterPath = map[string]string{
	model.StrDataDisk:      "/regdisk",
	model.StrSSHKey:        "/regkeypair",
	model.StrSecurityGroup: "/regsecuritygroup",
	model.StrCustomImage:   "/regmyimage",
}

// repairSpiderRegistration re-registers a resource whose IID mapping Spider lost so a
// retried DELETE can reach the CSP; binds to SystemId to never capture a same-named stranger
func repairSpiderRegistration(resourceType, connName, cspResourceName, cspResourceId string) error {
	if cspResourceId == "" {
		return fmt.Errorf("no stored SystemId for the %s; refusing name-only re-registration", resourceType)
	}
	path, ok := spiderRegisterPath[resourceType]
	if !ok {
		return fmt.Errorf("no Spider register endpoint for %s", resourceType)
	}
	if resourceType == model.StrDataDisk {
		requestBody := model.SpiderDiskReqInfoWrapper{
			ConnectionName: connName,
			ReqInfo:        model.SpiderDiskInfo{Name: cspResourceName, CSPid: cspResourceId},
		}
		var callResult model.SpiderDiskInfo
		client := clientManager.NewHttpClient()
		_, err := clientManager.ExecuteHttpRequest(
			client, "POST", model.SpiderRestUrl+path, nil,
			clientManager.SetUseBody(requestBody), &requestBody, &callResult,
			clientManager.MediumDuration,
		)
		return err
	}
	requestBody := struct {
		ConnectionName string
		ReqInfo        struct {
			Name  string
			CSPId string
		}
	}{ConnectionName: connName}
	requestBody.ReqInfo.Name = cspResourceName
	requestBody.ReqInfo.CSPId = cspResourceId
	var callResult any
	client := clientManager.NewHttpClient()
	_, err := clientManager.ExecuteHttpRequest(
		client, "POST", model.SpiderRestUrl+path, nil,
		clientManager.SetUseBody(requestBody), &requestBody, &callResult,
		clientManager.MediumDuration,
	)
	return err
}

// verifyResourceDeletedOnSpider re-checks with Spider that a resource no longer exists after a successful DELETE.
// It sends a GET request to Spider for the resource and expects an HTTP error (404 or 500 with "not found").
// If Spider still returns the resource, it logs a warning for operator investigation.
func verifyResourceDeletedOnSpider(deleteUrl string, connectionName string, resourceType string, resourceId string) error {
	// Build the GET URL from the DELETE URL (strip ?force=true query param if present)
	getUrl := strings.Split(deleteUrl, "?")[0]

	type JsonTemplate struct {
		ConnectionName string
	}
	requestBody := JsonTemplate{ConnectionName: connectionName}

	var verifyResult any
	client := clientManager.NewHttpClient()

	log.Debug().Msgf("Re-verifying deletion: GET %s (connectionName=%s)", getUrl, connectionName)

	_, err := clientManager.ExecuteHttpRequest(
		client,
		"GET",
		getUrl,
		nil,
		clientManager.SetUseBody(requestBody),
		&requestBody,
		&verifyResult,
		clientManager.VeryShortDuration,
	)

	if err != nil {
		// Expected: Spider returns HTTP 500 with "not found" or similar error → resource is indeed gone
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "not exist") || strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "does not exist") {
			log.Debug().Msgf("Re-verification confirmed: %s/%s no longer exists on Spider", resourceType, resourceId)
			return nil
		}
		// Other errors (e.g., network issue) — log but don't block
		log.Warn().Err(err).Msgf("Re-verification GET failed with unexpected error for %s/%s", resourceType, resourceId)
		return nil
	}

	// If GET succeeded (HTTP 200), the resource still exists on Spider — this is unexpected after a successful DELETE
	return fmt.Errorf("resource %s/%s still exists on Spider after DELETE reported success (re-verification GET returned HTTP 200)", resourceType, resourceId)
}

// Default parameters for post-deletion polling via Spider.
const (
	DefaultPollMaxAttempts = 5
	DefaultPollInterval    = 3 * time.Second
)

// PollResourceDeletedViaSpider polls Spider GET (query-param ConnectionName) up to maxAttempts×interval
// to confirm deletion. Handles GCP anomaly (200+Result:false → deleted) and eventual consistency.
// Returns: (true,nil) confirmed | (false,nil) still exists | (false,err) inconclusive.
func PollResourceDeletedViaSpider(getURL string, headers map[string]string, maxAttempts int, interval time.Duration) (deleted bool, verifyErr error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		reqBody := clientManager.NoBody
		var rawResult any
		client := clientManager.NewHttpClient()

		restyResp, callErr := clientManager.ExecuteHttpRequest(
			client,
			"GET",
			getURL,
			headers,
			clientManager.SetUseBody(reqBody),
			&reqBody,
			&rawResult,
			clientManager.ShortDuration,
		)
		err := clientManager.HandleHttpResponse(restyResp, callErr)

		if err != nil {
			if apierr.IsNotFound(err) {
				log.Debug().Msgf("PollResourceDeletedViaSpider: confirmed deleted on attempt %d/%d: %s", attempt, maxAttempts, getURL)
				return true, nil
			}
			// Other error (5xx, network) — inconclusive; continue polling
			log.Warn().Err(err).Msgf("PollResourceDeletedViaSpider: inconclusive error on attempt %d/%d: %s", attempt, maxAttempts, getURL)
			lastErr = err
		} else {
			// HTTP 200 — check for GCP-style Result:false (resource is actually gone)
			bodyBytes, _ := json.Marshal(rawResult)
			if result := gjson.GetBytes(bodyBytes, "Result").String(); strings.EqualFold(result, "false") {
				log.Debug().Msgf("PollResourceDeletedViaSpider: confirmed deleted (Result:false) on attempt %d/%d: %s", attempt, maxAttempts, getURL)
				return true, nil
			}
			log.Info().Msgf("PollResourceDeletedViaSpider: still visible on attempt %d/%d: %s", attempt, maxAttempts, getURL)
			lastErr = nil
		}

		if attempt < maxAttempts {
			time.Sleep(interval)
		}
	}
	return false, lastErr
}

// PollResourceDeletedOnCSP polls GetCspResourceStatus (e.g. GET /allrdbms) confirming systemId is gone from OnlyCSPList; mirrors PollResourceDeletedViaSpider's (bool, error) shape.
func PollResourceDeletedOnCSP(connConfig string, resourceType string, systemId string, maxAttempts int, interval time.Duration) (bool, error) {
	if systemId == "" {
		return true, nil
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			time.Sleep(interval)
		}
		status, err := GetCspResourceStatus(connConfig, resourceType)
		if err != nil {
			log.Warn().Err(err).Msgf("PollResourceDeletedOnCSP: inconclusive on attempt %d/%d for %s", attempt, maxAttempts, systemId)
			lastErr = err
			continue
		}
		lastErr = nil
		stillOnCSP := false
		for _, r := range status.AllList.OnlyCSPList {
			if r.SystemId == systemId {
				stillOnCSP = true
				break
			}
		}
		if !stillOnCSP {
			log.Debug().Msgf("PollResourceDeletedOnCSP: %s confirmed gone from CSP on attempt %d/%d", systemId, attempt, maxAttempts)
			return true, nil
		}
		log.Info().Msgf("PollResourceDeletedOnCSP: %s still present on CSP (OnlyCSPList), attempt %d/%d", systemId, attempt, maxAttempts)
	}
	return false, lastErr
}

// Sentinel errors PollResourceFullyDeleted wraps (match via errors.Is) so callers can tell which check never cleared.
var (
	ErrStillTrackedBySpider = errors.New("still tracked by Spider")
	ErrStillOnCSP           = errors.New("still present on CSP (OnlyCSPList)")
)

// PollResourceFullyDeleted confirms a resource is gone from both Spider's tracking and the CSP itself, wrapping ErrStillTrackedBySpider/ErrStillOnCSP if not.
func PollResourceFullyDeleted(getURL string, connConfig string, resourceType string, systemId string,
	spiderMaxAttempts int, spiderInterval time.Duration, cspMaxAttempts int, cspInterval time.Duration) (deleted bool, err error) {
	if ok, verifyErr := PollResourceDeletedViaSpider(getURL, nil, spiderMaxAttempts, spiderInterval); !ok {
		if verifyErr != nil {
			return false, fmt.Errorf("%w: %w", ErrStillTrackedBySpider, verifyErr)
		}
		return false, ErrStillTrackedBySpider
	}
	if ok, cspErr := PollResourceDeletedOnCSP(connConfig, resourceType, systemId, cspMaxAttempts, cspInterval); !ok {
		if cspErr != nil {
			return false, fmt.Errorf("%w: %w", ErrStillOnCSP, cspErr)
		}
		return false, ErrStillOnCSP
	}
	return true, nil
}

// DeregisterResource deregisters the TB Resource object from Spider and TB without deleting the actual CSP resource
// This function only removes the resource mapping from Spider and TB internal storage (kvstore, label, etc.)
// The actual CSP resource remains intact and can be re-registered later
