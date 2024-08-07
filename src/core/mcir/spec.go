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
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cloud-barista/cb-tumblebug/src/core/common"
	"github.com/cloud-barista/cb-tumblebug/src/kvstore/kvstore"
	validator "github.com/go-playground/validator/v10"
	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"

	//"github.com/cloud-barista/cb-tumblebug/src/core/mcis"

	_ "github.com/go-sql-driver/mysql"
)

// SpiderSpecInfo is a struct to create JSON body of 'Get spec request'
type SpiderSpecInfo struct {
	// https://github.com/cloud-barista/cb-spider/blob/master/cloud-control-manager/cloud-driver/interfaces/resources/VMSpecHandler.go

	Region string
	Name   string
	VCpu   SpiderVCpuInfo
	Mem    string
	Gpu    []SpiderGpuInfo

	KeyValueList []common.KeyValue
}

// SpiderVCpuInfo is a struct to handle vCPU Info from CB-Spider.
type SpiderVCpuInfo struct {
	Count string
	Clock string // GHz
}

// SpiderGpuInfo is a struct to handle GPU Info from CB-Spider.
type SpiderGpuInfo struct {
	Count string
	Mfr   string
	Model string
	Mem   string
}

// TbSpecReq is a struct to handle 'Register spec' request toward CB-Tumblebug.
type TbSpecReq struct { // Tumblebug
	Name           string `json:"name" validate:"required"`
	ConnectionName string `json:"connectionName" validate:"required"`
	CspSpecName    string `json:"cspSpecName" validate:"required"`
	Description    string `json:"description"`
}

// TbSpecReqStructLevelValidation is a function to validate 'TbSpecReq' object.
func TbSpecReqStructLevelValidation(sl validator.StructLevel) {

	u := sl.Current().Interface().(TbSpecReq)

	err := common.CheckString(u.Name)
	if err != nil {
		// ReportError(field interface{}, fieldName, structFieldName, tag, param string)
		sl.ReportError(u.Name, "name", "Name", err.Error(), "")
	}
}

// TbSpecInfo is a struct that represents TB spec object.
type TbSpecInfo struct { // Tumblebug
	Namespace             string   `json:"namespace,omitempty"` // required to save in RDB
	Id                    string   `json:"id,omitempty"`
	Name                  string   `json:"name,omitempty"`
	ConnectionName        string   `json:"connectionName,omitempty"`
	ProviderName          string   `json:"providerName,omitempty"`
	RegionName            string   `json:"regionName,omitempty"`
	CspSpecName           string   `json:"cspSpecName,omitempty"`
	InfraType             string   `json:"infraType,omitempty"` // vm|k8s|kubernetes|container, etc.
	OsType                string   `json:"osType,omitempty"`
	VCPU                  uint16   `json:"vCPU,omitempty"`
	MemoryGiB             float32  `json:"memoryGiB,omitempty"`
	StorageGiB            uint32   `json:"storageGiB,omitempty"`
	MaxTotalStorageTiB    uint16   `json:"maxTotalStorageTiB,omitempty"`
	NetBwGbps             uint16   `json:"netBwGbps,omitempty"`
	AcceleratorModel      string   `json:"acceleratorModel,omitempty"`
	AcceleratorCount      uint8    `json:"acceleratorCount,omitempty"`
	AcceleratorMemoryGB   float32  `json:"acceleratorMemoryGB,omitempty"`
	AcceleratorType       string   `json:"acceleratorType,omitempty"`
	CostPerHour           float32  `json:"costPerHour,omitempty"`
	Description           string   `json:"description,omitempty"`
	OrderInFilteredResult uint16   `json:"orderInFilteredResult,omitempty"`
	EvaluationStatus      string   `json:"evaluationStatus,omitempty"`
	EvaluationScore01     float32  `json:"evaluationScore01"`
	EvaluationScore02     float32  `json:"evaluationScore02"`
	EvaluationScore03     float32  `json:"evaluationScore03"`
	EvaluationScore04     float32  `json:"evaluationScore04"`
	EvaluationScore05     float32  `json:"evaluationScore05"`
	EvaluationScore06     float32  `json:"evaluationScore06"`
	EvaluationScore07     float32  `json:"evaluationScore07"`
	EvaluationScore08     float32  `json:"evaluationScore08"`
	EvaluationScore09     float32  `json:"evaluationScore09"`
	EvaluationScore10     float32  `json:"evaluationScore10"`
	RootDiskType          string   `json:"rootDiskType"`
	RootDiskSize          string   `json:"rootDiskSize"`
	AssociatedObjectList  []string `json:"associatedObjectList,omitempty"`
	IsAutoGenerated       bool     `json:"isAutoGenerated,omitempty"`

	// SystemLabel is for describing the MCIR in a keyword (any string can be used) for special System purpose
	SystemLabel string `json:"systemLabel,omitempty" example:"Managed by CB-Tumblebug" default:""`
}

