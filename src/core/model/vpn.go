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

// Package mci is to handle REST API for mci
package model

var ProviderNames = map[string]string{
	"AWS":   "aws",
	"Azure": "azure",
	"GCP":   "gcp",
}

// SiteDetail struct represents the structure for detailed site information
type SiteDetail struct {
	CSP            string `json:"csp" example:"aws"`
	Region         string `json:"region" example:"ap-northeast-2"`
	ConnectionName string `json:"connectionName" example:"aws-ap-northeast-2"`
	// Zone              string `json:"zone,omitempty" example:"ap-northeast-2a"`
	VNet              string `json:"vnet" example:"vpc-xxxxx"`
	Subnet            string `json:"subnet,omitempty" example:"subnet-xxxxx"`
	GatewaySubnetCidr string `json:"gatewaySubnetCidr,omitempty" example:"xxx.xxx.xxx.xxx/xx"`
	ResourceGroup     string `json:"resourceGroup,omitempty" example:"rg-xxxxx"`
}

// Sites struct represents the overall site information
type sites struct {
	Aws   []SiteDetail `json:"aws"`
	Azure []SiteDetail `json:"azure"`
	Gcp   []SiteDetail `json:"gcp"`
}

// SitesInfo struct represents the overall site information including namespace and MCI ID
type SitesInfo struct {
	NsId  string `json:"nsId" example:"ns-01"`
	MciId string `json:"mciId" example:"mci-01"`
	Count int    `json:"count" example:"3"`
	Sites sites  `json:"sites"`
}

func NewSiteInfo(nsId, mciId string) *SitesInfo {
	siteInfo := &SitesInfo{
		NsId:  nsId,
		MciId: mciId,
		Count: 0,
		Sites: sites{
			Aws:   []SiteDetail{},
			Azure: []SiteDetail{},
			Gcp:   []SiteDetail{},
		},
	}

	return siteInfo
}

type RestPostVpnRequest struct {
	Name  string     `json:"name" validate:"required" example:"vpn01"`
	Site1 SiteDetail `json:"site1" validate:"required"`
	Site2 SiteDetail `json:"site2" validate:"required"`
}

type Response struct {
	Success bool                   `json:"success" example:"true"`
	Status  int                    `json:"status,omitempty" example:"200"`
	Message string                 `json:"message" example:"Any message"`
	Detail  string                 `json:"details,omitempty" example:"Any details"`
	Object  map[string]interface{} `json:"object,omitempty"`
	List    []interface{}          `json:"list,omitempty"`
}

type VpnIdList struct {
	VpnIdList []string `json:"vpnIdList"`
}

type VpnInfoList struct {
	VpnInfoList []VPNInfo `json:"vpnInfoList"`
}

type VPNInfo struct {
	// ResourceType is the type of the resource
	ResourceType string `json:"resourceType"`

	// Id is unique identifier for the object
	Id string `json:"id" example:"vpn01"`
	// Uid is universally unique identifier for the object, used for labelSelector
	Uid string `json:"uid,omitempty" example:"wef12awefadf1221edcf"`

	// Name is human-readable string to represent the object
	Name           string           `json:"name" example:"vpn01"`
	Description    string           `json:"description"`
	Status         string           `json:"status"`
	VPNGatewayInfo []VPNGatewayInfo `json:"vpnGatewayInfo"`
}

type VPNGatewayInfo struct {
	ConnectionName   string     `json:"connectionName"`
	ConnectionConfig ConnConfig `json:"connectionConfig"`
	// CspResourceName is name assigned to the CSP resource. This name is internally used to handle the resource.
	CspResourceName string `json:"cspResourceName,omitempty" example:"we12fawefadf1221edcf"`
	// CspResourceId is resource identifier managed by CSP
	CspResourceId string      `json:"cspResourceId,omitempty" example:"csp-06eb41e14121c550a"`
	Status        string      `json:"status"`
	Description   string      `json:"description"`
	Details       interface{} `json:"details"`
}

// // TbVNetInfo is a struct that represents TB vNet object.
// type TbVNetInfo struct {
// 	// ResourceType is the type of the resource
// 	ResourceType string `json:"resourceType"`

