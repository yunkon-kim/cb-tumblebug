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
	"fmt"

	clientManager "github.com/cloud-barista/cb-tumblebug/src/core/common/client"
	"github.com/cloud-barista/cb-tumblebug/src/core/model"
	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"
)

// ResourceStatusFilter scopes GetCspResourceStatus to a specific parent resource.
type ResourceStatusFilter struct {
	// ParentResourceId is the parent's Spider NameId (e.g., VPC NameId for StrSubnet).
	// Empty means no scoping.
	ParentResourceId string
}

// GetCspResourceStatus returns Mapped/SpiderOnly/CspOnly resource lists from CB-Spider.
// Pass a ResourceStatusFilter to scope child resources to a specific parent
// (e.g., VPC NameId when querying StrSubnet).
func GetCspResourceStatus(connConfig string, resourceType string, filter ...ResourceStatusFilter) (model.CspResourceStatusResponse, error) {
	var response model.CspResourceStatusResponse

	// Initialize response with basic information
	response.ConnectionName = connConfig
	response.ResourceType = resourceType

	// Create HTTP client with connection close for efficiency
	client := clientManager.NewHttpClient()
	client.SetAllowGetMethodPayload(true)

	// Create request body
	requestBody := model.CspResourceStatusRequest{
		ConnectionName: connConfig,
	}

	// Determine Spider API endpoint based on resource type
	var spiderRequestURL string
	var isSubnetResource bool = false

	switch resourceType {
	case model.StrNLB:
		spiderRequestURL = model.SpiderRestUrl + "/allnlb"
	case model.StrNode:
		spiderRequestURL = model.SpiderRestUrl + "/allvm"
	case model.StrVNet:
		spiderRequestURL = model.SpiderRestUrl + "/allvpc"
	case model.StrSubnet:
		// Subnet requires special handling via VPC info
		spiderRequestURL = model.SpiderRestUrl + "/allvpcinfo"
		isSubnetResource = true
	case model.StrSecurityGroup:
		spiderRequestURL = model.SpiderRestUrl + "/allsecuritygroup"
	case model.StrSSHKey:
		spiderRequestURL = model.SpiderRestUrl + "/allkeypair"
	case model.StrDataDisk:
		spiderRequestURL = model.SpiderRestUrl + "/alldisk"
	case model.StrCustomImage:
		spiderRequestURL = model.SpiderRestUrl + "/allmyimage"
	case model.StrObjectStorage:
		spiderRequestURL = model.SpiderRestUrl + "/alls3"
	case model.StrRDBMS:
		spiderRequestURL = model.SpiderRestUrl + "/allrdbms"
	default:
		err := fmt.Errorf("unsupported resource type: %s", resourceType)
		response.Error = err.Error()
		return response, err
	}

	// Make HTTP request to CB-Spider
	method := "GET"
	var err error

	if isSubnetResource {
		// For Subnet, use different endpoint and query parameter
		spiderRequestURL = fmt.Sprintf("%s?ConnectionName=%s", spiderRequestURL, connConfig)
		noBody := clientManager.NoBody
		var callResult model.SpiderAllVpcInfoWrapper
		_, err = clientManager.ExecuteHttpRequest(
			client,
			method,
			spiderRequestURL,
			nil,
			clientManager.SetUseBody(noBody),
			&noBody,
			&callResult,
			clientManager.MediumDuration,
		)
		if err != nil {
			log.Error().Err(err).Str("connection", connConfig).Str("resourceType", resourceType).
				Msg("Failed to request CB-Spider for resource status")
			response.Error = fmt.Sprintf("HTTP request failed: %v", err)
			return response, fmt.Errorf("failed to request CB-Spider: %w", err)
		}

		// Extract subnet classification from VPC info.
		// When a filter is provided, only the matching VPC's subnets are returned.
		parentVpcNameId := ""
		if len(filter) > 0 {
			parentVpcNameId = filter[0].ParentResourceId
		}
		response.AllList = getCspSubnetResourceStatus(callResult, connConfig, parentVpcNameId)

		log.Trace().
			Int("mapped", len(response.AllList.MappedList)).
			Int("spiderOnly", len(response.AllList.OnlySpiderList)).
			Int("cspOnly", len(response.AllList.OnlyCSPList)).
			Msg("Extracted subnet list from VPC info")
	} else {
		// For other resources, use standard body-based request
		var callResult model.SpiderAllListWrapper
		_, err = clientManager.ExecuteHttpRequest(
			client,
			method,
			spiderRequestURL,
			nil,
			clientManager.SetUseBody(requestBody),
			&requestBody,
			&callResult,
			clientManager.MediumDuration,
		)
		if err != nil {
			log.Error().Err(err).Str("connection", connConfig).Str("resourceType", resourceType).
				Msg("Failed to request CB-Spider for resource status")
			response.Error = fmt.Sprintf("HTTP request failed: %v", err)
			return response, fmt.Errorf("failed to request CB-Spider: %w", err)
		}

		// Copy the AllList data to response
		response.AllList = callResult.AllList
	}

	// Add success message with resource counts
	mappedCount := len(response.AllList.MappedList)
	spiderOnlyCount := len(response.AllList.OnlySpiderList)
	cspOnlyCount := len(response.AllList.OnlyCSPList)

	response.SystemMessage = fmt.Sprintf("Successfully retrieved %s resources from %s: %d mapped, %d spider-only, %d csp-only",
		resourceType, connConfig, mappedCount, spiderOnlyCount, cspOnlyCount)

	log.Trace().Str("connection", connConfig).Str("resourceType", resourceType).
		Int("mapped", mappedCount).Int("spiderOnly", spiderOnlyCount).Int("cspOnly", cspOnlyCount).
		Msgf("Successfully retrieved '%s' resource status", resourceType)

	return response, nil
}

