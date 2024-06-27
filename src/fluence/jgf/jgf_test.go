package jgf

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewFluxJGF(t *testing.T) {

	// Create a new FluxGraph, assert that it is empty
	fluxgraph := NewFluxJGF()
	assert.Equal(t, len(fluxgraph.Graph.Nodes), 0)
	assert.Equal(t, fluxgraph.Resources.Elements, int64(0))
	assert.Equal(t, len(fluxgraph.NodeMap), 0)

	out, err := fluxgraph.ToJson()
	assert.Nil(t, err)
	fmt.Println()
	fmt.Println("== Empty graph:")
	fmt.Println(out)

	// Init the cluster (make the root node)
	clusterNode, err := fluxgraph.InitCluster("keebler")
	assert.Nil(t, err)

	out, err = fluxgraph.ToJson()
	assert.Nil(t, err)
	fmt.Println()
	fmt.Println("== Graph with Cluster Root:")
	fmt.Println(out)

	// Add subnets to it
	subnetNodeA := fluxgraph.MakeSubnet("east", 0)
	subnetNodeB := fluxgraph.MakeSubnet("west", 1)
	fluxgraph.MakeBidirectionalEdge(clusterNode.Id, subnetNodeA.Id)
	fluxgraph.MakeBidirectionalEdge(clusterNode.Id, subnetNodeB.Id)

	out, err = fluxgraph.ToJson()
	assert.Nil(t, err)
	fmt.Println()
	fmt.Println("== Graph with Two Subnets:")
	fmt.Println(out)

	// Add some nodes!
	computeNodeA := fluxgraph.MakeNode("node", subnetNodeA.Metadata.Name, 0)
	computeNodeB := fluxgraph.MakeNode("node", subnetNodeB.Metadata.Name, 1)
	fluxgraph.MakeBidirectionalEdge(subnetNodeA.Id, computeNodeA.Id)
	fluxgraph.MakeBidirectionalEdge(subnetNodeB.Id, computeNodeB.Id)

	out, err = fluxgraph.ToJson()
	assert.Nil(t, err)
	fmt.Println()
	fmt.Println("== Graph with Two Subnets, Each with a node:")
	fmt.Println(out)

	// Add a GPU to one, and cores to the other
	subpath := fmt.Sprintf("%s/%s", subnetNodeA.Metadata.Name, computeNodeA.Metadata.Name)
	gpuNodeA := fluxgraph.MakeGPU(NvidiaGPU, subpath, 1, 0)
	fluxgraph.MakeBidirectionalEdge(computeNodeA.Id, gpuNodeA.Id)

	subpath = fmt.Sprintf("%s/%s", subnetNodeB.Metadata.Name, computeNodeB.Metadata.Name)
	coreNode := fluxgraph.MakeCore(CoreType, subpath, 0)
	fluxgraph.MakeBidirectionalEdge(computeNodeB.Id, coreNode.Id)

	// Finally, add some memory to the second compute node
	memoryNode := fluxgraph.MakeMemory(MemoryType, subpath, 1<<10, 0)
	fluxgraph.MakeBidirectionalEdge(computeNodeA.Id, memoryNode.Id)

	out, err = fluxgraph.ToJson()
	assert.Nil(t, err)
	fmt.Println()
	fmt.Println("== Graph with Two Subnets, Two Nodes, with GPU/Core/Memory:")
	fmt.Println(out)

}
