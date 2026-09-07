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

// Package common is to include common methods for managing multi-cloud infra
package common

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	clientManager "github.com/cloud-barista/cb-tumblebug/src/core/common/client"
	"github.com/cloud-barista/cb-tumblebug/src/core/model"
	modelcsp "github.com/cloud-barista/cb-tumblebug/src/core/model/csp"
	"github.com/cloud-barista/cb-tumblebug/src/kvstore/kvstore"
	"github.com/cloud-barista/cb-tumblebug/src/kvstore/kvutil"
	"github.com/rs/zerolog/log"
)

func GetConnConfig(ConnConfigName string) (model.ConnConfig, error) {

	connConfig := model.ConnConfig{}

	key := GenConnectionKey(ConnConfigName)
	keyValue, exists, err := kvstore.GetKv(key)
	if err != nil {
		log.Error().Err(err).Msg("")
		return model.ConnConfig{}, err
	}
	if !exists {
		return model.ConnConfig{}, fmt.Errorf("Cannot find the model.ConnConfig %s", key)
	}
	err = json.Unmarshal([]byte(keyValue.Value), &connConfig)
	if err != nil {
		log.Error().Err(err).Msg("")
		return model.ConnConfig{}, err
	}

	return connConfig, nil
}

// GetConnConfig is func to get connection config
func GetProviderNameFromConnConfig(ConnConfigName string) (string, error) {
	connConfig, err := GetConnConfig(ConnConfigName)
	if err != nil {
		return "", err
	}
	return connConfig.ProviderName, nil
}

// CheckConnConfigAvailable is func to check if connection config is available by checking allkeypair list
func CheckConnConfigAvailable(connConfigName string) (bool, error) {

	var callResult model.SpiderAllListWrapper
	client := clientManager.NewHttpClient()
	client.SetTimeout(clientManager.AvailabilityCheckTimeout)
	url := model.SpiderRestUrl + "/allkeypair"
	method := "GET"
	requestBody := model.SpiderConnectionName{}
	requestBody.ConnectionName = connConfigName

	_, err := clientManager.ExecuteHttpRequest(
		client,
		method,
		url,
		nil,
		clientManager.SetUseBody(requestBody),
		&requestBody,
		&callResult,
		clientManager.ShortDuration,
	)

	if err != nil {
		//log.Info().Err(err).Msg("")
		return false, err
	}

	return true, nil
}

// CheckSpiderStatus is func to check if CB-Spider is ready
func CheckSpiderReady() error {

	var callResult any
	client := clientManager.NewHttpClient()
	url := model.SpiderRestUrl + "/readyz"
	method := "GET"
	requestBody := clientManager.NoBody

	_, err := clientManager.ExecuteHttpRequest(
		client,
		method,
		url,
		nil,
		clientManager.SetUseBody(requestBody),
		&requestBody,
		&callResult,
		clientManager.VeryShortDuration,
	)

	if err != nil {
		//log.Err(err).Msg("")
		return err
	}

	return nil
}