// FilterSpecsByRangeRequest is for 'FilterSpecsByRange'
type FilterSpecsByRangeRequest struct {
	Id                  string `json:"id"`
	Name                string `json:"name"`
	ConnectionName      string `json:"connectionName"`
	ProviderName        string `json:"providerName"`
	RegionName          string `json:"regionName"`
	CspSpecName         string `json:"cspSpecName"`
	InfraType           string `json:"infraType"`
	OsType              string `json:"osType"`
	VCPU                Range  `json:"vCPU"`
	MemoryGiB           Range  `json:"memoryGiB"`
	StorageGiB          Range  `json:"storageGiB"`
	MaxTotalStorageTiB  Range  `json:"maxTotalStorageTiB"`
	NetBwGbps           Range  `json:"netBwGbps"`
	AcceleratorModel    string `json:"acceleratorModel"`
	AcceleratorCount    Range  `json:"acceleratorCount"`
	AcceleratorMemoryGB Range  `json:"acceleratorMemoryGB"`
	AcceleratorType     string `json:"acceleratorType"`
	CostPerHour         Range  `json:"costPerHour"`
	Description         string `json:"description"`
	EvaluationStatus    string `json:"evaluationStatus"`
	EvaluationScore01   Range  `json:"evaluationScore01"`
	EvaluationScore02   Range  `json:"evaluationScore02"`
	EvaluationScore03   Range  `json:"evaluationScore03"`
	EvaluationScore04   Range  `json:"evaluationScore04"`
	EvaluationScore05   Range  `json:"evaluationScore05"`
	EvaluationScore06   Range  `json:"evaluationScore06"`
	EvaluationScore07   Range  `json:"evaluationScore07"`
	EvaluationScore08   Range  `json:"evaluationScore08"`
	EvaluationScore09   Range  `json:"evaluationScore09"`
	EvaluationScore10   Range  `json:"evaluationScore10"`
}

// ConvertSpiderSpecToTumblebugSpec accepts an Spider spec object, converts to and returns an TB spec object
func ConvertSpiderSpecToTumblebugSpec(spiderSpec SpiderSpecInfo) (TbSpecInfo, error) {
	if spiderSpec.Name == "" {
		err := fmt.Errorf("ConvertSpiderSpecToTumblebugSpec failed; spiderSpec.Name == \"\" ")
		emptyTumblebugSpec := TbSpecInfo{}
		return emptyTumblebugSpec, err
	}

	tumblebugSpec := TbSpecInfo{}

	tumblebugSpec.Name = spiderSpec.Name
	tumblebugSpec.CspSpecName = spiderSpec.Name
	tumblebugSpec.RegionName = spiderSpec.Region
	tempUint64, _ := strconv.ParseUint(spiderSpec.VCpu.Count, 10, 16)
	tumblebugSpec.VCPU = uint16(tempUint64)
	tempFloat64, _ := strconv.ParseFloat(spiderSpec.Mem, 32)
	tumblebugSpec.MemoryGiB = float32(tempFloat64 / 1024)

	return tumblebugSpec, nil
}

// SpiderSpecList is a struct to handle spec list from the CB-Spider's REST API response
type SpiderSpecList struct {
	Vmspec []SpiderSpecInfo `json:"vmspec"`
}

// LookupSpecList accepts Spider conn config,
// lookups and returns the list of all specs in the region of conn config
// in the form of the list of Spider spec objects
func LookupSpecList(connConfig string) (SpiderSpecList, error) {

	if connConfig == "" {
		content := SpiderSpecList{}
		err := fmt.Errorf("LookupSpec called with empty connConfig.")
		log.Error().Err(err).Msg("")
		return content, err
	}

	var callResult SpiderSpecList
	client := resty.New()
	client.SetTimeout(10 * time.Minute)
	url := common.SpiderRestUrl + "/vmspec"
	method := "GET"
	requestBody := common.SpiderConnectionName{}
	requestBody.ConnectionName = connConfig

	err := common.ExecuteHttpRequest(
		client,
		method,
		url,
		nil,
		common.SetUseBody(requestBody),
		&requestBody,
		&callResult,
		common.MediumDuration,
	)

	if err != nil {
		log.Trace().Err(err).Msg("")
		content := SpiderSpecList{}
		return content, err
	}

	temp := callResult
	return temp, nil

}