// getCspSubnetResourceStatus classifies subnets into Mapped/SpiderOnly/CspOnly using
// SystemId matching (allvpcinfo always returns NameId="" for subnets — confirmed Spider bug).
// When parentVpcNameId != "", only subnets of that Mapped VPC are classified (Phases 2/3 skipped).
func getCspSubnetResourceStatus(vpcAllInfo model.SpiderAllVpcInfoWrapper, connConfig string, parentVpcNameId string) model.SpiderAllList {
	client := clientManager.NewHttpClient()
	client.SetAllowGetMethodPayload(true)

	var mappedSubnets, spiderOnlySubnets, cspOnlySubnets []model.SpiderNameIdSystemId

	if parentVpcNameId != "" {
		// Scoped: classify subnets of the target Mapped VPC only.
		for _, vpc := range vpcAllInfo.AllListInfo.MappedInfoList {
			if vpc.IId.NameId == parentVpcNameId {
				m, s, c := classifyMappedVpcSubnets(client, vpc, connConfig)
				mappedSubnets = append(mappedSubnets, m...)
				spiderOnlySubnets = append(spiderOnlySubnets, s...)
				cspOnlySubnets = append(cspOnlySubnets, c...)
				break
			}
		}
	} else {
		// Unscoped: classify subnets across all VPCs and all phases.

		// [Phase 1] Mapped VPC subnets — classify each subnet individually.
		for _, vpc := range vpcAllInfo.AllListInfo.MappedInfoList {
			m, s, c := classifyMappedVpcSubnets(client, vpc, connConfig)
			mappedSubnets = append(mappedSubnets, m...)
			spiderOnlySubnets = append(spiderOnlySubnets, s...)
			cspOnlySubnets = append(cspOnlySubnets, c...)
		}

		// [Phase 2] SpiderOnly VPC subnets — all subnets of a SpiderOnly VPC are SpiderOnly.
		for _, vpc := range vpcAllInfo.AllListInfo.OnlySpiderList {
			for _, subnet := range vpc.SubnetInfoList {
				spiderOnlySubnets = append(spiderOnlySubnets, model.SpiderNameIdSystemId{
					NameId:   subnet.IId.NameId,
					SystemId: subnet.IId.SystemId,
				})
			}
		}

		// [Phase 3] CspOnly VPC subnets — all subnets of a CspOnly VPC are CspOnly.
		for _, vpc := range vpcAllInfo.AllListInfo.OnlyCSPInfoList {
			for _, subnet := range vpc.SubnetInfoList {
				cspOnlySubnets = append(cspOnlySubnets, model.SpiderNameIdSystemId{
					NameId:   subnet.IId.NameId,
					SystemId: subnet.IId.SystemId,
				})
			}
		}
	}

	return model.SpiderAllList{
		MappedList:     mappedSubnets,
		OnlySpiderList: spiderOnlySubnets,
		OnlyCSPList:    cspOnlySubnets,
	}
}