// GetConnConfigList is func to list filtered connection configs
func GetConnConfigList(filterCredentialHolder string, filterVerified bool, filterRegionRepresentative bool) (model.ConnConfigList, error) {
	var filteredConnections model.ConnConfigList
	var tmpConnections model.ConnConfigList

	key := "/connection"
	keyValue, err := kvstore.GetKvList(key)
	keyValue = kvutil.FilterKvListBy(keyValue, key, 1)

	if err != nil {
		log.Error().Err(err).Msg("")
		return model.ConnConfigList{}, err
	}
	if keyValue != nil {
		for _, v := range keyValue {
			tempObj := model.ConnConfig{}
			err = json.Unmarshal([]byte(v.Value), &tempObj)
			if err != nil {
				log.Error().Err(err).Msg("")
				return filteredConnections, err
			}
			filteredConnections.Connectionconfig = append(filteredConnections.Connectionconfig, tempObj)
		}
	} else {
		return model.ConnConfigList{}, nil
	}

	// filter by credential holder
	if filterCredentialHolder != "" {
		for _, connConfig := range filteredConnections.Connectionconfig {
			if strings.EqualFold(connConfig.CredentialHolder, filterCredentialHolder) {
				tmpConnections.Connectionconfig = append(tmpConnections.Connectionconfig, connConfig)
			}
		}
		filteredConnections = tmpConnections
		tmpConnections = model.ConnConfigList{}
	}

	// filter only verified
	if filterVerified {
		for _, connConfig := range filteredConnections.Connectionconfig {
			if connConfig.Verified {
				tmpConnections.Connectionconfig = append(tmpConnections.Connectionconfig, connConfig)
			}
		}
		filteredConnections = tmpConnections
		tmpConnections = model.ConnConfigList{}
	}

	// filter only region representative
	if filterRegionRepresentative {
		for _, connConfig := range filteredConnections.Connectionconfig {
			if connConfig.RegionRepresentative {
				tmpConnections.Connectionconfig = append(tmpConnections.Connectionconfig, connConfig)
			}
		}
		filteredConnections = tmpConnections
		tmpConnections = model.ConnConfigList{}
	}
	//log.Info().Msgf("Filtered connection config count: %d", len(filteredConnections.Connectionconfig))
	return filteredConnections, nil
}

// GetConnConfigListByProviderRegionZone filters connection configs by provider, region, and zone
// - provider empty: all connections
// - provider specified, region empty: all connections for the provider
// - provider + region specified, zone empty: all connections for the provider and region
// - provider + region + zone specified: connections matching all three
func GetConnConfigListByProviderRegionZone(provider, region, zone string) ([]string, error) {
	// Get all available connections
	allConnections, err := GetConnConfigList(model.DefaultCredentialHolder, true, true)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get connection config list")
		return nil, err
	}

	var filteredConnectionNames []string

	// If provider is empty, return all connection names
	if provider == "" {
		for _, conn := range allConnections.Connectionconfig {
			filteredConnectionNames = append(filteredConnectionNames, conn.ConfigName)
		}
		return filteredConnectionNames, nil
	}

	// Filter by provider, region, and zone
	for _, conn := range allConnections.Connectionconfig {
		// Match provider (case-insensitive)
		if !strings.EqualFold(conn.ProviderName, provider) {
			continue
		}

		// If region is specified, match region
		if region != "" {
			assignedRegion := conn.RegionZoneInfo.AssignedRegion
			if assignedRegion != region {
				continue
			}

			// If zone is specified, match zone
			if zone != "" {
				assignedZone := conn.RegionZoneInfo.AssignedZone
				if assignedZone != zone {
					continue
				}
			}
		}

		// Connection matches all criteria
		filteredConnectionNames = append(filteredConnectionNames, conn.ConfigName)
	}

	return filteredConnectionNames, nil
}

// RegisterAllCloudInfo is func to register all cloud info from asset to CB-Spider
func RegisterAllCloudInfo() error {
	// First, populate the cloud platform mapping for all CSPs
	for providerName, cspDetail := range snapshotCspDetails() {
		if cspDetail.CloudPlatform != "" {
			// Derived CSP: maps to the base platform (e.g., openstack-new01 → openstack)
			modelcsp.RegisterCloudPlatform(providerName, cspDetail.CloudPlatform)
		} else {
			// Standard CSP: identity mapping (e.g., aws → aws)
			modelcsp.RegisterCloudPlatform(providerName, providerName)
		}
	}

	for _, providerName := range listCspNames() {
		err := RegisterCloudInfo(providerName)
		if err != nil {
			log.Error().Err(err).Msg("")
		}
	}
	return nil
}

// GetProviderList is func to list all cloud providers
func GetProviderList() (*model.IdList, error) {
	providers := model.IdList{}
	providers.IdList = append(providers.IdList, listCspNames()...)
	return &providers, nil
}

