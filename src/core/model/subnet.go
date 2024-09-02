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

// Package model is to handle object of CB-Tumblebug
package model

// TbSubnetReq is a struct that represents TB subnet object.
type TbSubnetReq struct { // Tumblebug
	Name        string     `json:"name" validate:"required" example:"subnet00"`
	IPv4_CIDR   string     `json:"ipv4_CIDR" validate:"required" example:"10.0.1.0/24"`
	Zone        string     `json:"zone,omitempty"`
	TagList     []KeyValue `json:"tagList,omitempty"`
	Description string     `json:"description,omitempty" example:"subnet00 managed by CB-Tumblebug"`
	// KeyValueList []KeyValue `json:"keyValueList,omitempty"`
	// IdFromCsp    string     `json:"idFromCsp,omitempty"`
}

type TbRegisterSubnetReq struct {
	ConnectionName string `json:"connectionName" validate:"required"`
	CspSubnetId    string `json:"cspSubnetId" validate:"required"`
	Name           string `json:"name" validate:"required"`
	Zone           string `json:"zone,omitempty"`
	Description    string `json:"description,omitempty"`
}

// TbSubnetInfo is a struct that represents TB subnet object.
type TbSubnetInfo struct { // Tumblebug
	Id             string        `json:"id"`
	Name           string        `json:"name"`
	Uuid           string        `json:"uuid,omitempty"` // uuid is universally unique identifier for the resource
	ConnectionName string        `json:"connectionName"`
	CspVNetId      string        `json:"cspVNetId"`
	CspVNetName    string        `json:"cspVNetName"`
	CspSubnetId    string        `json:"cspSubnetId"`
	CspSubnetName  string        `json:"cspSubnetName"`
	Status         string        `json:"status"`
	IPv4_CIDR      string        `json:"ipv4_CIDR"`
	Zone           string        `json:"zone,omitempty"`
	TagList        []KeyValue    `json:"tagList,omitempty"`
	BastionNodes   []BastionNode `json:"bastionNodes,omitempty"`
	KeyValueList   []KeyValue    `json:"keyValueList,omitempty"`
	Description    string        `json:"description"`
}
