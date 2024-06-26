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

// Package mcis is to manage multi-cloud infra service
package mcis

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	validator "github.com/go-playground/validator/v10"
	"github.com/rs/zerolog/log"
	"github.com/tidwall/gjson"

	"github.com/cloud-barista/cb-tumblebug/src/core/common"
)

const (
	monMetricAll     string = "all"
	monMetricCpu     string = "cpu"
	monMetricCpufreq string = "cpufreq"
	monMetricMem     string = "mem"
	monMetricNet     string = "net"
	monMetricSwap    string = "swap"
	monMetricDisk    string = "disk"
	monMetricDiskio  string = "diskio"
)

// MonAgentInstallReq struct
type MonAgentInstallReq struct {
	NsId     string `json:"nsId,omitempty"`
	McisId   string `json:"mcisId,omitempty"`
	VmId     string `json:"vmId,omitempty"`
	PublicIp string `json:"publicIp,omitempty"`
	Port     string `json:"port,omitempty"`
	UserName string `json:"userName,omitempty"`
	SshKey   string `json:"sshKey,omitempty"`
	CspType  string `json:"cspType,omitempty"`
}

func DFMonAgentInstallReqStructLevelValidation(sl validator.StructLevel) {

	u := sl.Current().Interface().(MonAgentInstallReq)

	err := common.CheckString(u.NsId)
	if err != nil {
		// ReportError(field interface{}, fieldName, structFieldName, tag, param string)
		sl.ReportError(u.NsId, "nsId", "NsId", err.Error(), "")
	}

	err = common.CheckString(u.McisId)
	if err != nil {
		// ReportError(field interface{}, fieldName, structFieldName, tag, param string)
		sl.ReportError(u.McisId, "mcisId", "McisId", err.Error(), "")
	}

	err = common.CheckString(u.VmId)
	if err != nil {
		// ReportError(field interface{}, fieldName, structFieldName, tag, param string)
		sl.ReportError(u.VmId, "vmId", "VmId", err.Error(), "")
	}
}

/*
type DfTelegrafMetric struct {
	Name      string                 `json:"name"`
	Tags      map[string]interface{} `json:"tags"`
	Fields    map[string]interface{} `json:"fields"`
	Timestamp int64                  `json:"timestamp"`
	TagInfo   map[string]interface{} `json:"tagInfo"`
}
*/

// MonResultSimple struct is for containing vm monitoring results
type MonResultSimple struct {
	Metric string `json:"metric"`
	VmId   string `json:"vmId"`
	Value  string `json:"value"`
	Err    string `json:"err"`
}

// MonResultSimpleResponse struct is for containing Mcis monitoring results
type MonResultSimpleResponse struct {
	NsId           string            `json:"nsId"`
	McisId         string            `json:"mcisId"`
	McisMonitoring []MonResultSimple `json:"mcisMonitoring"`
}

// Module for checking CB-Dragonfly endpoint (call get config)
func CheckDragonflyEndpoint() error {

	cmd := "/config"

	url := common.DragonflyRestUrl + cmd
	method := "GET"

	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)

	if err != nil {
		log.Err(err).Msg("")
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		log.Err(err).Msg("")
		return err
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Err(err).Msg("")
		return err
	}
	defer res.Body.Close()

	log.Debug().Msg(string(body))
	return nil

}

// monAgentInstallReq is struct for CB-Dragonfly monitoring agent installation request
type monAgentInstallReq struct {
	NsId        string `json:"ns_id"`
	McisId      string `json:"mcis_id"`
	VmId        string `json:"vm_id"`
	PublicIp    string `json:"public_ip"`
	UserName    string `json:"user_name"`
	SshKey      string `json:"ssh_key"`
	CspType     string `json:"cspType"`
	ServiceType string `json:"service_type"`
	Port        string `json:"port"`
}