// RegisterCloudInfo is func to register cloud info from asset to CB-Spider
func RegisterCloudInfo(providerName string) error {

	cspDetail, ok := getCspDetail(providerName)
	if !ok {
		return fmt.Errorf("provider %q is not registered", providerName)
	}
	driverName := cspDetail.Driver

	// Resolve the cloud platform type for Spider registration.
	// Spider uses ProviderName to select the correct driver handler,
	// so it must be the platform type (e.g., "OPENSTACK") not the CSP instance name (e.g., "OPENSTACK-NEW01").
	platformName := modelcsp.ResolveCloudPlatform(providerName)

	client := clientManager.NewHttpClient()
	url := model.SpiderRestUrl + "/driver"
	method := "POST"
	var callResult model.CloudDriverInfo
	requestBody := model.CloudDriverInfo{ProviderName: strings.ToUpper(platformName), DriverName: driverName, DriverLibFileName: driverName}

	_, err := clientManager.ExecuteHttpRequest(
		client,
		method,
		url,
		nil,
		clientManager.SetUseBody(requestBody),
		&requestBody,
		&callResult,
		clientManager.MediumDuration,
	)

	if err != nil {
		log.Error().Err(err).Msg("")
		return err
	}

	for regionName := range cspDetail.Regions {
		err := RegisterRegionZone(providerName, regionName)
		if err != nil {
			log.Error().Err(err).Msg("")
			return err
		}
	}

	return nil
}

// RegisterRegionZone is func to register all regions to CB-Spider
func RegisterRegionZone(providerName string, regionName string) error {
	client := clientManager.NewHttpClient()
	url := model.SpiderRestUrl + "/region"
	method := "POST"
	var callResult model.SpiderRegionZoneInfo

	// Use platform name for Spider's ProviderName (driver selection),
	// but keep CSP instance name in RegionName for uniqueness.
	platformName := modelcsp.ResolveCloudPlatform(providerName)
	requestBody := model.SpiderRegionZoneInfo{ProviderName: strings.ToUpper(platformName), RegionName: regionName}

	// register representative regionZone (region only)
	requestBody.RegionName = providerName + "-" + regionName
	keyValueInfoList := []model.KeyValue{}
	emptyZone := ""

	// Determine representative zone based on configuration priority:
	// 1. If region has explicit representativeZone set -> use that value
	// 2. If CSP has useEmptyRepresentativeZone: true -> use empty zone (for flexible VM placement)
	// 3. Otherwise, use Zones[0] if available, or empty zone if no zones exist
	//
	// Note: "empty zone" means the zone field is left unspecified in the API request.
	// When zone is empty, CSPs typically auto-select an available zone for the resource.
	// This is useful for specialized resources (e.g., GPU VMs) that may only be available
	// in specific zones that vary by region and time.
	cspInfo, _ := getCspDetail(providerName)
	regionInfo := cspInfo.Regions[regionName]

	var representativeZone string
	if regionInfo.RepresentativeZone != nil {
		// Priority 1: Explicit representativeZone in region config
		representativeZone = *regionInfo.RepresentativeZone
	} else if cspInfo.UseEmptyRepresentativeZone {
		// Priority 2: CSP-level setting to use empty zone (e.g., Azure for GPU VM flexibility)
		representativeZone = emptyZone
	} else if len(regionInfo.Zones) > 0 {
		// Priority 3: Use first zone from zone list
		representativeZone = regionInfo.Zones[0]
	} else {
		// Default: Empty zone when no zones exist
		representativeZone = emptyZone
	}

	keyValueInfoList = []model.KeyValue{
		{Key: "Region", Value: regionInfo.RegionId},
		{Key: "Zone", Value: representativeZone},
	}
	requestBody.KeyValueInfoList = keyValueInfoList

	_, err := clientManager.ExecuteHttpRequest(
		client,
		method,
		url,
		nil,
		clientManager.SetUseBody(requestBody),
		&requestBody,
		&callResult,
		clientManager.MediumDuration,
	)

	if err != nil {
		log.Error().Err(err).Msg("")
		return err
	}

	// register all regionZones
	for _, zoneName := range regionInfo.Zones {
		requestBody.RegionName = providerName + "-" + regionName + "-" + zoneName
		keyValueInfoList := []model.KeyValue{
			{Key: "Region", Value: regionInfo.RegionId},
			{Key: "Zone", Value: zoneName},
		}
		requestBody.AvailableZoneList = regionInfo.Zones
		requestBody.KeyValueInfoList = keyValueInfoList

		_, err := clientManager.ExecuteHttpRequest(
			client,
			method,
			url,
			nil,
			clientManager.SetUseBody(requestBody),
			&requestBody,
			&callResult,
			clientManager.MediumDuration,
		)

		if err != nil {
			log.Error().Err(err).Msg("")
			return err
		}

	}

	return nil
}