// classifyMappedVpcSubnets classifies subnets of a single Mapped VPC into
// Mapped / SpiderOnly / CspOnly using SystemId matching against Spider's IID registry.
func classifyMappedVpcSubnets(client *resty.Client, vpc model.SpiderVpcInfo, connConfig string) (
	mapped, spiderOnly, cspOnly []model.SpiderNameIdSystemId,
) {
	// Step 1. Fetch Spider's IID registry for this VPC.
	// spiderSystemIds: SystemId set for classification
	// spiderNameIds:   SystemId → NameId for name resolution
	spiderSystemIds := make(map[string]bool)
	spiderNameIds := make(map[string]string)
	spiderVpcInfo, err := fetchSpiderVpcInfo(client, vpc.IId.NameId, connConfig)
	if err != nil {
		log.Warn().Err(err).Msgf("Failed to query Spider VPC info for %s", vpc.IId.NameId)
	} else {
		for _, sub := range spiderVpcInfo.SubnetInfoList {
			if sub.IId.SystemId == "" {
				continue
			}
			spiderSystemIds[sub.IId.SystemId] = true
			if sub.IId.NameId != "" {
				spiderNameIds[sub.IId.SystemId] = sub.IId.NameId
			}
		}
	}

	// Step 2. Classify CSP subnets: Mapped if SystemId is in Spider registry, CspOnly otherwise.
	cspSubnetSystemIds := make(map[string]bool)
	for _, subnet := range vpc.SubnetInfoList {
		if subnet.IId.SystemId == "" {
			continue
		}
		cspSubnetSystemIds[subnet.IId.SystemId] = true

		if spiderSystemIds[subnet.IId.SystemId] {
			mapped = append(mapped, model.SpiderNameIdSystemId{
				NameId:   spiderNameIds[subnet.IId.SystemId],
				SystemId: subnet.IId.SystemId,
			})
		} else {
			cspOnly = append(cspOnly, model.SpiderNameIdSystemId{
				NameId:   "",
				SystemId: subnet.IId.SystemId,
			})
		}
	}

	// Step 3. Subnets in Spider's IID registry but absent from CSP → SpiderOnly.
	for _, sub := range spiderVpcInfo.SubnetInfoList {
		if sub.IId.SystemId == "" || cspSubnetSystemIds[sub.IId.SystemId] {
			continue
		}
		spiderOnly = append(spiderOnly, model.SpiderNameIdSystemId{
			NameId:   sub.IId.NameId,
			SystemId: sub.IId.SystemId,
		})
	}

	return mapped, spiderOnly, cspOnly
}

