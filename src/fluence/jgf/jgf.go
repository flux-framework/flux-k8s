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
	"strconv"
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

// InitJGF initializes the Flux Json Graph Format object
func InitJGF() (fluxgraph Fluxjgf) {
	var g graph
	fluxgraph = Fluxjgf{
		Graph:    g,
		Elements: 0,
		NodeMap:  make(map[string]node),
	}
	return
}

// getDefaultPaths returns a new map with empty containment
// this cannot be a global shared variable or we get an error
// about inserting an edge to itself.
func getDefaultPaths() map[string]string {
	return map[string]string{containmentKey: ""}
}

// addNode adds a node to the JGF
func (g *Fluxjgf) addNode(toadd node) {
	g.Graph.Nodes = append(g.Graph.Nodes, toadd)
	g.NodeMap[toadd.Id] = toadd
	g.Elements = g.Elements + 1
}

// MakeEdge creates an edge for the JGF
func (g *Fluxjgf) MakeEdge(source string, target string, contains string) {
	newedge := edge{
		Source: source,
		Target: target,
		Metadata: edgeMetadata{
			Name: map[string]string{containmentKey: contains},
		},
	}
	g.Graph.Edges = append(g.Graph.Edges, newedge)
	if contains == ContainsRelation {
		tnode := g.NodeMap[target]
		tnode.Metadata.Paths[containmentKey] = g.NodeMap[source].Metadata.Paths[containmentKey] + "/" + tnode.Metadata.Name
	}
}

// MakeSubnet creates a subnet for the graph
func (g *Fluxjgf) MakeSubnet(index int64, ip string) string {
	newnode := node{
		Id: fmt.Sprintf("%d", g.Elements),
		Metadata: nodeMetadata{
			Type:      SubnetType,
			Basename:  ip,
			Name:      ip + fmt.Sprintf("%d", g.Elements),
			Id:        index,
			Uniq_id:   g.Elements,
			Rank:      defaultRank,
			Exclusive: defaultExclusive,
			Unit:      defaultUnit,
			Size:      defaultSize,
			Paths:     getDefaultPaths(),
		},
	}
	g.addNode(newnode)
	return newnode.Id
}

// MakeNode creates a new node for the graph
func (g *Fluxjgf) MakeNode(index int, exclusive bool, subnet string) string {
	newnode := node{
		Id: fmt.Sprintf("%d", g.Elements),
		Metadata: nodeMetadata{
			Type:      NodeType,
			Basename:  subnet,
			Name:      subnet + fmt.Sprintf("%d", g.Elements),
			Id:        g.Elements,
			Uniq_id:   g.Elements,
			Rank:      defaultRank,
			Exclusive: exclusive,
			Unit:      defaultUnit,
			Size:      defaultSize,
			Paths:     getDefaultPaths(),
		},
	}
	g.addNode(newnode)
	return newnode.Id
}

// MakeSocket creates a socket for the graph
func (g *Fluxjgf) MakeSocket(index int64, name string) string {
	newnode := node{
		Id: fmt.Sprintf("%d", g.Elements),
		Metadata: nodeMetadata{
			Type:      SocketType,
			Basename:  name,
			Name:      name + fmt.Sprintf("%d", index),
			Id:        index,
			Uniq_id:   g.Elements,
			Rank:      defaultRank,
			Exclusive: defaultExclusive,
			Unit:      defaultUnit,
			Size:      defaultSize,
			Paths:     getDefaultPaths(),
		},
	}
	g.addNode(newnode)
	return newnode.Id
}

// MakeCore creates a core for the graph
func (g *Fluxjgf) MakeCore(index int64, name string) string {
	newnode := node{
		Id: fmt.Sprintf("%d", g.Elements),
		Metadata: nodeMetadata{
			Type:      CoreType,
			Basename:  name,
			Name:      name + fmt.Sprintf("%d", index),
			Id:        index,
			Uniq_id:   g.Elements,
			Rank:      defaultRank,
			Exclusive: defaultExclusive,
			Unit:      defaultUnit,
			Size:      defaultSize,
			Paths:     getDefaultPaths(),
		},
	}
	g.addNode(newnode)
	return newnode.Id
}

// MakeVCore makes a vcore (I think 2 vcpu == 1 cpu) for the graph
func (g *Fluxjgf) MakeVCore(coreid string, index int64, name string) string {
	newnode := node{
		Id: fmt.Sprintf("%d", g.Elements),
		Metadata: nodeMetadata{
			Type:      VirtualCoreType,
			Basename:  name,
			Name:      name + fmt.Sprintf("%d", index),
			Id:        index,
			Uniq_id:   g.Elements,
			Rank:      defaultRank,
			Exclusive: defaultExclusive,
			Unit:      defaultUnit,
			Size:      defaultSize,
			Paths:     getDefaultPaths(),
		},
	}
	g.addNode(newnode)
	g.MakeEdge(coreid, newnode.Id, ContainsRelation)
	g.MakeEdge(newnode.Id, coreid, InRelation)
	return newnode.Id
}

