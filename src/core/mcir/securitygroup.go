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

// Package mcir is to manage multi-cloud infra resource
package mcir

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/cloud-barista/cb-spider/interface/api"
	"github.com/cloud-barista/cb-tumblebug/src/core/common"
	validator "github.com/go-playground/validator/v10"
	"github.com/go-resty/resty/v2"
)

// SpiderSecurityReqInfoWrapper is a wrapper struct to create JSON body of 'Create security group request'
type SpiderSecurityReqInfoWrapper struct { // Spider
	ConnectionName string
	ReqInfo        SpiderSecurityInfo
}

// SpiderSecurityRuleInfo is a struct to handle security group rule info from/to CB-Spider.
type SpiderSecurityRuleInfo struct { // Spider
	FromPort   string //`json:"fromPort"`
	ToPort     string //`json:"toPort"`
	IPProtocol string //`json:"ipProtocol"`
	Direction  string //`json:"direction"`
	CIDR       string
}

// SpiderSecurityRuleInfo is a struct to create JSON body of 'Create security group request'
type SpiderSecurityInfo struct { // Spider
	// Fields for request
	Name    string
	VPCName string

	// Fields for both request and response
	SecurityRules *[]SpiderSecurityRuleInfo

	// Fields for response
	IId          common.IID // {NameId, SystemId}
	VpcIID       common.IID // {NameId, SystemId}
	Direction    string     // @todo userd??
	KeyValueList []common.KeyValue
}

// TbSecurityGroupReq is a struct to handle 'Create security group' request toward CB-Tumblebug.
type TbSecurityGroupReq struct { // Tumblebug
	Name           string                    `json:"name" validate:"required"`
	ConnectionName string                    `json:"connectionName" validate:"required"`
	VNetId         string                    `json:"vNetId" validate:"required"`
	Description    string                    `json:"description"`
	FirewallRules  *[]SpiderSecurityRuleInfo `json:"firewallRules" validate:"required"`
}

// TbSecurityGroupReqStructLevelValidation is a function to validate 'TbSecurityGroupReq' object.
func TbSecurityGroupReqStructLevelValidation(sl validator.StructLevel) {

	u := sl.Current().Interface().(TbSecurityGroupReq)

	err := common.CheckString(u.Name)
	if err != nil {
		// ReportError(field interface{}, fieldName, structFieldName, tag, param string)
		sl.ReportError(u.Name, "name", "Name", err.Error(), "")
	}
}

// TbSecurityGroupInfo is a struct that represents TB security group object.
type TbSecurityGroupInfo struct { // Tumblebug
	Id                   string                    `json:"id"`
	Name                 string                    `json:"name"`
	ConnectionName       string                    `json:"connectionName"`
	VNetId               string                    `json:"vNetId"`
	Description          string                    `json:"description"`
	FirewallRules        *[]SpiderSecurityRuleInfo `json:"firewallRules"`
	CspSecurityGroupId   string                    `json:"cspSecurityGroupId"`
	CspSecurityGroupName string                    `json:"cspSecurityGroupName"`
	KeyValueList         []common.KeyValue         `json:"keyValueList"`
	AssociatedObjectList []string                  `json:"associatedObjectList"`
	IsAutoGenerated      bool                      `json:"isAutoGenerated"`

	// SystemLabel is for describing the MCIR in a keyword (any string can be used) for special System purpose
	SystemLabel string `json:"systemLabel" example:"Managed by CB-Tumblebug" default:""`

	// Disabled for now
	//ResourceGroupName  string `json:"resourceGroupName"`
}

