/*
Copyright 2022 The Kubernetes Authors.

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

package core

import (
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/kubernetes/pkg/scheduler/framework"
	pb "sigs.k8s.io/scheduler-plugins/pkg/fluence/fluxcli-grpc"
)

// FluxStateData is a CycleState
// It holds the PodCache for a pod, which has node assignment, group, and group size
// We also save the group name and size, and time created, in case we want to (somehow) resume scheduling
// In practice I'm not sure how CycleState objects are dumped and loaded. Kueue has a dumper :P
// https://github.com/kubernetes/enhancements/blob/master/keps/sig-scheduling/624-scheduling-framework/README.md#cyclestate
type FluxStateData struct {
	NodeCache NodeCache
}

// Clone is required for CycleState plugins
func (s *FluxStateData) Clone() framework.StateData {
	return &FluxStateData{NodeCache: s.NodeCache}
}

// NewFluxState creates an entry for the CycleState with the minimum that we might need
func NewFluxState(nodeName string, groupName string, size int32) *FluxStateData {
	cache := NodeCache{
		NodeName:     nodeName,
		GroupName:    groupName,
		MinGroupSize: size,
	}
	return &FluxStateData{NodeCache: cache}
}

// NodeCache holds the node name and tasks for the node
// For the PodGroupCache, these are organized by group name,
// and there is a list of them
type NodeCache struct {
	NodeName string

	// This is derived from tasks, where
	// task is an allocation to some node
	// High level it is most often akin to the
	// number of pods on the node. I'm not sure that I understand this
	// https://github.com/flux-framework/flux-k8s/blob/9f24f36752e3cced1b1112d93bfa366fb58b3c84/src/fluence/fluxion/fluxion.go#L94-L97
	// How does that relate to a single pod? It is called "Count" in other places
	Tasks int

	// These fields are primarily for the FluxStateData
	// Without a PodGroup CRD we keep min size here
	MinGroupSize int32
	GroupName    string
}

// A pod group cache holds a list of nodes for an allocation, where each has some number of tasks
// along with the expected group size. This is intended to replace PodGroup
// given the group name, size (derived from annotations) and timestamp
type PodGroupCache struct {

	// This is a cache of nodes for pods
	Nodes       []NodeCache
	Size        int32
	Name        string
	IsAllocated bool

	// Keep track of when the group was initially created!
	// This is like, the main thing we need.
	TimeCreated metav1.Time
}

// Memory cache of pod group name to pod group cache, above
var podGroupCache map[string]*PodGroupCache

// Init populates the podGroupCache
func Init() {
	podGroupCache = map[string]*PodGroupCache{}
}

// RegisterPodGroup ensures that the PodGroup exists in the cache
// This is an experimental replacement for an actual PodGroup
// We take a timestampo, which if called from Less (during sorting) is tiem.Time
// if called later (an individual pod) we go for its creation timestamp
func RegisterPodGroup(pod *v1.Pod, groupName string, groupSize int32) error {
	entry, ok := podGroupCache[groupName]

	// Always honor the earlier time
	// Important time.Now is a LOCAL timestamp
	// If groups can be created across timezones (and could then be compared)
	// this would maybe be an issue
	creationTime := metav1.NewTime(time.Now())
	if pod.CreationTimestamp.Before(&creationTime) {
		creationTime = pod.CreationTimestamp
	}

	if !ok {
		nodes := []NodeCache{}

		// Create the new entry for the pod group
		entry = &PodGroupCache{
			Name:        groupName,
			Size:        groupSize,
			Nodes:       nodes,
			TimeCreated: creationTime,
		}
	}
	// Tell the user when it was created
	klog.Infof("Pod group %s was created at %s", entry.Name, entry.TimeCreated)

	// If the size has changed, we currently do not allow updating it.
	// We issue a warning. In the future this could be supported with a grow command.
	if entry.Size != groupSize {
		klog.Warningf("Pod group %s request to change size from %s to %s is not yet supported", groupName, entry.Size, groupSize)
		// entry.GroupSize = groupSize
	}
	podGroupCache[groupName] = entry
	return nil
}

// GetPodGroup gets a pod group in the cache by name
func GetPodGroup(groupName string) *PodGroupCache {
	entry, _ := podGroupCache[groupName]
	return entry
}

// DeletePodGroup deletes a pod from the group cache
func DeletePodGroup(groupName string) {
	delete(podGroupCache, groupName)
}

// CreateNodePodsList creates a list of node pod caches
func CreateNodePodsList(nodelist []*pb.NodeAlloc, groupName string) (nodepods []NodeCache) {

	// Create a pod cache for each node
	nodepods = make([]NodeCache, len(nodelist))

	for i, v := range nodelist {
		nodepods[i] = NodeCache{
			NodeName: v.GetNodeID(),
			Tasks:    int(v.GetTasks()),
		}
	}

	// Update the pods in the PodGraphCache
	updatePodGroupNodes(groupName, nodepods)
	klog.Info("Pod Group Cache ", podGroupCache)
	return nodepods
}

// updatePodGroupList updates the PodGroupCache with a listing of nodes
func updatePodGroupNodes(groupName string, nodes []NodeCache) {
	group := podGroupCache[groupName]
	group.Nodes = nodes
	podGroupCache[groupName] = group
}

// HavePodNodes returns true if the listing of pods is not empty
// This should be all pods that are needed - the allocation will not
// be successful otherwise, so we just check > 0
func (p *PodGroupCache) HavePodNodes() bool {
	return len(p.Nodes) > 0
}

// CancelAllocation resets the node cache and allocation status
func (p *PodGroupCache) CancelAllocation() {
	p.Nodes = []NodeCache{}
	p.IsAllocated = false
}

// GetNextNode gets the next available node we can allocate for a group
func GetNextNode(groupName string) (string, error) {
	entry, ok := podGroupCache[groupName]

	// This case should not happen
	if !ok {
		return "", fmt.Errorf("no entry for pod group %s in cache", groupName)
	}

	// We don't have nodes, but we had it allocated, so we are done
	if len(entry.Nodes) == 0 && entry.IsAllocated {
		return "", nil
	}

	// If we aren't allocated but no nodes, likely we failed and need to try again
	if len(entry.Nodes) == 0 && !entry.IsAllocated {
		return "", fmt.Errorf("no nodes have been allocated for %s", groupName)
	}

	nodename := entry.Nodes[0].NodeName

	// Note from Vsoch: I don't understand this logic of checking tasks
	// I think this is saying that when we've used up all the allocations
	// assigned for the group, we can clean it up from the pod group. We also
	// might be able to clean it up on the Cancel function?
	if entry.Nodes[0].Tasks == 1 {
		slice := entry.Nodes[1:]
		if len(slice) != 0 {
			updatePodGroupNodes(groupName, slice)
		}
		return nodename, nil
	}
	// Why are we subtracting one here?
	entry.Nodes[0].Tasks = entry.Nodes[0].Tasks - 1
	return nodename, nil
}
