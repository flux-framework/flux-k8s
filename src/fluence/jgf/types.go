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

import "fmt"

type Node struct {
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
	Id         int64             `json:"id"`
	Uniq_id    int64             `json:"uniq_id"`
	Rank       int64             `json:"rank,omitempty"`
	Exclusive  bool              `json:"exclusive"`
	Unit       string            `json:"unit"`
	Size       int64             `json:"size"`
	Paths      map[string]string `json:"paths,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

type graph struct {
	Nodes []Node `json:"nodes"`
	Edges []edge `json:"edges"`
	//	Metadata metadata 	`json:"metadata,omitempty"`
	Directed bool `json:"directed,omitempty"`
}

type FluxJGF struct {
	Graph   graph           `json:"graph"`
	NodeMap map[string]Node `json:"-"`

	// Counters for specific resource types (e.g., rack, node)
	Resources ResourceCounter `json:"-"`
}

// ResourceCounter keeps track of indices for each resource type
type ResourceCounter struct {

	// count of elements by resource type
	counts map[string]int64

	// Total elements in the graph
	Elements int64

	// Name or path of root
	RootName string
}

// ResourceCount provides complete metadata to populate a new node
// This object is returned by the resourceCounter for a node to use
// to quickly derive values, etc.
type ResourceCount struct {

	// Name of the resource (e.g., "red")
	Name string

	// Name of the resource type (e.g., "node")
	Type string

	// Element ID, in the context of total elements in the graph
	ElementId int64

	// Index or count for the resource in question
	Index int64
}

// Return the resource name + resource <count>
// This is scoped to the resource and not global for all the
// elements in the graph
func (r *ResourceCount) NameWithIndex() string {
	return fmt.Sprintf("%s%d", r.Name, r.Index)
}

// StringElementId is the global index as a string
func (r *ResourceCount) StringElementId() string {
	return fmt.Sprintf("%d", r.ElementId)
}

// StringResourceIndex is the string variant of the resource index
func (r *ResourceCount) StringResourceIndex() string {
	return fmt.Sprintf("%d", r.Index)
}

// NextIndex returns the next global index and adds 1 to the count
func (r *ResourceCounter) NextIndex() int64 {
	nextIndex := r.Elements
	r.Elements = nextIndex + 1
	return nextIndex
}

// NextIndex returns the next resource index and adds 1 to the count
func (r *ResourceCounter) NextResourceIndex(resourceType string) int64 {
	nextIndex, ok := r.counts[resourceType]
	if !ok {
		nextIndex = int64(0)
	}
	r.counts[resourceType] = nextIndex + 1
	return nextIndex
}

// getCounter returns the counter context for a specific resource type
func (r *ResourceCounter) getCounter(
	resourceName string,
	resourceType string,
) ResourceCount {
	resourceCount := ResourceCount{
		Index:     r.NextResourceIndex(resourceName),
		Type:      resourceType,
		Name:      resourceName,
		ElementId: r.NextIndex(),
	}

	// Update the count for the next element (global) and resource count
	return resourceCount
}