// CreateSecurityGroup accepts SG creation request, creates and returns an TB SG object
func CreateSecurityGroup(nsId string, u *TbSecurityGroupReq) (TbSecurityGroupInfo, error) {

	resourceType := common.StrSecurityGroup

	err := common.CheckString(nsId)
	if err != nil {
		temp := TbSecurityGroupInfo{}
		common.CBLog.Error(err)
		return temp, err
	}

	// returns InvalidValidationError for bad validation input, nil or ValidationErrors ( []FieldError )
	err = validate.Struct(u)
	if err != nil {

		// this check is only needed when your code could produce
		// an invalid value for validation such as interface with nil
		// value most including myself do not usually have code like this.
		if _, ok := err.(*validator.InvalidValidationError); ok {
			fmt.Println(err)
			temp := TbSecurityGroupInfo{}
			return temp, err
		}

		// for _, err := range err.(validator.ValidationErrors) {

		// 	fmt.Println(err.Namespace()) // can differ when a custom TagNameFunc is registered or
		// 	fmt.Println(err.Field())     // by passing alt name to ReportError like below
		// 	fmt.Println(err.StructNamespace())
		// 	fmt.Println(err.StructField())
		// 	fmt.Println(err.Tag())
		// 	fmt.Println(err.ActualTag())
		// 	fmt.Println(err.Kind())
		// 	fmt.Println(err.Type())
		// 	fmt.Println(err.Value())
		// 	fmt.Println(err.Param())
		// 	fmt.Println()
		// }

		temp := TbSecurityGroupInfo{}
		return temp, err
	}

	check, err := CheckResource(nsId, resourceType, u.Name)

	if check {
		temp := TbSecurityGroupInfo{}
		err := fmt.Errorf("The securityGroup " + u.Name + " already exists.")
		return temp, err
	}
	if err != nil {
		common.CBLog.Error(err)
		content := TbSecurityGroupInfo{}
		err := fmt.Errorf("Cannot create securityGroup")
		return content, err
	}

	tempInterface, err := GetResource(nsId, common.StrVNet, u.VNetId)
	if err != nil {
		err := fmt.Errorf("Failed to get the TbVNetInfo " + u.VNetId + ".")
		return TbSecurityGroupInfo{}, err
	}
	vNetInfo := TbVNetInfo{}
	err = common.CopySrcToDest(&tempInterface, &vNetInfo)
	if err != nil {
		err := fmt.Errorf("Failed to get the TbVNetInfo-CopySrcToDest() " + u.VNetId + ".")
		return TbSecurityGroupInfo{}, err
	}

	tempReq := SpiderSecurityReqInfoWrapper{}
	tempReq.ConnectionName = u.ConnectionName
	tempReq.ReqInfo.Name = u.Name
	tempReq.ReqInfo.VPCName = vNetInfo.CspVNetName
	tempReq.ReqInfo.SecurityRules = u.FirewallRules

	var tempSpiderSecurityInfo *SpiderSecurityInfo

	if os.Getenv("SPIDER_CALL_METHOD") == "REST" {

		url := common.SpiderRestUrl + "/securitygroup"

		client := resty.New().SetCloseConnection(true)

		resp, err := client.R().
			SetHeader("Content-Type", "application/json").
			SetBody(tempReq).
			SetResult(&SpiderSecurityInfo{}). // or SetResult(AuthSuccess{}).
			//SetError(&AuthError{}).       // or SetError(AuthError{}).
			Post(url)

		if err != nil {
			common.CBLog.Error(err)
			content := TbSecurityGroupInfo{}
			err := fmt.Errorf("an error occurred while requesting to CB-Spider")
			return content, err
		}

		fmt.Println("HTTP Status code: " + strconv.Itoa(resp.StatusCode()))
		switch {
		case resp.StatusCode() >= 400 || resp.StatusCode() < 200:
			err := fmt.Errorf(string(resp.Body()))
			common.CBLog.Error(err)
			content := TbSecurityGroupInfo{}
			return content, err
		}

		tempSpiderSecurityInfo = resp.Result().(*SpiderSecurityInfo)

	} else {

		// Set CCM gRPC API
		ccm := api.NewCloudResourceHandler()
		err := ccm.SetConfigPath(os.Getenv("CBTUMBLEBUG_ROOT") + "/conf/grpc_conf.yaml")
		if err != nil {
			common.CBLog.Error("ccm failed to set config : ", err)
			return TbSecurityGroupInfo{}, err
		}
		err = ccm.Open()
		if err != nil {
			common.CBLog.Error("ccm api open failed : ", err)
			return TbSecurityGroupInfo{}, err
		}
		defer ccm.Close()

		payload, _ := json.Marshal(tempReq)
		fmt.Println("payload: " + string(payload)) // for debug

		result, err := ccm.CreateSecurity(string(payload))
		if err != nil {
			common.CBLog.Error(err)
			return TbSecurityGroupInfo{}, err
		}

		tempSpiderSecurityInfo = &SpiderSecurityInfo{}
		err = json.Unmarshal([]byte(result), &tempSpiderSecurityInfo)
		if err != nil {
			common.CBLog.Error(err)
			return TbSecurityGroupInfo{}, err
		}
	}

	content := TbSecurityGroupInfo{}
	content.Id = u.Name
	content.Name = u.Name
	content.ConnectionName = u.ConnectionName
	content.VNetId = tempSpiderSecurityInfo.VpcIID.NameId
	content.CspSecurityGroupId = tempSpiderSecurityInfo.IId.SystemId
	content.CspSecurityGroupName = tempSpiderSecurityInfo.IId.NameId
	content.Description = u.Description
	content.FirewallRules = tempSpiderSecurityInfo.SecurityRules
	content.KeyValueList = tempSpiderSecurityInfo.KeyValueList
	content.AssociatedObjectList = []string{}

	// cb-store
	fmt.Println("=========================== PUT CreateSecurityGroup")
	Key := common.GenResourceKey(nsId, resourceType, content.Id)
	Val, _ := json.Marshal(content)
	err = common.CBStore.Put(Key, string(Val))
	if err != nil {
		common.CBLog.Error(err)
		return content, err
	}
	keyValue, err := common.CBStore.Get(Key)
	if err != nil {
		common.CBLog.Error(err)
		err = fmt.Errorf("In CreateSecurityGroup(); CBStore.Get() returned an error.")
		common.CBLog.Error(err)
		// return nil, err
	}

	fmt.Println("<" + keyValue.Key + "> \n" + keyValue.Value)
	fmt.Println("===========================")
	return content, nil
}