var privateKeyStore = make(map[string]*rsa.PrivateKey)
var mu sync.Mutex // Concurrency safety

// GetPublicKeyForCredentialEncryption generates an RSA key pair,
// stores the private key in memory, and returns the public key along with its token ID.

func RegisterConnectionConfig(connConfig model.ConnConfig) (model.ConnConfig, error) {
	client := clientManager.NewHttpClient()
	url := model.SpiderRestUrl + "/connectionconfig"
	method := "POST"
	var callResult model.SpiderConnConfig
	requestBody := model.SpiderConnConfig{}
	requestBody.ConfigName = connConfig.ConfigName
	// Spider needs the platform type (e.g., "OPENSTACK") for driver selection,
	// not the CSP instance name (e.g., "openstack-new01")
	requestBody.ProviderName = strings.ToUpper(modelcsp.ResolveCloudPlatform(connConfig.ProviderName))
	requestBody.DriverName = connConfig.DriverName
	requestBody.CredentialName = connConfig.CredentialName
	requestBody.RegionName = connConfig.RegionZoneInfoName

	_, err := clientManager.ExecuteHttpRequest(
		client,
		method,
		url,
		nil,
		clientManager.SetUseBody(requestBody),
		&requestBody,
		&callResult,
		clientManager.MediumDuration,
	)

	if err != nil {
		log.Error().Err(err).Msg("")
		return model.ConnConfig{}, err
	}

	// Register connection to cb-tumblebug with availability check
	// verified, err := CheckConnConfigAvailable(callResult.ConfigName)
	// if err != nil {
	// 	log.Error().Err(err).Msgf("Cannot check model.ConnConfig %s is available", connConfig.ConfigName)
	// }
	// callResult.ProviderName = strings.ToLower(callResult.ProviderName)
	// if verified {
	// 	nativeRegion, _, err := GetRegion(callResult.RegionName)
	// 	if err != nil {
	// 		log.Error().Err(err).Msgf("Cannot get region for %s", callResult.RegionName)
	// 		callResult.Verified = false
	// 	} else {
	// 		location, err := GetCloudLocation(callResult.ProviderName, nativeRegion)
	// 		if err != nil {
	// 			log.Error().Err(err).Msgf("Cannot get location for %s/%s", callResult.ProviderName, nativeRegion)
	// 		}
	// 		callResult.Location = location
	// 	}
	// }

	connection := model.ConnConfig{}
	connection.ConfigName = callResult.ConfigName
	// Preserve the original CSP instance name (e.g., "openstack-new01"),
	// not the platform type returned by Spider (e.g., "OPENSTACK")
	connection.ProviderName = strings.ToLower(connConfig.ProviderName)
	connection.DriverName = callResult.DriverName
	connection.CredentialName = callResult.CredentialName
	connection.RegionZoneInfoName = callResult.RegionName
	connection.CredentialHolder = connConfig.CredentialHolder

	// load region info
	url = model.SpiderRestUrl + "/region/" + connection.RegionZoneInfoName
	method = "GET"
	var callResultRegion model.SpiderRegionZoneInfo
	requestNoBody := clientManager.NoBody

	_, err = clientManager.ExecuteHttpRequest(
		client,
		method,
		url,
		nil,
		clientManager.SetUseBody(requestNoBody),
		&requestNoBody,
		&callResultRegion,
		clientManager.MediumDuration,
	)
	if err != nil {
		log.Error().Err(err).Msg("")
		return model.ConnConfig{}, err
	}
	regionZoneInfo := model.RegionZoneInfo{}
	for _, keyVal := range callResultRegion.KeyValueInfoList {
		if keyVal.Key == "Region" {
			regionZoneInfo.AssignedRegion = keyVal.Value
		}
		if keyVal.Key == "Zone" {
			regionZoneInfo.AssignedZone = keyVal.Value
		}
	}
	connection.RegionZoneInfo = regionZoneInfo

	regionDetail, err := GetRegion(connection.ProviderName, connection.RegionZoneInfo.AssignedRegion)
	if err != nil {
		log.Error().Err(err).Msgf("Cannot get region for %s", connection.RegionZoneInfo.AssignedRegion)
		return model.ConnConfig{}, err
	}
	connection.RegionDetail = regionDetail

	key := GenConnectionKey(connection.ConfigName)
	val, err := json.Marshal(connection)
	if err != nil {
		return model.ConnConfig{}, err
	}
	err = kvstore.Put(string(key), string(val))
	if err != nil {
		log.Error().Err(err).Msg("")
		return model.ConnConfig{}, err
	}

	return connection, nil
}