// LookupSpec accepts Spider conn config and CSP spec name, lookups and returns the Spider spec object
func LookupSpec(connConfig string, specName string) (SpiderSpecInfo, error) {

	if connConfig == "" {
		content := SpiderSpecInfo{}
		err := fmt.Errorf("LookupSpec() called with empty connConfig.")
		log.Error().Err(err).Msg("")
		return content, err
	} else if specName == "" {
		content := SpiderSpecInfo{}
		err := fmt.Errorf("LookupSpec() called with empty specName.")
		log.Error().Err(err).Msg("")
		return content, err
	}

	client := resty.New()
	client.SetTimeout(2 * time.Minute)
	url := common.SpiderRestUrl + "/vmspec/" + specName
	method := "GET"
	requestBody := common.SpiderConnectionName{}
	requestBody.ConnectionName = connConfig
	callResult := SpiderSpecInfo{}

	err := common.ExecuteHttpRequest(
		client,
		method,
		url,
		nil,
		common.SetUseBody(requestBody),
		&requestBody,
		&callResult,
		common.MediumDuration,
	)

	if err != nil {
		log.Error().Err(err).Msg("")
		return callResult, err
	}

	return callResult, nil
}

// FetchSpecsForConnConfig lookups all specs for region of conn config, and saves into TB spec objects
func FetchSpecsForConnConfig(connConfig string, nsId string) (specCount uint, err error) {
	log.Debug().Msg("FetchSpecsForConnConfig(" + connConfig + ")")

	spiderSpecList, err := LookupSpecList(connConfig)
	if err != nil {
		log.Error().Err(err).Msg("")
		return 0, err
	}

	for _, spiderSpec := range spiderSpecList.Vmspec {
		tumblebugSpec, err := ConvertSpiderSpecToTumblebugSpec(spiderSpec)
		if err != nil {
			log.Error().Err(err).Msg("")
			return 0, err
		}

		tumblebugSpecId := connConfig + "-" + ToNamingRuleCompatible(tumblebugSpec.Name)

		check, err := CheckResource(nsId, common.StrSpec, tumblebugSpecId)
		if check {
			log.Info().Msgf("The spec %s already exists in TB; continue", tumblebugSpecId)
			continue
		} else if err != nil {
			log.Info().Msgf("Cannot check the existence of %s in TB; continue", tumblebugSpecId)
			continue
		} else {
			tumblebugSpec.Name = tumblebugSpecId
			tumblebugSpec.ConnectionName = connConfig

			_, err := RegisterSpecWithInfo(nsId, &tumblebugSpec, true)
			if err != nil {
				log.Error().Err(err).Msg("")
				return 0, err
			}
			specCount++
		}
	}
	return specCount, nil
}

// FetchSpecsForAllConnConfigs gets all conn configs from Spider, lookups all specs for each region of conn config, and saves into TB spec objects
func FetchSpecsForAllConnConfigs(nsId string) (connConfigCount uint, specCount uint, err error) {

	err = common.CheckString(nsId)
	if err != nil {
		log.Error().Err(err).Msg("")
		return 0, 0, err
	}

	connConfigs, err := common.GetConnConfigList(common.DefaultCredentialHolder, true, true)
	if err != nil {
		log.Error().Err(err).Msg("")
		return 0, 0, err
	}

	for _, connConfig := range connConfigs.Connectionconfig {
		temp, _ := FetchSpecsForConnConfig(connConfig.ConfigName, nsId)
		specCount += temp
		connConfigCount++
	}
	return connConfigCount, specCount, nil
}