// CallMonitoringAsync is func to call CB-Dragonfly monitoring framework
func CallMonitoringAsync(wg *sync.WaitGroup, nsID string, mcisID string, mcisServiceType string, vmID string, givenUserName string, method string, cmd string, returnResult *[]SshCmdResult) {

	defer wg.Done() //goroutin sync done

	vmIP, _, sshPort, err := GetVmIp(nsID, mcisID, vmID)
	errStr := ""
	if err != nil {
		log.Error().Err(err).Msg("")
		errStr += "/ " + err.Error()
	}
	userName, privateKey, err := VerifySshUserName(nsID, mcisID, vmID, vmIP, sshPort, givenUserName)
	if err != nil {
		log.Error().Err(err).Msg("")
		errStr += "/ " + err.Error()
	}
	log.Debug().Msg("[CallMonitoringAsync] " + mcisID + "/" + vmID + "(" + vmIP + ")" + "with userName:" + userName)

	// set vm MonAgentStatus = "installing" (to avoid duplicated requests)
	vmInfoTmp, _ := GetVmObject(nsID, mcisID, vmID)
	vmInfoTmp.MonAgentStatus = "installing"
	UpdateVmInfo(nsID, mcisID, vmInfoTmp)

	if mcisServiceType == "" {
		mcisServiceType = common.StrMCIS
	}

	url := common.DragonflyRestUrl + cmd
	log.Debug().Msg("\n[Calling DRAGONFLY] START")
	log.Debug().Msg("VM:" + nsID + "/" + mcisID + "/" + vmID + ", URL:" + url + ", userName:" + userName + ", cspType:" + vmInfoTmp.ConnectionConfig.ProviderName + ", service_type:" + mcisServiceType)

	requestBody := monAgentInstallReq{
		NsId:        nsID,
		McisId:      mcisID,
		VmId:        vmID,
		PublicIp:    vmIP,
		Port:        sshPort,
		UserName:    userName,
		SshKey:      privateKey,
		CspType:     vmInfoTmp.ConnectionConfig.ProviderName,
		ServiceType: mcisServiceType,
	}
	if requestBody.SshKey == "" {
		common.PrintJsonPretty(requestBody)
		err = fmt.Errorf("/request body to install monitoring agent: privateKey is empty/")
		log.Error().Err(err).Msg("")
		errStr += "/ " + err.Error()
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		log.Error().Err(err).Msg("")
		errStr += "/ " + err.Error()
	}

	responseLimit := 8
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: time.Duration(responseLimit) * time.Minute,
	}
	req, err := http.NewRequest(method, url, strings.NewReader(string(payload)))

	if err != nil {
		log.Error().Err(err).Msg("")
		errStr += "/ " + err.Error()
	}

	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)

	result := ""

	log.Debug().Msg("Called CB-DRAGONFLY API")
	if err != nil {
		log.Error().Err(err).Msg("")
		errStr += "/ " + err.Error()
	} else {

		body, err := io.ReadAll(res.Body)
		if err != nil {
			log.Error().Err(err).Msg("")
			errStr += "/ " + err.Error()
		}
		defer res.Body.Close()

		fmt.Println("HTTP Status code: " + strconv.Itoa(res.StatusCode))
		switch {
		case res.StatusCode >= 400 || res.StatusCode < 200:
			err = fmt.Errorf("CB-DF HTTP Status: " + strconv.Itoa(res.StatusCode) + " / " + string(body))
			log.Error().Err(err).Msg("")
			errStr += "/ " + err.Error()
		}

		result = string(body)
	}

	//wg.Done() //goroutin sync done

	//vmInfoTmp, _ := GetVmObject(nsID, mcisID, vmID)

	sshResultTmp := SshCmdResult{}
	sshResultTmp.McisId = mcisID
	sshResultTmp.VmId = vmID
	sshResultTmp.VmIp = vmIP

	sshResultTmp.Stdout = make(map[int]string)
	sshResultTmp.Stderr = make(map[int]string)

	if err != nil || errStr != "" {
		log.Error().Err(err).Msgf("[Monitoring Agent deployment errors] %s", errStr)
		sshResultTmp.Stderr[0] = errStr
		sshResultTmp.Err = err
		*returnResult = append(*returnResult, sshResultTmp)
		vmInfoTmp.MonAgentStatus = "failed"
	} else {
		fmt.Println("Result: " + result)
		sshResultTmp.Stdout[0] = result
		sshResultTmp.Err = nil
		*returnResult = append(*returnResult, sshResultTmp)
		vmInfoTmp.MonAgentStatus = "installed"
	}

	UpdateVmInfo(nsID, mcisID, vmInfoTmp)

}