// fetchSpiderVpcInfo queries Spider's IID registry for a VPC and its registered subnets.
func fetchSpiderVpcInfo(client *resty.Client, vpcNameId, connConfig string) (model.SpiderVpcInfo, error) {
	url := fmt.Sprintf("%s/vpc/%s?ConnectionName=%s", model.SpiderRestUrl, vpcNameId, connConfig)
	noBody := clientManager.NoBody
	var spiderVpcInfo model.SpiderVpcInfo
	_, err := clientManager.ExecuteHttpRequest(
		client,
		"GET",
		url,
		nil,
		clientManager.SetUseBody(noBody),
		&noBody,
		&spiderVpcInfo,
		clientManager.MediumDuration,
	)
	return spiderVpcInfo, err
}

// GetResourceSyncState returns the observed ResourceSyncState across 3 layers (TB, Spider, CSP).
// Assumes Tumblebug metadata exists for the resource (TB: O).
func GetResourceSyncState(resourceName string, resourceSystemId string, statusResp model.CspResourceStatusResponse) model.ResourceSyncState {
	allList := statusResp.AllList
	for _, item := range allList.MappedList {
		if item.NameId == resourceName || (resourceSystemId != "" && item.SystemId == resourceSystemId) {
			return model.SyncStateInSync // Mapped (TB: O, SP: O, CSP: O)
		}
	}
	for _, item := range allList.OnlySpiderList {
		if item.NameId == resourceName {
			return model.SyncStateCspResourceMissing // OnlySpider (TB: O, SP: O, CSP: X)
		}
	}
	for _, item := range allList.OnlyCSPList {
		if item.NameId == resourceName || (resourceSystemId != "" && item.SystemId == resourceSystemId) {
			return model.SyncStateSpMetaMissing // OnlyCSP (TB: O, SP: X, CSP: O)
		}
	}
	return model.SyncStateTbMetaOnly // Absent (TB: O, SP: X, CSP: X)
}

// GetCspResourceStatusBatch retrieves resource status for multiple resource types in a single connection
//
// This is a convenience function that calls GetCspResourceStatus for multiple resource types
// and returns a map of results. This is useful when you need to check multiple resource types
// for the same connection configuration.
//
// Parameters:
//   - connConfig: Connection configuration name for the target CSP
//   - resourceTypes: List of resource types to query
//
// Returns:
//   - map[string]model.CspResourceStatusResponse: Map of resource type to response
//   - error: Error if any of the operations fail (returns first error encountered)
//
// Example usage:
//
//	resourceTypes := []string{model.StrNode, model.StrVNet, model.StrSecurityGroup}
//	responses, err := GetCspResourceStatusBatch("aws-connection", resourceTypes)
//	if err != nil {
//	    log.Error().Err(err).Msg("Failed to get batch CSP resource status")
//	    return err
//	}
//
//	for resourceType, response := range responses {
//	    fmt.Printf("%s: %s\n", resourceType, response.SystemMessage)
//	}
func GetCspResourceStatusBatch(connConfig string, resourceTypes []string) (map[string]model.CspResourceStatusResponse, error) {
	results := make(map[string]model.CspResourceStatusResponse)

	for _, resourceType := range resourceTypes {
		response, err := GetCspResourceStatus(connConfig, resourceType)
		if err != nil {
			log.Error().Err(err).Str("connection", connConfig).Str("resourceType", resourceType).
				Msg("Failed to get CSP resource status in batch operation")
			return results, fmt.Errorf("failed to get status for %s in %s: %w", resourceType, connConfig, err)
		}
		results[resourceType] = response
	}

	log.Info().Str("connection", connConfig).Int("resourceTypes", len(resourceTypes)).
		Msg("Successfully completed batch CSP resource status retrieval")

	return results, nil
}