type TbSecurityGroupRegReq struct {
	Name           string `json:"name" validate:"required"`
	ConnectionName string `json:"connectionName" validate:"required"`
	VNetId         string `json:"vNetId" validate:"required"`
	Description    string `json:"description"`
}

// RegisterSecurityGroup accepts SG registration request, registers and returns an TB SG object
func RegisterSecurityGroup(nsId string, u *TbSecurityGroupRegReq) (TbSecurityGroupInfo, error) {

	resourceType := common.StrSecurityGroup

	err := common.CheckString(nsId)
	if err != nil {
		temp := TbSecurityGroupInfo{}
		common.CBLog.Error(err)
		return temp, err
	}

	check, err := CheckResource(nsId, resourceType, u.Name)

	if check {
		temp := TbSecurityGroupInfo{}
		err := fmt.Errorf("The securityGroup " + u.Name + " already exists.")
		return temp, err
	}
	if err != nil {
		common.CBLog.Error(err)
		content := TbSecurityGroupInfo{}
		err := fmt.Errorf("in RegisterSecurityGroup(); Error occurred while checking the existence of SG")
		return content, err
	}

	err = validate.Struct(u)
	if err != nil {
		if _, ok := err.(*validator.InvalidValidationError); ok {
			fmt.Println(err)
			temp := TbSecurityGroupInfo{}
			return temp, err
		}

		temp := TbSecurityGroupInfo{}
		return temp, err
	}

	tempInterface, err := GetResource(nsId, common.StrVNet, u.VNetId)
	if err != nil {
		err := fmt.Errorf("Failed to get the TbVNetInfo " + u.VNetId + ".")
		return TbSecurityGroupInfo{}, err
	}
	vNetInfo := TbVNetInfo{}
	err = common.CopySrcToDest(&tempInterface, &vNetInfo)
	if err != nil {
		err := fmt.Errorf("Failed to get the TbVNetInfo-CopySrcToDest() " + u.VNetId + ".")
		return TbSecurityGroupInfo{}, err
	}

	tempReq := SpiderSecurityReqInfoWrapper{}
	tempReq.ConnectionName = u.ConnectionName

	var tempSpiderSecurityInfo *SpiderSecurityInfo

	if os.Getenv("SPIDER_CALL_METHOD") == "REST" {

		client := resty.New().SetCloseConnection(true)
		client.SetAllowGetMethodPayload(true)

		url := fmt.Sprintf("%s/securitygroup/%s", common.SpiderRestUrl, u.Name)

		resp, err := client.R().
			SetHeader("Content-Type", "application/json").
			SetBody(tempReq).
			SetResult(&SpiderSecurityInfo{}). // or SetResult(AuthSuccess{}).
			//SetError(&AuthError{}).       // or SetError(AuthError{}).
			Get(url)

		if err != nil {
			common.CBLog.Error(err)
			content := TbSecurityGroupInfo{}
			err := fmt.Errorf("an error occurred while requesting to CB-Spider")
			return content, err
		}

		fmt.Println("HTTP Status code: " + strconv.Itoa(resp.StatusCode()))
		switch {
		case resp.StatusCode() >= 400 || resp.StatusCode() < 200:
			err := fmt.Errorf(string(resp.Body()))
			common.CBLog.Error(err)
			content := TbSecurityGroupInfo{}
			return content, err
		}

		tempSpiderSecurityInfo = resp.Result().(*SpiderSecurityInfo)

	} else {

		// Set CCM gRPC API
		ccm := api.NewCloudResourceHandler()
		err := ccm.SetConfigPath(os.Getenv("CBTUMBLEBUG_ROOT") + "/conf/grpc_conf.yaml")
		if err != nil {
			common.CBLog.Error("ccm failed to set config : ", err)
			return TbSecurityGroupInfo{}, err
		}
		err = ccm.Open()
		if err != nil {
			common.CBLog.Error("ccm api open failed : ", err)
			return TbSecurityGroupInfo{}, err
		}
		defer ccm.Close()

		payload, _ := json.Marshal(tempReq)
		fmt.Println("payload: " + string(payload)) // for debug

		result, err := ccm.GetSecurity(string(payload))

		if err != nil {
			common.CBLog.Error(err)
			return TbSecurityGroupInfo{}, err
		}

		tempSpiderSecurityInfo = &SpiderSecurityInfo{}
		err = json.Unmarshal([]byte(result), &tempSpiderSecurityInfo)
		if err != nil {
			common.CBLog.Error(err)
			return TbSecurityGroupInfo{}, err
		}
	}

	content := TbSecurityGroupInfo{}
	content.Id = u.Name
	content.Name = u.Name
	content.ConnectionName = u.ConnectionName
	content.VNetId = tempSpiderSecurityInfo.VpcIID.NameId
	content.CspSecurityGroupId = tempSpiderSecurityInfo.IId.SystemId
	content.CspSecurityGroupName = tempSpiderSecurityInfo.IId.NameId
	content.Description = u.Description
	content.FirewallRules = tempSpiderSecurityInfo.SecurityRules
	content.KeyValueList = tempSpiderSecurityInfo.KeyValueList
	content.AssociatedObjectList = []string{}
	content.SystemLabel = "Registered from CSP resource"

	// cb-store
	fmt.Println("=========================== PUT RegisterSecurityGroup")
	Key := common.GenResourceKey(nsId, resourceType, content.Id)
	Val, _ := json.Marshal(content)
	err = common.CBStore.Put(Key, string(Val))
	if err != nil {
		common.CBLog.Error(err)
		return content, err
	}
	keyValue, err := common.CBStore.Get(Key)
	if err != nil {
		common.CBLog.Error(err)
		err = fmt.Errorf("In RegisterSecurityGroup(); CBStore.Get() returned an error.")
		common.CBLog.Error(err)
		// return nil, err
	}

	fmt.Println("<" + keyValue.Key + "> \n" + keyValue.Value)
	fmt.Println("===========================")
	return content, nil
}