// MakeNFProperties makes the node feature discovery properties for the graph
func (g *Fluxjgf) MakeNFDProperties(coreid string, index int64, filter string, labels *map[string]string) {
	for key, _ := range *labels {
		if strings.Contains(key, filter) {
			name := strings.Split(key, "/")[1]
			if strings.Contains(name, ".") {
				name = strings.Split(name, ".")[1]
			}

			newnode := node{
				Id: fmt.Sprintf("%d", g.Elements),
				Metadata: nodeMetadata{
					Type:      name,
					Basename:  name,
					Name:      name + fmt.Sprintf("%d", index),
					Id:        index,
					Uniq_id:   g.Elements,
					Rank:      defaultRank,
					Exclusive: defaultExclusive,
					Unit:      defaultUnit,
					Size:      defaultSize,
					Paths:     getDefaultPaths(),
				},
			}
			g.addNode(newnode)
			g.MakeEdge(coreid, newnode.Id, ContainsRelation)
		}
	}
}

func (g *Fluxjgf) MakeNFDPropertiesByValue(coreid string, index int64, filter string, labels *map[string]string) {
	for key, val := range *labels {
		if strings.Contains(key, filter) {
			name := val

			newnode := node{
				Id: fmt.Sprintf("%d", g.Elements),
				Metadata: nodeMetadata{
					Type:      name,
					Basename:  name,
					Name:      name + fmt.Sprintf("%d", index),
					Id:        index,
					Uniq_id:   g.Elements,
					Rank:      defaultRank,
					Exclusive: defaultExclusive,
					Unit:      defaultUnit,
					Size:      defaultSize,
					Paths:     getDefaultPaths(),
				},
			}
			g.addNode(newnode)
			g.MakeEdge(coreid, newnode.Id, ContainsRelation)
		}
	}
}

// MakeMemory creates memory for the graph
func (g *Fluxjgf) MakeMemory(index int64, name string, unit string, size int64) string {
	newnode := node{
		Id: fmt.Sprintf("%d", g.Elements),
		Metadata: nodeMetadata{
			Type:      MemoryType,
			Basename:  name,
			Name:      name + fmt.Sprintf("%d", index),
			Id:        index,
			Uniq_id:   g.Elements,
			Rank:      defaultRank,
			Exclusive: defaultExclusive,
			Unit:      unit,
			Size:      size,
			Paths:     getDefaultPaths(),
		},
	}
	g.addNode(newnode)
	return newnode.Id
}

// MakeGPU makes a gpu for the graph
func (g *Fluxjgf) MakeGPU(index int64, name string, size int64) string {
	newnode := node{
		Id: fmt.Sprintf("%d", g.Elements),
		Metadata: nodeMetadata{
			Type:      GPUType,
			Basename:  name,
			Name:      name + fmt.Sprintf("%d", index),
			Id:        index,
			Uniq_id:   g.Elements,
			Rank:      defaultRank,
			Exclusive: defaultExclusive,
			Unit:      defaultUnit,
			Size:      size,
			Paths:     getDefaultPaths(),
		},
	}
	g.addNode(newnode)
	return newnode.Id
}

// MakeCluster creates the cluster
func (g *Fluxjgf) MakeCluster(clustername string) string {
	g.Elements = 0
	newnode := node{
		Id: strconv.Itoa(0),
		Metadata: nodeMetadata{
			Type:      ClusterType,
			Basename:  clustername,
			Name:      clustername + "0",
			Id:        g.Elements,
			Uniq_id:   0,
			Rank:      defaultRank,
			Exclusive: defaultExclusive,
			Unit:      defaultUnit,
			Size:      defaultSize,
			Paths: map[string]string{
				containmentKey: "/" + clustername + "0",
			},
		},
	}
	g.addNode(newnode)
	return newnode.Id
}

// MakeRack makes the rack
func (g *Fluxjgf) MakeRack(index int64) string {
	newnode := node{
		Id: fmt.Sprintf("%d", g.Elements),
		Metadata: nodeMetadata{
			Type:      RackType,
			Basename:  RackType,
			Name:      RackType + fmt.Sprintf("%d", index),
			Id:        index,
			Uniq_id:   g.Elements,
			Rank:      defaultRank,
			Exclusive: defaultExclusive,
			Unit:      defaultUnit,
			Size:      defaultSize,
			Paths:     getDefaultPaths(),
		},
	}
	g.addNode(newnode)
	return newnode.Id
}

func (g *Fluxjgf) WriteJGF(path string) error {

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