// CheckAssociatedCspResourceExistence checks if a CB-TB resource's associated CSP resource exists in Spider and CSP
//
// This function takes a CB-TB resource and checks if its corresponding CSP resource exists in:
//   - CSP (Cloud Service Provider): Checks MappedList and OnlyCSPList
//   - Spider (CB-Spider): Checks MappedList and OnlySpiderList
//
// Parameters:
//   - nsId: Namespace ID of the CB-TB resource
//   - resourceType: Type of the CB-TB resource (e.g., model.StrNode, model.StrVNet, etc.)
//   - resourceId: ID of the CB-TB resource
//   - connConfig: Connection configuration name for the target CSP
//
// Returns:
//   - onCsp: true if the resource exists in CSP (either mapped or CSP-only)
//   - onSpider: true if the resource exists in Spider (either mapped or Spider-only)
//   - error: Error if the operation fails (connection errors, resource not found, etc.)
//
// Example usage:
//
//	onCsp, onSpider, err := CheckAssociatedCspResourceExistence("default", model.StrNode, "my-vm-01", "aws-connection")
//	if err != nil {
//	    log.Error().Err(err).Msg("Failed to check resource existence")
//	    return err
//	}
//
//	if onCsp && onSpider {
//	    fmt.Println("Resource exists in both CSP and Spider (mapped)")
//	} else if onCsp && !onSpider {
//	    fmt.Println("Resource exists only in CSP")
//	} else if !onCsp && onSpider {
//	    fmt.Println("Resource exists only in Spider")
//	} else {
//	    fmt.Println("Resource does not exist in either CSP or Spider")
//	}
func CheckAssociatedCspResourceExistence(nsId string, resourceType string, resourceId string, connConfig string) (onCsp bool, onSpider bool, err error) {
	// Initialize return values
	onCsp = false
	onSpider = false

	// Get the CSP resource ID/name from CB-TB resource
	cspResourceId, err := GetCspResourceId(nsId, resourceType, resourceId)
	if err != nil {
		log.Error().Err(err).Str("nsId", nsId).Str("resourceType", resourceType).
			Str("resourceId", resourceId).Msg("Failed to get CSP resource ID from CB-TB resource")
		return false, false, fmt.Errorf("failed to get CSP resource ID for %s/%s/%s: %w", nsId, resourceType, resourceId, err)
	}

	if cspResourceId == "" {
		log.Warn().Str("nsId", nsId).Str("resourceType", resourceType).
			Str("resourceId", resourceId).Msg("CSP resource ID is empty")
		return false, false, fmt.Errorf("CSP resource ID is empty for %s/%s/%s", nsId, resourceType, resourceId)
	}

	// Get CSP resource status from Spider
	response, err := GetCspResourceStatus(connConfig, resourceType)
	if err != nil {
		log.Error().Err(err).Str("connection", connConfig).Str("resourceType", resourceType).
			Msg("Failed to get CSP resource status")
		return false, false, fmt.Errorf("failed to get CSP resource status for %s/%s: %w", connConfig, resourceType, err)
	}

	// Check if the CSP resource exists in MappedList
	for _, resource := range response.AllList.MappedList {
		if resource.SystemId == cspResourceId {
			log.Debug().Str("cspResourceId", cspResourceId).Str("systemId", resource.SystemId).
				Msg("Found resource in MappedList")
			return true, true, nil // Mapped resources exist in both CSP and Spider
		}
	}

	// Check if the CSP resource exists in OnlyCSPList
	for _, resource := range response.AllList.OnlyCSPList {
		if resource.SystemId == cspResourceId {
			log.Debug().Str("cspResourceId", cspResourceId).Str("systemId", resource.SystemId).
				Msg("Found resource in OnlyCSPList")
			return true, false, nil // Exists only in CSP
		}
	}

	// Check if the CSP resource exists in OnlySpiderList
	for _, resource := range response.AllList.OnlySpiderList {
		if resource.SystemId == cspResourceId {
			log.Debug().Str("cspResourceId", cspResourceId).Str("systemId", resource.SystemId).
				Msg("Found resource in OnlySpiderList")
			return false, true, nil // Exists only in Spider
		}
	}

	// Resource not found in any list
	log.Debug().Str("cspResourceId", cspResourceId).Str("connection", connConfig).
		Str("resourceType", resourceType).Msg("Resource not found in any list")
	return false, false, nil
}

