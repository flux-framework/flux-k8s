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

func InitJGF() (fluxgraph Fluxjgf) {
	var g graph
	fluxgraph = Fluxjgf{
		Graph:    g,
		Elements: 0,
		NodeMap:  make(map[string]node),
	}
	return
}
func (g *Fluxjgf) addNode(toadd node) {
	g.Graph.Nodes = append(g.Graph.Nodes, toadd)
	g.NodeMap[toadd.Id] = toadd
	g.Elements = g.Elements + 1
}

func (g *Fluxjgf) MakeEdge(source string, target string, contains string) {
	newedge := edge{
		Source: source,
		Target: target,
		Metadata: edgeMetadata{
			Name: map[string]string{
				"containment": contains,
			},
		},
	}
	g.Graph.Edges = append(g.Graph.Edges, newedge)
	if contains == "contains" {
		tnode := g.NodeMap[target]
		tnode.Metadata.Paths["containment"] = g.NodeMap[source].Metadata.Paths["containment"] + "/" + tnode.Metadata.Name
	}

}

func processLabels(labels *map[string]string, filter string) (filtered map[string]string) {
	filtered = make(map[string]string, 0)
	for key, v := range *labels {
		if strings.Contains(key, filter) {

			filtered[key] = v
		}
	}
	return
}

func (g *Fluxjgf) MakeNode(index int, exclusive bool, ip string) string {
	newnode := node{
		Id: strconv.Itoa(g.Elements),
		Metadata: nodeMetadata{
			Type:      "node",
			Basename:  ip,
			Name:      ip + strconv.Itoa(g.Elements),
			Id:        index,
			Uniq_id:   g.Elements,
			Rank:      -1,
			Exclusive: exclusive,
			Unit:      "",
			Size:      1,
			Paths: map[string]string{
				"containment": "",
			},
		},
	}
	g.addNode(newnode)
	return newnode.Id
}

func (g *Fluxjgf) MakeSocket(index int, name string) string {
	newnode := node{
		Id: strconv.Itoa(g.Elements),
		Metadata: nodeMetadata{
			Type:      "socket",
			Basename:  name,
			Name:      name + strconv.Itoa(index),
			Id:        index,
			Uniq_id:   g.Elements,
			Rank:      -1,
			Exclusive: false,
			Unit:      "",
			Size:      1,
			Paths: map[string]string{
				"containment": "",
			},
		},
	}
	g.addNode(newnode)
	return newnode.Id
}

func (g *Fluxjgf) MakeCore(index int, name string) string {
	newnode := node{
		Id: strconv.Itoa(g.Elements),
		Metadata: nodeMetadata{
			Type:      "core",
			Basename:  name,
			Name:      name + strconv.Itoa(index),
			Id:        index,
			Uniq_id:   g.Elements,
			Rank:      -1,
			Exclusive: false,
			Unit:      "",
			Size:      1,
			Paths: map[string]string{
				"containment": "",
			},
			// Properties: processLabels(labels, "cpu-cpuid"),
		},
	}
	g.addNode(newnode)
	return newnode.Id
}

func (g *Fluxjgf) MakeNFDProperties(coreid string, index int, filter string, labels *map[string]string) {
	for key, _ := range *labels {
		if strings.Contains(key, filter) {
			// fmt.Println("want to split ", key)
			name := strings.Split(key, "/")[1]
			fmt.Println("name ", name)
			if strings.Contains(name, ".") {
				name = strings.Split(name, ".")[1]
			}

			newnode := node{
				Id: strconv.Itoa(g.Elements),
				Metadata: nodeMetadata{
					Type:      name,
					Basename:  name,
					Name:      name + strconv.Itoa(index),
					Id:        index,
					Uniq_id:   g.Elements,
					Rank:      -1,
					Exclusive: false,
					Unit:      "",
					Size:      1,
					Paths: map[string]string{
						"containment": "",
					},
				},
			}
			g.addNode(newnode)
			// fmt.Println("making edge between core ", coreid, " and property ", newnode.Id)
			g.MakeEdge(coreid, newnode.Id, "contains")
		}
	}

	//return newnode.Id
}

func (g *Fluxjgf) MakeMemory(index int, name string, unit string, size int) string {
	newnode := node{
		Id: strconv.Itoa(g.Elements),
		Metadata: nodeMetadata{
			Type:      "memory",
			Basename:  name,
			Name:      name + strconv.Itoa(index),
			Id:        index,
			Uniq_id:   g.Elements,
			Rank:      -1,
			Exclusive: false,
			Unit:      unit,
			Size:      size,
			Paths: map[string]string{
				"containment": "",
			},
		},
	}
	g.addNode(newnode)
	return newnode.Id
}

func (g *Fluxjgf) MakeGPU(index int, name string, size int) string {
	newnode := node{
		Id: strconv.Itoa(g.Elements),
		Metadata: nodeMetadata{
			Type:      "GPU",
			Basename:  name,
			Name:      name + strconv.Itoa(index),
			Id:        index,
			Uniq_id:   g.Elements,
			Rank:      -1,
			Exclusive: false,
			Unit:      "",
			Size:      size,
			Paths: map[string]string{
				"containment": "",
			},
		},
	}
	g.addNode(newnode)
	return newnode.Id
}

func (g *Fluxjgf) MakeCluster(clustername string) string {
	g.Elements = 0
	newnode := node{
		Id: strconv.Itoa(0),
		Metadata: nodeMetadata{
			Type:      "cluster",
			Basename:  clustername,
			Name:      clustername + "0",
			Id:        g.Elements,
			Uniq_id:   0,
			Rank:      -1,
			Exclusive: false,
			Unit:      "",
			Size:      1,
			Paths: map[string]string{
				"containment": "/" + clustername + "0",
			},
		},
	}
	g.addNode(newnode)
	return newnode.Id
}

func (g *Fluxjgf) MakeRack(id int) string {
	newnode := node{
		Id: strconv.Itoa(g.Elements),
		Metadata: nodeMetadata{
			Type:      "rack",
			Basename:  "rack",
			Name:      "rack" + strconv.Itoa(id),
			Id:        id,
			Uniq_id:   g.Elements,
			Rank:      -1,
			Exclusive: false,
			Unit:      "",
			Size:      1,
			Paths: map[string]string{
				"containment": "",
			},
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
