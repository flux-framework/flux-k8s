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

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

var (
	// Defaults for nodes
	defaultExclusive = false
	defaultRank      = int64(-1)
	defaultSize      = int64(1)
	defaultUnit      = ""

	// Relations
	ContainsRelation = "contains"
	InRelation       = "in"

	// Vertex (node) types
	// These are public to be used in the utils package
	ClusterType     = "cluster"
	NodeType        = "node"
	CoreType        = "core"
	VirtualCoreType = "vcore"
	RackType        = "rack"
	SocketType      = "socket"
	SubnetType      = "subnet"
	MemoryType      = "memory"
	NvidiaGPU       = "nvidiagpu"
	GPUType         = "gpu"

	// Paths
	containmentKey = "containment"
)

// NewFluxJGF creates and returns a new Flux Json Graph Format object
func NewFluxJGF() FluxJGF {

	// Create a new cluster, and count the top level as a resource
	// The index 0 (of the element count) is the cluster
	counters := map[string]int64{"cluster": int64(1)}
	return FluxJGF{
		Graph:   graph{},
		NodeMap: make(map[string]Node),

		// Counters and lookup for resources
		Resources: ResourceCounter{counts: counters},
	}
}

// ToJson returns a Json string of the graph
func (g *FluxJGF) ToJson() (string, error) {
	toprint, err := json.MarshalIndent(g.Graph, "", "\t")
	return string(toprint), err
}

// GetNodePath returns the node containment path
func getNodePath(root, subpath string) string {
	var path string
	if subpath == "" {
		path = fmt.Sprintf("/%s", root)
	} else {
		path = fmt.Sprintf("/%s/%s", root, subpath)
	}
	// Hack to allow for imperfection of slash placement
	path = strings.ReplaceAll(path, "//", "/")
	return path
}

// getContainmentPath returns a new map with containment metadata
func (g *FluxJGF) getContainmentPath(subpath string) map[string]string {
	return map[string]string{containmentKey: getNodePath(g.Resources.RootName, subpath)}
}

// MakeBidirectionalEdge makes an edge for a parent and child
func (g *FluxJGF) MakeBidirectionalEdge(parent, child string) {
	g.MakeEdge(parent, child, ContainsRelation)
	g.MakeEdge(child, parent, InRelation)
}

// MakeEdge creates an edge for the JGF
func (g *FluxJGF) MakeEdge(source string, target string, contains string) {
	newedge := edge{
		Source: source,
		Target: target,
		Metadata: edgeMetadata{
			Name: map[string]string{containmentKey: contains},
		},
	}
	g.Graph.Edges = append(g.Graph.Edges, newedge)
}

// MakeSubnet creates a subnet for the graph
// The name is typically the ip address
func (g *FluxJGF) MakeSubnet(name string) Node {

	// Get a resource counter for the subnet
	resource := g.Resources.getCounter(name, SubnetType)
	subpath := resource.NameWithIndex()
	return g.makeNewNode(resource, subpath, defaultUnit, defaultSize)
}

// makeNewNode is a shared function to make a new node from a resource spec
// subpath is the subpath to add to the graph root, e.g., <cluster>/<subpath>
// Since there is some variability to this structure, it is assembled by
// the calling function
func (g *FluxJGF) makeNewNode(
	resource ResourceCount,
	subpath, unit string,
	size int64,
) Node {

	// A subnet comes directly under the cluster, which is the parent
	newNode := Node{

		// Global identifier in graph, as a string
		Id: resource.StringElementId(),
		Metadata: nodeMetadata{
			Type: resource.Type,

			// The original name without an index
			Basename: resource.Name,

			// The name with an index
			Name: resource.NameWithIndex(),

			// Integer resource index
			Id: resource.Index,

			// Integer global element index
			Uniq_id:   resource.ElementId,
			Rank:      defaultRank,
			Exclusive: defaultExclusive,
			Unit:      unit,
			Size:      size,

			// subnet is one above root graph, so just need it's name
			Paths: g.getContainmentPath(subpath),
		},
	}

	// Add the new node to the graph
	g.Graph.Nodes = append(g.Graph.Nodes, newNode)
	g.NodeMap[newNode.Id] = newNode
	return newNode
}

// MakeNode creates a new node for the graph
func (g *FluxJGF) MakeNode(name, subpath string) Node {

	// Get a resource counter for the node, which is under the subnet
	resource := g.Resources.getCounter(name, NodeType)

	// Here the full containment path will be:
	// <cluster-root>/<subnet>/<node>
	subpath = fmt.Sprintf("%s/%s", subpath, resource.NameWithIndex())
	return g.makeNewNode(resource, subpath, defaultUnit, defaultSize)
}

// MakeCore creates a core for the graph
func (g *FluxJGF) MakeCore(name, subpath string) Node {

	// A core is located at the subnet->node->core
	resource := g.Resources.getCounter(name, CoreType)

	// Here the full containment path will be:
	// <cluster-root>/<subnet>/<node>/<core>
	subpath = fmt.Sprintf("%s/%s", subpath, resource.NameWithIndex())
	return g.makeNewNode(resource, subpath, defaultUnit, defaultSize)
}

// MakeMemory creates memory for the graph
// Flux doesn't understand memory? Not sure if this is doing anything
func (g *FluxJGF) MakeMemory(name, subpath string, size int64) Node {

	// unit is assumed to be MB
	unit := "MB"

	// A core is located at the subnet->node->core
	resource := g.Resources.getCounter(name, MemoryType)

	// Here the full containment path will be:
	// <cluster-root>/<subnet>/<node>/<memory>
	subpath = fmt.Sprintf("%s/%s", subpath, resource.NameWithIndex())
	return g.makeNewNode(resource, subpath, unit, size)
}

// MakeGPU makes a gpu for the graph
func (g *FluxJGF) MakeGPU(name, subpath string, size int64) Node {

	// Get a resource counter for the gpu, which is under the subnet->node->gpu
	resource := g.Resources.getCounter(name, GPUType)

	// Here the full containment path will be:
	// <cluster-root>/<subnet>/<node>
	subpath = fmt.Sprintf("%s/%s", subpath, resource.NameWithIndex())
	return g.makeNewNode(resource, subpath, defaultUnit, size)
}

// InitCluster creates a new cluster, primarily the root "cluster" node
func (g *FluxJGF) InitCluster(name string) (Node, error) {
	if g.Resources.Elements > 0 {
		return Node{}, fmt.Errorf("init can only be called for a new cluster")
	}

	// The cluster name is the index (always 0) with the original name
	g.Resources.RootName = fmt.Sprintf("%s0", name)
	resource := g.Resources.getCounter(name, ClusterType)
	return g.makeNewNode(resource, "", defaultUnit, defaultSize), nil
}

func (g *FluxJGF) WriteJGF(path string) error {

	encodedJGF, err := json.MarshalIndent(g, "", "  ")

	if err != nil {
		log.Fatalf("[JGF] json.Marshal failed with '%s'\n", err)
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("[JGF] Couldn't create JGF file!!\n")
		return err
	}
	defer f.Close()

	_, err = f.Write(encodedJGF)
	if err != nil {
		log.Fatalf("[JGF] Couldn't write JGF file!!\n")
		return err
	}
	return nil
}