// RegisterSpecWithCspSpecName accepts spec creation request, creates and returns an TB spec object
func RegisterSpecWithCspSpecName(nsId string, u *TbSpecReq, update bool) (TbSpecInfo, error) {

	resourceType := common.StrSpec
	content := TbSpecInfo{}

	err := common.CheckString(nsId)
	if err != nil {
		log.Error().Err(err).Msg("")
		return content, err
	}

	err = validate.Struct(u)
	if err != nil {
		if _, ok := err.(*validator.InvalidValidationError); ok {
			log.Err(err).Msg("")
			return content, err
		}
		return content, err
	}

	check, err := CheckResource(nsId, resourceType, u.Name)

	if err != nil {
		log.Error().Err(err).Msg("")
		return content, err
	}

	if !update {
		if check {
			err := fmt.Errorf("The spec " + u.Name + " already exists.")
			return content, err
		}
	}

	res, err := LookupSpec(u.ConnectionName, u.CspSpecName)
	if err != nil {
		log.Error().Err(err).Msgf("Cannot LookupSpec ConnectionName(%s), CspSpecName(%s)", u.ConnectionName, u.CspSpecName)
		return content, err
	}

	content.Namespace = nsId
	content.Id = u.Name
	content.Name = u.Name
	content.CspSpecName = res.Name
	content.ConnectionName = u.ConnectionName
	content.AssociatedObjectList = []string{}

	tempUint64, _ := strconv.ParseUint(res.VCpu.Count, 10, 16)
	content.VCPU = uint16(tempUint64)

	//content.Num_core = res.Num_core

	tempFloat64, _ := strconv.ParseFloat(res.Mem, 32)
	content.MemoryGiB = float32(tempFloat64 / 1024)

	//content.StorageGiB = res.StorageGiB
	//content.Description = res.Description

	log.Trace().Msg("PUT registerSpec")
	Key := common.GenResourceKey(nsId, resourceType, content.Id)
	Val, _ := json.Marshal(content)
	err = kvstore.Put(Key, string(Val))
	if err != nil {
		log.Error().Err(err).Msg("Cannot put data to Key Value Store")
		return content, err
	}

	// "INSERT INTO `spec`(`namespace`, `id`, ...) VALUES ('nsId', 'content.Id', ...);
	_, err = common.ORM.Insert(&content)
	if err != nil {
		log.Error().Err(err).Msg("Cannot insert data to RDB")
	} else {
		log.Trace().Msg("SQL: Insert success")
	}

	return content, nil
}

// RegisterSpecWithInfo accepts spec creation request, creates and returns an TB spec object
func RegisterSpecWithInfo(nsId string, content *TbSpecInfo, update bool) (TbSpecInfo, error) {

	resourceType := common.StrSpec

	err := common.CheckString(nsId)
	if err != nil {
		temp := TbSpecInfo{}
		log.Error().Err(err).Msg("")
		return temp, err
	}
	// err = common.CheckString(content.Name)
	// if err != nil {
	// 	temp := TbSpecInfo{}
	// 	log.Error().Err(err).Msg("")
	// 	return temp, err
	// }
	check, err := CheckResource(nsId, resourceType, content.Name)

	if err != nil {
		temp := TbSpecInfo{}
		log.Error().Err(err).Msg("")
		return temp, err
	}

	if !update {
		if check {
			temp := TbSpecInfo{}
			err := fmt.Errorf("The spec " + content.Name + " already exists.")
			return temp, err
		}
	}

	content.Namespace = nsId
	content.Id = content.Name
	content.AssociatedObjectList = []string{}

	log.Trace().Msg("PUT registerSpec")
	Key := common.GenResourceKey(nsId, resourceType, content.Id)
	Val, _ := json.Marshal(content)
	err = kvstore.Put(Key, string(Val))
	if err != nil {
		log.Error().Err(err).Msg("")
		return *content, err
	}

	// "INSERT INTO `spec`(`namespace`, `id`, ...) VALUES ('nsId', 'content.Id', ...);
	_, err = common.ORM.Insert(content)
	if err != nil {
		log.Error().Err(err).Msg("")
	} else {
		log.Trace().Msg("SQL: Insert success")
	}

	return *content, nil
}

// Range struct is for 'FilterSpecsByRange'
type Range struct {
	Min float32 `json:"min"`
	Max float32 `json:"max"`
}

