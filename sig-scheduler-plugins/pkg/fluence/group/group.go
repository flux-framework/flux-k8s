package group

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klog "k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	sched "sigs.k8s.io/scheduler-plugins/apis/scheduling/v1alpha1"
)

// DefaultWaitTime is 60s if ScheduleTimeoutSeconds is not specified.
const DefaultWaitTime = 60 * time.Second

// CreateFakeGroup wraps an arbitrary pod in a fake group for fluence to schedule
// This happens only in PreFilter so we already sorted
func CreateFakeGroup(pod *corev1.Pod) *sched.PodGroup {
	groupName := fmt.Sprintf("fluence-solo-%s-%s", pod.Namespace, pod.Name)
	return &sched.PodGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      groupName,
			Namespace: pod.Namespace,
		},
		Spec: sched.PodGroupSpec{MinMember: int32(1)},
	}
}

// GetCreationTimestamp first tries the fluence group, then falls back to the initial attempt timestamp
// This is the only update we have made to the upstream PodGroupManager, because we are expecting
// a MicroTime and not a time.Time.
func GetCreationTimestamp(groupName string, pg *sched.PodGroup, podInfo *framework.QueuedPodInfo) metav1.MicroTime {

	// Don't try to get a time for a pod group that does not exist
	if pg == nil {
		return metav1.NewMicroTime(*podInfo.InitialAttemptTimestamp)
	}

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

// GetWaitTimeDuration returns a wait timeout based on the following precedences:
// 1. spec.scheduleTimeoutSeconds of the given pg, if specified
// 2. given scheduleTimeout, if not nil
// 3. fall back to DefaultWaitTime
func GetWaitTimeDuration(pg *sched.PodGroup, scheduleTimeout *time.Duration) time.Duration {
	if pg != nil && pg.Spec.ScheduleTimeoutSeconds != nil {
		return time.Duration(*pg.Spec.ScheduleTimeoutSeconds) * time.Second
	}
	if scheduleTimeout != nil && *scheduleTimeout != 0 {
		return *scheduleTimeout
	}
	return DefaultWaitTime
}
