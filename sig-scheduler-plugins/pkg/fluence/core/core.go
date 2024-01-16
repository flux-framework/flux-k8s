package core

import (
	"fmt"

	v1 "k8s.io/api/core/v1"

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
	Nodes []NodeCache
	Size  int32
	Name  string

	// Keep track of when the group was initially created!
	// This is like, the main thing we need.
	TimeCreated metav1.MicroTime
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

	if !ok {

		// Assume we create the group with the timestamp
		// of the first pod seen. There might be imperfections
		// by the second, but as long as we sort them via millisecond
		// this should prevent interleaving
		nodes := []NodeCache{}

		// Create the new entry for the pod group
		entry = &PodGroupCache{
			Name:        groupName,
			Size:        groupSize,
			Nodes:       nodes,
			TimeCreated: metav1.NowMicro(),
		}

		// Tell the user when it was created
		fmt.Printf("[Fluence] Pod group %s was created at %s\n", entry.Name, entry.TimeCreated)
	}

	// If the size has changed, we currently do not allow updating it.
	// We issue a warning. In the future this could be supported with a grow command.
	if entry.Size != groupSize {
		fmt.Printf("[Fluence] Pod group %s request to change size from %s to %s is not yet supported\n", groupName, entry.Size, groupSize)
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
	fmt.Printf("[Fluence] Pod group cache updated with nodes\n", podGroupCache)
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
}

// GetNextNode gets the next available node we can allocate for a group
func GetNextNode(groupName string) (string, error) {
	entry, ok := podGroupCache[groupName]
	if !ok {
		err := fmt.Errorf("[Fluence] Map is empty\n")
		return "", err
	}
	if len(entry.Nodes) == 0 {
		err := fmt.Errorf("[Fluence] Error while getting a node\n")
		return "", err
	}

	nodename := entry.Nodes[0].NodeName
	fmt.Printf("[Fluence] Next node for group %s is %s", groupName, nodename)

	if entry.Nodes[0].Tasks == 1 {
		fmt.Println("[Fluence] First node has one task")
		slice := entry.Nodes[1:]
		if len(slice) == 0 {
			fmt.Printf("[Fluence] After this node, the slice is empty, deleting group %s from cache\n", groupName)
			delete(podGroupCache, groupName)
			return nodename, nil
		}
		fmt.Println("[Fluence] After this node, the slide still has nodes")
		updatePodGroupNodes(groupName, slice)
		return nodename, nil
	}
	fmt.Println("[Fluence] Subtracting one task from first node")
	entry.Nodes[0].Tasks = entry.Nodes[0].Tasks - 1
	return nodename, nil
}