// FilterSpecsByRange accepts criteria ranges for filtering, and returns the list of filtered TB spec objects
func FilterSpecsByRange(nsId string, filter FilterSpecsByRangeRequest) ([]TbSpecInfo, error) {
	if err := common.CheckString(nsId); err != nil {
		log.Error().Err(err).Msg("Invalid namespace ID")
		return nil, err
	}

	// Start building the query using field names as database column names
	session := common.ORM.Where("Namespace = ?", nsId)

	// Use reflection to iterate over filter struct
	val := reflect.ValueOf(filter)
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		value := val.Field(i)

		// Convert the first letter of the field name to lowercase to match typical database column naming conventions
		dbFieldName := strings.ToLower(field.Name[:1]) + field.Name[1:]
		log.Debug().Msgf("Field: %s, Value: %v", dbFieldName, value)

		if value.Kind() == reflect.Struct {
			// Handle range filters like VCPU, MemoryGiB, etc.
			min := value.FieldByName("Min")
			max := value.FieldByName("Max")

			if min.IsValid() && !min.IsZero() {
				session = session.And(dbFieldName+" >= ?", min.Interface())
			}
			if max.IsValid() && !max.IsZero() {
				session = session.And(dbFieldName+" <= ?", max.Interface())
			}
		} else if value.IsValid() && !value.IsZero() {
			switch value.Kind() {
			case reflect.String:
				cleanValue := ToNamingRuleCompatible(value.String())
				session = session.And(dbFieldName+" LIKE ?", "%"+cleanValue+"%")
				log.Info().Msgf("Filtering by %s: %s", dbFieldName, cleanValue)
			}
		}
	}

	var specs []TbSpecInfo
	err := session.Find(&specs)
	if err != nil {
		log.Error().Err(err).Msg("Failed to execute query")
		return nil, err
	}

	return specs, nil
}

// SortSpecs accepts the list of TB spec objects, criteria and sorting direction,
// sorts and returns the sorted list of TB spec objects
func SortSpecs(specList []TbSpecInfo, orderBy string, direction string) ([]TbSpecInfo, error) {
	var err error = nil

	sort.Slice(specList, func(i, j int) bool {
		if orderBy == "vCPU" {
			if direction == "descending" {
				return specList[i].VCPU > specList[j].VCPU
			} else if direction == "ascending" {
				return specList[i].VCPU < specList[j].VCPU
			} else {
				err = fmt.Errorf("'direction' should one of these: ascending, descending")
				return true
			}
		} else if orderBy == "memoryGiB" {
			if direction == "descending" {
				return specList[i].MemoryGiB > specList[j].MemoryGiB
			} else if direction == "ascending" {
				return specList[i].MemoryGiB < specList[j].MemoryGiB
			} else {
				err = fmt.Errorf("'direction' should one of these: ascending, descending")
				return true
			}
		} else if orderBy == "storageGiB" {
			if direction == "descending" {
				return specList[i].StorageGiB > specList[j].StorageGiB
			} else if direction == "ascending" {
				return specList[i].StorageGiB < specList[j].StorageGiB
			} else {
				err = fmt.Errorf("'direction' should one of these: ascending, descending")
				return true
			}
		} else if orderBy == "evaluationScore01" {
			if direction == "descending" {
				return specList[i].EvaluationScore01 > specList[j].EvaluationScore01
			} else if direction == "ascending" {
				return specList[i].EvaluationScore01 < specList[j].EvaluationScore01
			} else {
				err = fmt.Errorf("'direction' should one of these: ascending, descending")
				return true
			}
		} else if orderBy == "evaluationScore02" {
			if direction == "descending" {
				return specList[i].EvaluationScore02 > specList[j].EvaluationScore02
			} else if direction == "ascending" {
				return specList[i].EvaluationScore02 < specList[j].EvaluationScore02
			} else {
				err = fmt.Errorf("'direction' should one of these: ascending, descending")
				return true
			}
		} else if orderBy == "evaluationScore03" {
			if direction == "descending" {
				return specList[i].EvaluationScore03 > specList[j].EvaluationScore03
			} else if direction == "ascending" {
				return specList[i].EvaluationScore03 < specList[j].EvaluationScore03
			} else {
				err = fmt.Errorf("'direction' should one of these: ascending, descending")
				return true
			}
		} else if orderBy == "evaluationScore04" {
			if direction == "descending" {
				return specList[i].EvaluationScore04 > specList[j].EvaluationScore04
			} else if direction == "ascending" {
				return specList[i].EvaluationScore04 < specList[j].EvaluationScore04
			} else {
				err = fmt.Errorf("'direction' should one of these: ascending, descending")
				return true
			}
		} else if orderBy == "evaluationScore05" {
			if direction == "descending" {
				return specList[i].EvaluationScore05 > specList[j].EvaluationScore05
			} else if direction == "ascending" {
				return specList[i].EvaluationScore05 < specList[j].EvaluationScore05
			} else {
				err = fmt.Errorf("'direction' should one of these: ascending, descending")
				return true
			}
		} else if orderBy == "evaluationScore06" {
			if direction == "descending" {
				return specList[i].EvaluationScore06 > specList[j].EvaluationScore06
			} else if direction == "ascending" {
				return specList[i].EvaluationScore06 < specList[j].EvaluationScore06
			} else {
				err = fmt.Errorf("'direction' should one of these: ascending, descending")
				return true
			}
		} else if orderBy == "evaluationScore07" {
			if direction == "descending" {
				return specList[i].EvaluationScore07 > specList[j].EvaluationScore07
			} else if direction == "ascending" {
				return specList[i].EvaluationScore07 < specList[j].EvaluationScore07
			} else {
				err = fmt.Errorf("'direction' should one of these: ascending, descending")
				return true
			}
		} else if orderBy == "evaluationScore08" {
			if direction == "descending" {
				return specList[i].EvaluationScore08 > specList[j].EvaluationScore08
			} else if direction == "ascending" {
				return specList[i].EvaluationScore08 < specList[j].EvaluationScore08
			} else {
				err = fmt.Errorf("'direction' should one of these: ascending, descending")
				return true
			}
		} else if orderBy == "evaluationScore09" {
			if direction == "descending" {
				return specList[i].EvaluationScore09 > specList[j].EvaluationScore09
			} else if direction == "ascending" {
				return specList[i].EvaluationScore09 < specList[j].EvaluationScore09
			} else {
				err = fmt.Errorf("'direction' should one of these: ascending, descending")
				return true
			}
		} else if orderBy == "evaluationScore10" {
			if direction == "descending" {
				return specList[i].EvaluationScore10 > specList[j].EvaluationScore10
			} else if direction == "ascending" {
				return specList[i].EvaluationScore10 < specList[j].EvaluationScore10
			} else {
				err = fmt.Errorf("'direction' should one of these: ascending, descending")
				return true
			}
		} else {
			err = fmt.Errorf("'orderBy' should one of these: vCPU, memoryGiB, storageGiB")
			return true
		}
	})

	for i := range specList {
		specList[i].OrderInFilteredResult = uint16(i + 1)
	}

	return specList, err
}