func InstallMonitorAgentToMcis(nsId string, mcisId string, mcisServiceType string, req *McisCmdReq) (AgentInstallContentWrapper, error) {

	err := common.CheckString(nsId)
	if err != nil {
		temp := AgentInstallContentWrapper{}
		log.Error().Err(err).Msg("")
		return temp, err
	}

	err = common.CheckString(mcisId)
	if err != nil {
		temp := AgentInstallContentWrapper{}
		log.Error().Err(err).Msg("")
		return temp, err
	}
	check, _ := CheckMcis(nsId, mcisId)

	if !check {
		temp := AgentInstallContentWrapper{}
		err := fmt.Errorf("The mcis " + mcisId + " does not exist.")
		return temp, err
	}

	content := AgentInstallContentWrapper{}

	//install script
	cmd := "/agent"

	vmList, err := ListVmId(nsId, mcisId)
	if err != nil {
		log.Error().Err(err).Msg("")
		return content, err
	}

	log.Debug().Msg("[Install agent for each VM]")

	//goroutin sync wg
	var wg sync.WaitGroup

	var resultArray []SshCmdResult

	method := "POST"

	for _, v := range vmList {
		vmObjTmp, _ := GetVmObject(nsId, mcisId, v)
		fmt.Println("MonAgentStatus : " + vmObjTmp.MonAgentStatus)

		// Request agent installation (skip if in installing or installed status)
		if vmObjTmp.MonAgentStatus != "installed" && vmObjTmp.MonAgentStatus != "installing" {

			// Avoid RunRemoteCommand to not ready VM
			if err == nil {
				wg.Add(1)
				go CallMonitoringAsync(&wg, nsId, mcisId, mcisServiceType, v, req.UserName, method, cmd, &resultArray)
			} else {
				log.Error().Err(err).Msg("")
			}

		}
	}
	wg.Wait() //goroutin sync wg

	for _, v := range resultArray {

		resultTmp := AgentInstallContent{}
		resultTmp.McisId = mcisId
		resultTmp.VmId = v.VmId
		resultTmp.VmIp = v.VmIp
		resultTmp.Result = v.Stdout[0]
		content.ResultArray = append(content.ResultArray, resultTmp)
	}

	common.PrintJsonPretty(content)

	return content, nil

}

// SetMonitoringAgentStatusInstalled is func to Set Monitoring Agent Status Installed
func SetMonitoringAgentStatusInstalled(nsId string, mcisId string, vmId string) error {
	targetStatus := "installed"
	return UpdateMonitoringAgentStatusManually(nsId, mcisId, vmId, targetStatus)
}

// UpdateMonitoringAgentStatusManually is func to Update Monitoring Agent Installation Status Manually
func UpdateMonitoringAgentStatusManually(nsId string, mcisId string, vmId string, targetStatus string) error {

	vmInfoTmp, err := GetVmObject(nsId, mcisId, vmId)
	if err != nil {
		log.Error().Err(err).Msg("")
		return err
	}

	// set vm MonAgentStatus
	vmInfoTmp.MonAgentStatus = targetStatus
	UpdateVmInfo(nsId, mcisId, vmInfoTmp)

	//TODO: add validation for monitoring

	return nil
}