// GetRegion is func to get regionInfo with the native region name
func GetRegion(ProviderName, RegionName string) (model.RegionDetail, error) {

	ProviderName = strings.ToLower(ProviderName)
	RegionName = strings.ToLower(RegionName)

	cloudInfo, err := GetCloudInfo()
	if err != nil {
		return model.RegionDetail{}, err
	}

	cspDetail, ok := cloudInfo.CSPs[ProviderName]
	if !ok {
		return model.RegionDetail{}, fmt.Errorf("cloudType '%s' not found", ProviderName)
	}

	// directly getting value from the map is disabled because of some possible case mismatches (enhancement needed)
	// regionDetail, ok := cspDetail.Regions[nativeRegion]
	// if !ok {
	// 	model.RegionDetail{}, fmt.Errorf("nativeRegion '%s' not found in Provider '%s'", RegionName, ProviderName)
	// }
	for key, regionDetail := range cspDetail.Regions {
		if strings.EqualFold(RegionName, key) {
			return regionDetail, nil
		}
	}

	return model.RegionDetail{}, fmt.Errorf("nativeRegion '%s' not found in Provider '%s'", RegionName, ProviderName)
}

// GetRegions is func to get regionInfo list
func GetRegions(ProviderName string) (model.RegionList, error) {

	ProviderName = strings.ToLower(ProviderName)

	cloudInfo, err := GetCloudInfo()
	if err != nil {
		return model.RegionList{}, err
	}

	cspDetail, ok := cloudInfo.CSPs[ProviderName]
	if !ok {
		return model.RegionList{}, fmt.Errorf("cloudType '%s' not found", ProviderName)
	}

	regionList := model.RegionList{}
	for _, regionDetail := range cspDetail.Regions {
		regionList.Regions = append(regionList.Regions, regionDetail)
	}

	return regionList, nil
}

// RetrieveRegionListFromCsp is func to retrieve region list
func RetrieveRegionListFromCsp() (model.RetrievedRegionList, error) {

	url := model.SpiderRestUrl + "/region"

	client := clientManager.NewHttpClient()

	resp, err := client.R().
		SetResult(&model.RetrievedRegionList{}).
		//SetError(&SimpleMsg{}).
		Get(url)

	if err != nil {
		log.Error().Err(err).Msg("")
		content := model.RetrievedRegionList{}
		err := fmt.Errorf("an error occurred while requesting to CB-Spider")
		return content, err
	}

	switch {
	case resp.StatusCode() >= 400 || resp.StatusCode() < 200:
		err := fmt.Errorf("%s", string(resp.Body()))
		log.Error().Err(err).Msg("")
		content := model.RetrievedRegionList{}
		return content, err
	}

	temp, _ := resp.Result().(*model.RetrievedRegionList)
	return *temp, nil

}

// ConvertToMessage is func to change input data to gRPC message
