package fluence

import (
	"fmt"
	"strconv"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	fcore "sigs.k8s.io/scheduler-plugins/pkg/fluence/core"
)

const (
	PodGroupNameLabel = "fluence.pod-group"
	PodGroupSizeLabel = "fluence.group-size"
)

// getDefaultGroupName returns a group name based on the pod namespace and name
// We could do this for pods that are not labeled, and treat them as a size 1 group
func (f *Fluence) getDefaultGroupName(pod *v1.Pod) string {
	return fmt.Sprintf("%s-%s", pod.Namespace, pod.Name)
}

// getPodsGroup gets the pods group, if it exists.
func (f *Fluence) getPodsGroup(pod *v1.Pod) *fcore.PodGroupCache {
	groupName := f.ensureFluenceGroup(pod)
	return fcore.GetPodGroup(groupName)
}

// ensureFluenceGroup ensure that a podGroup is created for the named fluence group
// Preference goes to the traditional PodGroup (created by the user)
// and falls back to having one created by fluence. If there is no PodGroup
// created and no fluence annotation, we do not create the group.
// Likely for fluence we'd want a cleanup function somehow too,
// for now assume groups are unique by name.
func (f *Fluence) ensureFluenceGroup(pod *v1.Pod) string {

	// Get the group name and size from the fluence labels
	groupName := f.getFluenceGroupName(pod)
	groupSize := f.getFluenceGroupSize(pod)

	// If there isn't a group, make a single node sized group
	// This is so we can always treat the cases equally
	if groupName == "" {
		klog.Infof("  [Fluence] Group annotation missing for pod %s", pod.Name)
		groupName = f.getDefaultGroupName(pod)
	}
	klog.Infof("  [Fluence] Group name for %s is %s", pod.Name, groupName)
	klog.Infof("  [Fluence] Group size for %s is %d", pod.Name, groupSize)

	// Register the pod group (with the pod) in our cache
	fcore.RegisterPodGroup(pod, groupName, groupSize)
	return groupName
}

// deleteFluenceGroup ensures the pod group is deleted, if it exists
func (f *Fluence) deleteFluenceGroup(pod *v1.Pod) {

	// Get the group name and size from the fluence labels
	pg := f.getPodsGroup(pod)
	fcore.DeletePodGroup(pg.Name)
}

// getFluenceGroupName looks for the group to indicate a fluence group, and returns it
func (f *Fluence) getFluenceGroupName(pod *v1.Pod) string {
	groupName, _ := pod.Labels[PodGroupNameLabel]
	return groupName
}

// getFluenceGroupSize gets the size of the fluence group
func (f *Fluence) getFluenceGroupSize(pod *v1.Pod) int32 {
	size, _ := pod.Labels[PodGroupSizeLabel]

	// Default size of 1 if the label is not set (but name is)
	if size == "" {
		return 1
	}

	// We don't want the scheduler to fail if someone puts a value for size
	// that doesn't convert nicely. They can find this in the logs.
	intSize, err := strconv.ParseUint(size, 10, 32)
	if err != nil {
		klog.Error("  [Fluence] Parsing integer size for pod group")
	}
	return int32(intSize)
}

// getCreationTimestamp first tries the fluence group, then falls back to the initial attempt timestamp
func (f *Fluence) getCreationTimestamp(groupName string, podInfo *framework.QueuedPodInfo) metav1.MicroTime {
	pg := fcore.GetPodGroup(groupName)

	// IsZero is an indicator if this was actually set
	// If the group label was present and we have a group, this will be true
	if !pg.TimeCreated.IsZero() {
		klog.Infof("  [Fluence] Pod group %s was created at %s\n", groupName, pg.TimeCreated)
		return pg.TimeCreated
	}
	// We should actually never get here.
	klog.Errorf("  [Fluence] Pod group %s time IsZero, we should not have reached here", groupName)
	return metav1.NewMicroTime(*podInfo.InitialAttemptTimestamp)
}