// GetMonitoringData func retrieves monitoring data from cb-dragonfly
func GetMonitoringData(nsId string, mcisId string, metric string) (MonResultSimpleResponse, error) {

	err := common.CheckString(nsId)
	if err != nil {
		temp := MonResultSimpleResponse{}
		log.Error().Err(err).Msg("")
		return temp, err
	}

	err = common.CheckString(mcisId)
	if err != nil {
		temp := MonResultSimpleResponse{}
		log.Error().Err(err).Msg("")
		return temp, err
	}
	check, _ := CheckMcis(nsId, mcisId)

	if !check {
		temp := MonResultSimpleResponse{}
		err := fmt.Errorf("The mcis " + mcisId + " does not exist.")
		return temp, err
	}

	content := MonResultSimpleResponse{}

	vmList, err := ListVmId(nsId, mcisId)
	if err != nil {
		//log.Error().Err(err).Msg("")
		return content, err
	}

	//goroutin sync wg
	var wg sync.WaitGroup

	var resultArray []MonResultSimple

	method := "GET"

	for _, vmId := range vmList {
		wg.Add(1)

		vmIp, _, _, err := GetVmIp(nsId, mcisId, vmId)
		if err != nil {
			log.Error().Err(err).Msg("")
			wg.Done()
			// continue to next vm even if error occurs
		} else {
			// DF: Get vm on-demand monitoring metric info
			// Path Param: /ns/:nsId/mcis/:mcisId/vm/:vmId/agent_ip/:agent_ip/metric/:metric_name/ondemand-monitoring-info
			cmd := "/ns/" + nsId + "/mcis/" + mcisId + "/vm/" + vmId + "/agent_ip/" + vmIp + "/metric/" + metric + "/ondemand-monitoring-info"
			go CallGetMonitoringAsync(&wg, nsId, mcisId, vmId, vmIp, method, metric, cmd, &resultArray)
		}
	}
	wg.Wait() //goroutin sync wg

	content.NsId = nsId
	content.McisId = mcisId
	for _, v := range resultArray {
		content.McisMonitoring = append(content.McisMonitoring, v)
	}

	fmt.Printf("%+v\n", content)

	return content, nil

}

func CallGetMonitoringAsync(wg *sync.WaitGroup, nsID string, mcisID string, vmID string, vmIP string, method string, metric string, cmd string, returnResult *[]MonResultSimple) {

	defer wg.Done() //goroutin sync done

	log.Info().Msg("[Call CB-DF] " + mcisID + "/" + vmID + "(" + vmIP + ")")

	var response string
	var errStr string
	var result string
	var err error

	url := common.DragonflyRestUrl + cmd
	log.Debug().Msg("URL: " + url)

	responseLimit := 8
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: time.Duration(responseLimit) * time.Minute,
	}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		log.Error().Err(err).Msg("")
		errStr = err.Error()
	}

	fmt.Print("[Call CB-DF Result (" + mcisID + "," + vmID + ")] ")
	res, err := client.Do(req)

	if err != nil {
		log.Error().Err(err).Msg("")
		errStr = err.Error()
	} else {
		fmt.Println("HTTP Status code: " + strconv.Itoa(res.StatusCode))
		switch {
		case res.StatusCode >= 400 || res.StatusCode < 200:
			err1 := fmt.Errorf("HTTP Status: not in 200-399")
			log.Error().Err(err1).Msg("")
			errStr = err1.Error()
		}

		body, err2 := io.ReadAll(res.Body)
		if err2 != nil {
			log.Error().Err(err2).Msg("")
			errStr = err2.Error()
		}
		defer res.Body.Close()
		response = string(body)
	}

	if !gjson.Valid(response) {
		log.Debug().Msg("!gjson.Valid(response)")
	}

	switch metric {
	case monMetricCpu:
		value := gjson.Get(response, "values.cpu_utilization")
		result = value.String()
	case monMetricMem:
		value := gjson.Get(response, "values.mem_utilization")
		result = value.String()
	case monMetricDisk:
		value := gjson.Get(response, "values.disk_utilization")
		result = value.String()
	case monMetricNet:
		value := gjson.Get(response, "values.bytes_out")
		result = value.String()
	default:
		result = response
	}

	//wg.Done() //goroutin sync done

	ResultTmp := MonResultSimple{}
	ResultTmp.VmId = vmID
	ResultTmp.Metric = metric

	if err != nil {
		fmt.Println("CB-DF Error message: " + errStr)
		ResultTmp.Value = errStr
		ResultTmp.Err = err.Error()
		*returnResult = append(*returnResult, ResultTmp)
	} else {
		log.Debug().Msg("CB-DF Result: " + result)
		ResultTmp.Value = result
		*returnResult = append(*returnResult, ResultTmp)
	}

}