// UpdateSpec accepts to-be TB spec objects,
// updates and returns the updated TB spec objects
func UpdateSpec(nsId string, specId string, fieldsToUpdate TbSpecInfo) (TbSpecInfo, error) {
	resourceType := common.StrSpec

	err := common.CheckString(nsId)
	if err != nil {
		temp := TbSpecInfo{}
		log.Error().Err(err).Msg("")
		return temp, err
	}

	if len(fieldsToUpdate.Namespace) > 0 {
		temp := TbSpecInfo{}
		err := fmt.Errorf("You should not specify 'namespace' in the JSON request body.")
		log.Error().Err(err).Msg("")
		return temp, err
	}

	if len(fieldsToUpdate.Id) > 0 {
		temp := TbSpecInfo{}
		err := fmt.Errorf("You should not specify 'id' in the JSON request body.")
		log.Error().Err(err).Msg("")
		return temp, err
	}

	check, err := CheckResource(nsId, resourceType, specId)

	if err != nil {
		temp := TbSpecInfo{}
		log.Error().Err(err).Msg("")
		return temp, err
	}

	if !check {
		temp := TbSpecInfo{}
		err := fmt.Errorf("The spec " + specId + " does not exist.")
		return temp, err
	}

	tempInterface, err := GetResource(nsId, resourceType, specId)
	if err != nil {
		temp := TbSpecInfo{}
		err := fmt.Errorf("Failed to get the spec " + specId + ".")
		return temp, err
	}
	asIsSpec := TbSpecInfo{}
	err = common.CopySrcToDest(&tempInterface, &asIsSpec)
	if err != nil {
		temp := TbSpecInfo{}
		err := fmt.Errorf("Failed to CopySrcToDest() " + specId + ".")
		return temp, err
	}

	// Update specified fields only
	toBeSpec := asIsSpec
	toBeSpecJSON, _ := json.Marshal(fieldsToUpdate)
	err = json.Unmarshal(toBeSpecJSON, &toBeSpec)

	Key := common.GenResourceKey(nsId, resourceType, toBeSpec.Id)
	Val, _ := json.Marshal(toBeSpec)
	err = kvstore.Put(Key, string(Val))
	if err != nil {
		temp := TbSpecInfo{}
		log.Error().Err(err).Msg("")
		return temp, err
	}

	// "UPDATE `spec` SET `id`='" + specId + "', ... WHERE `namespace`='" + nsId + "' AND `id`='" + specId + "';"
	_, err = common.ORM.Update(&toBeSpec, &TbSpecInfo{Namespace: nsId, Id: specId})
	if err != nil {
		log.Error().Err(err).Msg("")
	} else {
		log.Trace().Msg("SQL: Update success")
	}

	return toBeSpec, nil
}
