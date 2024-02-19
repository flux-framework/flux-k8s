/*
Copyright © 2021 IBM Corporation

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

package jgf

type node struct {
	Id       string       `json:"id"`
	Label    string       `json:"label,omitempty"`
	Metadata nodeMetadata `json:"metadata,omitempty"`
}

type edge struct {
	Source   string       `json:"source"`
	Relation string       `json:"relation,omitempty"`
	Target   string       `json:"target"`
	Directed bool         `json:"directed,omitempty"`
	Metadata edgeMetadata `json:"metadata"`
}

type edgeMetadata struct {
	Name map[string]string `json:"name,omitempty"`
}

type nodeMetadata struct {
	Type       string            `json:"type"`
	Basename   string            `json:"basename"`
	Name       string            `json:"name"`
	Id         int               `json:"id"`
	Uniq_id    int               `json:"uniq_id"`
	Rank       int               `json:"rank,omitempty"`
	Exclusive  bool              `json:"exclusive"`
	Unit       string            `json:"unit"`
	Size       int               `json:"size"`
	Paths      map[string]string `json:"paths,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

type graph struct {
	Nodes []node `json:"nodes"`
	Edges []edge `json:"edges"`
	//	Metadata metadata 	`json:"metadata,omitempty"`
	Directed bool `json:"directed,omitempty"`
}

type Fluxjgf struct {
	Graph    graph           `json:"graph"`
	Elements int             `json:"-"`
	NodeMap  map[string]node `json:"-"`
}
