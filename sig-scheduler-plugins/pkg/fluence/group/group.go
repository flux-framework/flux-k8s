package group

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klog "k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	sched "sigs.k8s.io/scheduler-plugins/apis/scheduling/v1alpha1"
)

// GetCreationTimestamp first tries the fluence group, then falls back to the initial attempt timestamp
// This is the only update we have made to the upstream PodGroupManager, because we are expecting
// a MicroTime and not a time.Time.
func GetCreationTimestamp(groupName string, pg *sched.PodGroup, podInfo *framework.QueuedPodInfo) metav1.MicroTime {

	// IsZero is an indicator if this was actually set
	// If the group label was present and we have a group, this will be true
	if !pg.Status.ScheduleStartTime.IsZero() {
		klog.Infof("   [Fluence] Pod group %s was created at %s\n", groupName, pg.Status.ScheduleStartTime)
		return pg.Status.ScheduleStartTime
	}
	// We should actually never get here.
	klog.Errorf("   [Fluence] Pod group %s time IsZero, we should not have reached here", groupName)
	return metav1.NewMicroTime(*podInfo.InitialAttemptTimestamp)
}
