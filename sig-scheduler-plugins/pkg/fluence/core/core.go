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

	"k8s.io/kubernetes/pkg/scheduler/framework"
	pb "sigs.k8s.io/scheduler-plugins/pkg/fluence/fluxcli-grpc"
)

// FluxStateData is a CycleState
// It holds the PodCache for a pod, which has node assignment, group, and group size
// We also save the group name and size, and time created, in case we want to (somehow) resume scheduling
// In practice I'm not sure how CycleState objects are dumped and loaded. Kueue has a dumper :P
// https://github.com/kubernetes/enhancements/blob/master/keps/sig-scheduling/624-scheduling-framework/README.md#cyclestate
type FluxStateData struct {
	PodCache PodCache
}

// Clone is required for CycleState plugins
func (s *FluxStateData) Clone() framework.StateData {
	return &FluxStateData{PodCache: s.PodCache}
}

// NewFluxState creates an entry for the CycleState with the minimum that we might need
func NewFluxState(nodeName string, groupName string, size int32) *FluxStateData {
	podCache := PodCache{
		NodeName:     nodeName,
		GroupName:    groupName,
		MinGroupSize: size,
	}
	return &FluxStateData{PodCache: podCache}
}

// PodCache holds the node name and tasks for the node
// For the PodGroupCache, these are organized by group name,
// and there is a list of them
type PodCache struct {
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

// A pod group cache holds a list of pods, where each has some number of tasks
// along with the expected group size. This is intended to replace PodGroup
// given the group name, size (derived from annotations) and timestamp
type PodGroupCache struct {

	// TODO need to debug that this not being a pointer isn't an issue
	Pods      []PodCache
	GroupSize int32
	GroupName string

	// Keep track of when the group was initially created!
	// This is like, the main thing we need.
	TimeCreated time.Time
}

// Memory cache of pod group name to pod group cache, above
var podGroupCache map[string]PodGroupCache

// Init populates the podGroupCache
func Init() {
	podGroupCache = map[string]PodGroupCache{}
}

// RegisterPodGroup ensures that the PodGroup exists in the cache
// This is an experimental replacement for an actual PodGroup
func RegisterPodGroup(pod *v1.Pod, groupName string, groupSize int32) error {
	entry, ok := podGroupCache[groupName]
	if !ok {

		// Important - this is a LOCAL timestamp
		// If groups can be created across timezones (and could then be compared)
		// this would maybe be an issue
		creationTime := time.Now()
		pods := []PodCache{}

		// Create the new entry for the pod group
		entry = PodGroupCache{
			GroupName:   groupName,
			GroupSize:   groupSize,
			Pods:        pods,
			TimeCreated: creationTime,
		}
	}

	// If the size has changed, update it. We assume the running user might change it
	if entry.GroupSize != groupSize {
		klog.Warningf("Pod group %s changing size from %s to %s", groupName, entry.GroupSize, groupSize)
		entry.GroupSize = groupSize
	}
	podGroupCache[groupName] = entry
	return nil
}

// GetPodGroup gets a pod group in the cache by name
func GetPodGroup(groupName string) PodGroupCache {
	entry, _ := podGroupCache[groupName]
	return entry
}

// CreateNodePodsList creates a list of node pod caches
func CreateNodePodsList(nodelist []*pb.NodeAlloc, groupName string) (nodepods []PodCache) {

	// Create a pod cache for each node
	nodepods = make([]PodCache, len(nodelist))

	for i, v := range nodelist {
		nodepods[i] = PodCache{
			NodeName: v.GetNodeID(),
			Tasks:    int(v.GetTasks()),
		}
	}

	// Update the pods in the PodGraphCache
	updatePodGroupList(groupName, nodepods)
	klog.Info("Pod Group Cache ", podGroupCache)
	return nodepods
}

// updatePodGroupList updates the PodGroupCache with a group.
func updatePodGroupList(groupName string, pods []PodCache) {
	group := podGroupCache[groupName]
	group.Pods = pods
	podGroupCache[groupName] = group
}

// HavePodList returns true if the listing of pods is not empty
func (p *PodGroupCache) HavePodList() bool {
	return len(p.Pods) > 0
}

// GetNextNode gets the next available node we can allocate for a group
func GetNextNode(groupName string) (string, error) {
	entry, ok := podGroupCache[groupName]

	if !ok {
		return "", fmt.Errorf("No entry for pod group %s in cache", groupName)
	}
	if len(entry.Pods) == 0 {
		return "", fmt.Errorf("Error while getting a node for pod group %s", groupName)
	}

	nodename := entry.Pods[0].NodeName

	// I think this is saying that when we've used up all the allocations
	// assigned for the group, we can clean it up from the pod group. We also
	// might be able to clean it up on the Cancel function?
	if entry.Pods[0].Tasks == 1 {
		slice := entry.Pods[1:]
		if len(slice) == 0 {
			delete(podGroupCache, groupName)
			return nodename, nil
		}

		// This means there are still allocations for the group?
		updatePodGroupList(groupName, slice)
		return nodename, nil
	}
	// Why are we subtracting one here?
	entry.Pods[0].Tasks = entry.Pods[0].Tasks - 1
	return nodename, nil
}
