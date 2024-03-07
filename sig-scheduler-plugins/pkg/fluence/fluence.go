/*
Copyright 2020 The Kubernetes Authors.

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

package fluence

import (
	"context"
	"fmt"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"

	fgroup "sigs.k8s.io/scheduler-plugins/pkg/fluence/group"
	label "sigs.k8s.io/scheduler-plugins/pkg/fluence/labels"

	corev1helpers "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/scheduler-plugins/pkg/util"

	"sigs.k8s.io/scheduler-plugins/apis/config"
	"sigs.k8s.io/scheduler-plugins/apis/scheduling"
	"sigs.k8s.io/scheduler-plugins/apis/scheduling/v1alpha1"
	fcore "sigs.k8s.io/scheduler-plugins/pkg/fluence/core"
)

// Fluence schedules pods in a group using Fluxion as a backend
// We inherit cosched.Coscheduling to use some of the primary functions
type Fluence struct {
	mutex  sync.Mutex
	client client.Client

	// Store jobid on the level of a group (which can be a single pod)
	groupToJobId map[string]uint64

	frameworkHandler framework.Handle
	pgMgr            fcore.Manager
	scheduleTimeout  *time.Duration
	pgBackoff        *time.Duration
}

var (
	_ framework.QueueSortPlugin   = &Fluence{}
	_ framework.PreFilterPlugin   = &Fluence{}
	_ framework.PostFilterPlugin  = &Fluence{} // Here down are from coscheduling
	_ framework.PermitPlugin      = &Fluence{}
	_ framework.ReservePlugin     = &Fluence{}
	_ framework.EnqueueExtensions = &Fluence{}
)

const (
	// Name is the name of the plugin used in Registry and configurations.
	Name = "Fluence"
)

// Initialize and return a new Fluence Custom Scheduler Plugin
func New(obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {

	// Keep these empty for now, use defaults
	args := config.CoschedulingArgs{}

	scheme := runtime.NewScheme()
	_ = clientscheme.AddToScheme(scheme)
	_ = v1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	client, err := client.New(handle.KubeConfig(), client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	// Performance improvement when retrieving list of objects by namespace or we'll log 'index not exist' warning.
	handle.SharedInformerFactory().Core().V1().Pods().Informer().AddIndexers(cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})

	// PermitWaitingTimeSeconds is the waiting timeout in seconds.
	scheduleTimeDuration := time.Duration(args.PermitWaitingTimeSeconds) * time.Second
	pgMgr := fcore.NewPodGroupManager(
		client,
		handle.SnapshotSharedLister(),
		&scheduleTimeDuration,
		// Keep the podInformer (from frameworkHandle) as the single source of Pods.
		handle.SharedInformerFactory().Core().V1().Pods(),
	)

	// The main difference here is adding the groupToJobId lookup
	plugin := &Fluence{
		frameworkHandler: handle,
		pgMgr:            pgMgr,
		scheduleTimeout:  &scheduleTimeDuration,
		groupToJobId:     make(map[string]uint64),
	}

	// PodGroupBackoffSeconds:  backoff time in seconds before a pod group can be scheduled again.
	if args.PodGroupBackoffSeconds < 0 {
		err := fmt.Errorf("parse arguments failed")
		klog.ErrorS(err, "PodGroupBackoffSeconds cannot be negative")
		return nil, err
	} else if args.PodGroupBackoffSeconds > 0 {
		pgBackoff := time.Duration(args.PodGroupBackoffSeconds) * time.Second
		plugin.pgBackoff = &pgBackoff
	}
	return plugin, nil
}

func (f *Fluence) Name() string {
	return Name
}

// Fluence has added delete, although I wonder if update includes that signal
// and it's redundant?
func (f *Fluence) EventsToRegister() []framework.ClusterEventWithHint {
	// To register a custom event, follow the naming convention at:
	// https://git.k8s.io/kubernetes/pkg/scheduler/eventhandlers.go#L403-L410
	pgGVK := fmt.Sprintf("podgroups.v1alpha1.%v", scheduling.GroupName)
	return []framework.ClusterEventWithHint{
		{Event: framework.ClusterEvent{Resource: framework.Pod, ActionType: framework.Add | framework.Delete}},
		{Event: framework.ClusterEvent{Resource: framework.GVK(pgGVK), ActionType: framework.Add | framework.Update | framework.Delete}},
	}
}

// Less is used to sort pods in the scheduling queue in the following order.
// 1. Compare the priorities of Pods.
// 2. Compare the initialization timestamps of PodGroups or Pods.
// 3. Compare the keys of PodGroups/Pods: <namespace>/<podname>.
func (f *Fluence) Less(podInfo1, podInfo2 *framework.QueuedPodInfo) bool {
	klog.Infof("ordering pods in fluence scheduler plugin")
	prio1 := corev1helpers.PodPriority(podInfo1.Pod)
	prio2 := corev1helpers.PodPriority(podInfo2.Pod)
	if prio1 != prio2 {
		return prio1 > prio2
	}

	// Important: this GetPodGroup returns the first name as the Namespaced one,
	// which is what fluence needs to distinguish between namespaces. Just the
	// name could be replicated between different namespaces
	ctx := context.TODO()
	name1, podGroup1 := f.pgMgr.GetPodGroup(ctx, podInfo1.Pod)
	name2, podGroup2 := f.pgMgr.GetPodGroup(ctx, podInfo2.Pod)

	// Fluence can only compare if we have two known groups.
	// This tries for that first, and falls back to the initial attempt timestamp
	creationTime1 := fgroup.GetCreationTimestamp(name1, podGroup1, podInfo1)
	creationTime2 := fgroup.GetCreationTimestamp(name2, podGroup2, podInfo2)

	// If they are the same, fall back to sorting by name.
	if creationTime1.Equal(&creationTime2) {
		return fcore.GetNamespacedName(podInfo1.Pod) < fcore.GetNamespacedName(podInfo2.Pod)
	}
	return creationTime1.Before(&creationTime2)

}

// PreFilter performs the following validations.
// 1. Whether the PodGroup that the Pod belongs to is on the deny list.
// 2. Whether the total number of pods in a PodGroup is less than its `minMember`.
func (f *Fluence) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) (*framework.PreFilterResult, *framework.Status) {
	// If PreFilter fails, return framework.UnschedulableAndUnresolvable to avoid
	// any preemption attempts.
	if err := f.pgMgr.PreFilter(ctx, pod); err != nil {
		klog.ErrorS(err, "PreFilter failed", "pod", klog.KObj(pod))
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	return nil, framework.NewStatus(framework.Success, "")
}

// PostFilter is used to reject a group of pods if a pod does not pass PreFilter or Filter.
func (f *Fluence) PostFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod,
	filteredNodeStatusMap framework.NodeToStatusMap) (*framework.PostFilterResult, *framework.Status) {
	pgName, pg := f.pgMgr.GetPodGroup(ctx, pod)
	if pg == nil {
		klog.V(4).InfoS("Pod does not belong to any group", "pod", klog.KObj(pod))
		return &framework.PostFilterResult{}, framework.NewStatus(framework.Unschedulable, "can not find pod group")
	}

	// This indicates there are already enough Pods satisfying the PodGroup,
	// so don't bother to reject the whole PodGroup.
	assigned := f.pgMgr.CalculateAssignedPods(pg.Name, pod.Namespace)
	if assigned >= int(pg.Spec.MinMember) {
		klog.V(4).InfoS("Assigned pods", "podGroup", klog.KObj(pg), "assigned", assigned)
		return &framework.PostFilterResult{}, framework.NewStatus(framework.Unschedulable)
	}

	// If the gap is less than/equal 10%, we may want to try subsequent Pods
	// to see they can satisfy the PodGroup
	notAssignedPercentage := float32(int(pg.Spec.MinMember)-assigned) / float32(pg.Spec.MinMember)
	if notAssignedPercentage <= 0.1 {
		klog.V(4).InfoS("A small gap of pods to reach the quorum", "podGroup", klog.KObj(pg), "percentage", notAssignedPercentage)
		return &framework.PostFilterResult{}, framework.NewStatus(framework.Unschedulable)
	}

	// It's based on an implicit assumption: if the nth Pod failed,
	// it's inferrable other Pods belonging to the same PodGroup would be very likely to fail.
	f.frameworkHandler.IterateOverWaitingPods(func(waitingPod framework.WaitingPod) {
		if waitingPod.GetPod().Namespace == pod.Namespace && label.GetPodGroupLabel(waitingPod.GetPod()) == pg.Name {
			klog.V(3).InfoS("PostFilter rejects the pod", "podGroup", klog.KObj(pg), "pod", klog.KObj(waitingPod.GetPod()))
			waitingPod.Reject(f.Name(), "optimistic rejection in PostFilter")
		}
	})

	if f.pgBackoff != nil {
		pods, err := f.frameworkHandler.SharedInformerFactory().Core().V1().Pods().Lister().Pods(pod.Namespace).List(
			labels.SelectorFromSet(labels.Set{v1alpha1.PodGroupLabel: label.GetPodGroupLabel(pod)}),
		)
		if err == nil && len(pods) >= int(pg.Spec.MinMember) {
			f.pgMgr.BackoffPodGroup(pgName, *f.pgBackoff)
		}
	}

	f.pgMgr.DeletePermittedPodGroup(pgName)
	return &framework.PostFilterResult{}, framework.NewStatus(framework.Unschedulable,
		fmt.Sprintf("PodGroup %v gets rejected due to Pod %v is unschedulable even after PostFilter", pgName, pod.Name))
}

// PreFilterExtensions returns a PreFilterExtensions interface if the plugin implements one.
func (f *Fluence) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

// Permit is the functions invoked by the framework at "Permit" extension point.
func (f *Fluence) Permit(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (*framework.Status, time.Duration) {
	waitTime := *f.scheduleTimeout
	s := f.pgMgr.Permit(ctx, pod)
	var retStatus *framework.Status
	switch s {
	case fcore.PodGroupNotSpecified:
		return framework.NewStatus(framework.Success, ""), 0
	case fcore.PodGroupNotFound:
		return framework.NewStatus(framework.Unschedulable, "PodGroup not found"), 0
	case fcore.Wait:
		klog.InfoS("Pod is waiting to be scheduled to node", "pod", klog.KObj(pod), "nodeName", nodeName)
		_, pg := f.pgMgr.GetPodGroup(ctx, pod)

		// Note this is in seconds, defaults to 60 seconds
		if wait := util.GetWaitTimeDuration(pg, f.scheduleTimeout); wait != 0 {
			waitTime = wait
		}
		retStatus = framework.NewStatus(framework.Wait)
		// We will also request to move the sibling pods back to activeQ.
		f.pgMgr.ActivateSiblings(pod, state)
	case fcore.Success:
		pgFullName := label.GetPodGroupFullName(pod)
		f.frameworkHandler.IterateOverWaitingPods(func(waitingPod framework.WaitingPod) {
			if label.GetPodGroupFullName(waitingPod.GetPod()) == pgFullName {
				klog.V(3).InfoS("Permit allows", "pod", klog.KObj(waitingPod.GetPod()))
				waitingPod.Allow(f.Name())
			}
		})
		klog.V(3).InfoS("Permit allows", "pod", klog.KObj(pod))
		retStatus = framework.NewStatus(framework.Success)
		waitTime = 0
	}

	return retStatus, waitTime
}

// Reserve is the functions invoked by the framework at "reserve" extension point.
func (f *Fluence) Reserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	return nil
}

// Unreserve rejects all other Pods in the PodGroup when one of the pods in the group times out.
func (f *Fluence) Unreserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) {
	pgName, pg := f.pgMgr.GetPodGroup(ctx, pod)
	if pg == nil {
		return
	}
	f.frameworkHandler.IterateOverWaitingPods(func(waitingPod framework.WaitingPod) {
		if waitingPod.GetPod().Namespace == pod.Namespace && label.GetPodGroupLabel(waitingPod.GetPod()) == pg.Name {
			klog.V(3).InfoS("Unreserve rejects", "pod", klog.KObj(waitingPod.GetPod()), "podGroup", klog.KObj(pg))
			waitingPod.Reject(f.Name(), "rejection in Unreserve")
		}
	})
	f.pgMgr.DeletePermittedPodGroup(pgName)
}
