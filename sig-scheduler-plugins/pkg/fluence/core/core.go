package core

import (
	"fmt"

	klog "k8s.io/klog/v2"

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

// NewFluxState creates an entry for the CycleState with the node and group name
func NewFluxState(nodeName string, groupName string) *FluxStateData {
	cache := NodeCache{NodeName: nodeName}
	return &FluxStateData{NodeCache: cache}
}

// NodeCache holds the node name and tasks for the node
// For the PodGroupCache, these are organized by group name,
// and there is a list of them
type NodeCache struct {
	NodeName string

	// Tie assignment back to PodGroup, which can be used to get size and time created
	GroupName string

	// Assigned tasks (often pods) to nodes
	// https://github.com/flux-framework/flux-k8s/blob/9f24f36752e3cced1b1112d93bfa366fb58b3c84/src/fluence/fluxion/fluxion.go#L94-L97
	AssignedTasks int
}

// A pod group cache holds a list of nodes for an allocation, where each has some number of tasks
// along with the expected group size. This is intended to replace PodGroup
// given the group name, size (derived from annotations) and timestamp
type PodGroupCache struct {
	GroupName string

	// This is a cache of nodes for pods
	Nodes []NodeCache
}

// PodGroups seen by fluence
var groupsSeen map[string]*PodGroupCache

// Init populates the groupsSeen cache
func Init() {
	groupsSeen = map[string]*PodGroupCache{}
}

// GetFluenceCache determines if a group has been seen.
// Yes -> we return the PodGroupCache entry
// No -> the entry is nil / does not exist
func GetFluenceCache(groupName string) *PodGroupCache {
	entry, _ := groupsSeen[groupName]
	return entry
}

// DeletePodGroup deletes a pod from the group cache
func DeletePodGroup(groupName string) {
	delete(groupsSeen, groupName)
}

// CreateNodePodsList creates a list of node pod caches
func CreateNodeList(nodelist []*pb.NodeAlloc, groupName string) (nodepods []NodeCache) {

	// Create a pod cache for each node
	nodepods = make([]NodeCache, len(nodelist))

	// TODO: should we be integrating topology information here? Could it be the
	// case that some nodes (pods) in the group should be closer?
	for i, v := range nodelist {
		nodepods[i] = NodeCache{
			NodeName:      v.GetNodeID(),
			AssignedTasks: int(v.GetTasks()),
			GroupName:     groupName,
		}
	}

	// Update the pods in the PodGroupCache (groupsSeen)
	updatePodGroupCache(groupName, nodepods)
	return nodepods
}

// updatePodGroupList updates the PodGroupCache with a listing of nodes
func updatePodGroupCache(groupName string, nodes []NodeCache) {
	cache := PodGroupCache{
		Nodes:     nodes,
		GroupName: groupName,
	}
	groupsSeen[groupName] = &cache
}

// GetNextNode gets the next node in the PodGroupCache
func (p *PodGroupCache) GetNextNode() (string, error) {

	nextnode := ""

	// Quick failure state - we ran out of nodes
	if len(p.Nodes) == 0 {
		return nextnode, fmt.Errorf("[Fluence] PodGroup %s ran out of nodes.", p.GroupName)
	}

	// The next is the 0th in the list
	nextnode = p.Nodes[0].NodeName
	klog.Infof("[Fluence] Next node for group %s is %s", p.GroupName, nextnode)

	// If there is only one task left, we are going to use it (and remove the node)
	if p.Nodes[0].AssignedTasks == 1 {
		klog.Infof("[Fluence] First node has one remaining task slot")
		slice := p.Nodes[1:]

		// If after we remove the node there are no nodes left...
		// Note that I'm not deleting the node from the cache because that is the
		// only way fluence knows it has already assigned work (presence of the key)
		if len(slice) == 0 {
			klog.Infof("[Fluence] Assigning node %s. There are NO reamining nodes for group %s\n", nextnode, p.GroupName)
			// delete(podGroupCache, groupName)
			return nextnode, nil
		}

		klog.Infof("[Fluence] Assigning node %s. There are nodes left for group", nextnode, p.GroupName)
		updatePodGroupCache(p.GroupName, slice)
		return nextnode, nil
	}

	// If we get here the first node had >1 assigned tasks
	klog.Infof("[Fluence] Assigning node %s for group %s. There are still task assignments available for this node.", nextnode, p.GroupName)
	p.Nodes[0].AssignedTasks = p.Nodes[0].AssignedTasks - 1
	return nextnode, nil
}

// GetNextNode gets the next available node we can allocate for a group
// TODO this should be able to take and pass forward a number of tasks.
// It is implicity 1 now, but doesn't have to be.
func GetNextNode(groupName string) (string, error) {

	// Get our entry from the groupsSeen cache
	klog.Infof("[Fluence] groups seen %s", groupsSeen)
	entry, ok := groupsSeen[groupName]

	// This case should not happen
	if !ok {
		return "", fmt.Errorf("[Fluence] Map is empty")
	}
	// Get the next node from the PodGroupCache
	return entry.GetNextNode()
}