// 	// Id is unique identifier for the object
// 	Id string `json:"id" example:"aws-ap-southeast-1"`
// 	// Uid is universally unique identifier for the object, used for labelSelector
// 	Uid string `json:"uid,omitempty" example:"wef12awefadf1221edcf"`
// 	// CspResourceName is name assigned to the CSP resource. This name is internally used to handle the resource.
// 	CspResourceName string `json:"cspResourceName,omitempty" example:"we12fawefadf1221edcf"`
// 	// CspResourceId is resource identifier managed by CSP
// 	CspResourceId string `json:"cspResourceId,omitempty" example:"csp-06eb41e14121c550a"`

// 	// Name is human-readable string to represent the object
// 	Name             string     `json:"name" example:"aws-ap-southeast-1"`
// 	ConnectionName   string     `json:"connectionName"`
// 	ConnectionConfig ConnConfig `json:"connectionConfig"`

// 	CidrBlock            string         `json:"cidrBlock"`
// 	SubnetInfoList       []TbSubnetInfo `json:"subnetInfoList"`
// 	Description          string         `json:"description"`
// 	Status               string         `json:"status"`
// 	KeyValueList         []KeyValue     `json:"keyValueList,omitempty"`
// 	AssociatedObjectList []string       `json:"associatedObjectList"`
// 	IsAutoGenerated      bool           `json:"isAutoGenerated"`
// 	// todo: restore the tag list later
// 	// TagList              []KeyValue     `json:"tagList,omitempty"`

// 	// SystemLabel is for describing the Resource in a keyword (any string can be used) for special System purpose
// 	SystemLabel string `json:"systemLabel" example:"Managed by CB-Tumblebug" default:""`

// 	// Disabled for now
// 	//Region         string `json:"region"`
// 	//ResourceGroupName string `json:"resourceGroupName"`
// }

// type RestPostVpnGcpToAwsRequest struct {
// 	TfVars TfVarsGcpAwsVpnTunnel `json:"tfVars"`
// }

// type TfVarsGcpAwsVpnTunnel struct {
// 	ResourceGroupId   string `json:"resource-group-id,omitempty" default:"" example:""`
// 	AwsRegion         string `json:"aws-region" validate:"required" default:"ap-northeast-2" example:"ap-northeast-2"`
// 	AwsVpcId          string `json:"aws-vpc-id" validate:"required" example:"vpc-xxxxx"`
// 	AwsSubnetId       string `json:"aws-subnet-id" validate:"required" example:"subnet-xxxxx"`
// 	GcpRegion         string `json:"gcp-region" validate:"required" default:"asia-northeast3" example:"asia-northeast3"`
// 	GcpVpcNetworkName string `json:"gcp-vpc-network-name" validate:"required" default:"vpc01" example:"vpc01"`
// 	// GcpBgpAsn                   string `json:"gcp-bgp-asn" default:"65530"`
// }

// type TfVarsGcpAzureVpnTunnel struct {
// 	ResourceGroupId             string `json:"resource-group-id,omitempty" default:"" example:""`
// 	AzureRegion                 string `json:"azure-region" default:"koreacentral" example:"koreacentral"`
// 	AzureResourceGroupName      string `json:"azure-resource-group-name" default:"tofu-rg-01" example:"tofu-rg-01"`
// 	AzureVirtualNetworkName     string `json:"azure-virtual-network-name" default:"tofu-azure-vnet" example:"tofu-azure-vnet"`
// 	AzureGatewaySubnetCidrBlock string `json:"azure-gateway-subnet-cidr-block" default:"192.168.130.0/24" example:"192.168.130.0/24"`
// 	GcpRegion                   string `json:"gcp-region" default:"asia-northeast3" example:"asia-northeast3"`
// 	GcpVpcNetworkName           string `json:"gcp-vpc-network-name" default:"tofu-gcp-vpc" example:"tofu-gcp-vpc"`
// 	// AzureBgpAsn				 	string `json:"azure-bgp-asn" default:"65515"`
// 	// GcpBgpAsn                   string `json:"gcp-bgp-asn" default:"65534"`
// 	// AzureSubnetName             string `json:"azure-subnet-name" default:"tofu-azure-subnet-0"`
// 	// GcpVpcSubnetworkName    string `json:"gcp-vpc-subnetwork-name" default:"tofu-gcp-subnet-1"`
// }
