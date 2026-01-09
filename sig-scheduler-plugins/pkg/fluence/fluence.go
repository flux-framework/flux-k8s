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

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"

	"sigs.k8s.io/scheduler-plugins/pkg/logger"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"

	fgroup "sigs.k8s.io/scheduler-plugins/pkg/fluence/group"
	flabel "sigs.k8s.io/scheduler-plugins/pkg/fluence/labels"

	corev1helpers "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/scheduler-plugins/apis/scheduling"
	"sigs.k8s.io/scheduler-plugins/apis/scheduling/v1alpha1"
	fcore "sigs.k8s.io/scheduler-plugins/pkg/fluence/core"
)

// Fluence schedules pods in a group using Fluxion as a backend
// We inherit cosched.Coscheduling to use some of the primary functions
type Fluence struct {
	mutex            sync.Mutex
	client           client.Client
	frameworkHandler framework.Handle
	podGroupManager  fcore.Manager
	scheduleTimeout  *time.Duration
	podGroupBackoff  *time.Duration
	log              *logger.DebugLogger
}

var (
	_ framework.QueueSortPlugin = &Fluence{}
	_ framework.PreFilterPlugin = &Fluence{}
	_ framework.FilterPlugin    = &Fluence{}

	_ framework.PostFilterPlugin = &Fluence{}
	_ framework.PermitPlugin     = &Fluence{}
	_ framework.ReservePlugin    = &Fluence{}

	_ framework.EnqueueExtensions = &Fluence{}

	// Set to be the same as coscheduling
	permitWaitingTimeSeconds int64 = 300
	podGroupBackoffSeconds   int64 = 0
)

const (
	// Name is the name of the plugin used in Registry and configurations.
	Name = "Fluence"
)

// Initialize and return a new Fluence Custom Scheduler Plugin
func New(_ context.Context, obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {

	ctx := context.TODO()

	// Make fluence his own little logger!
	// This can eventually be a flag, but just going to set for now
	// It shall be a very chonky file. Oh lawd he comin!
	l := logger.NewDebugLogger(logger.LevelDebug, "/tmp/fluence.log")

	scheme := runtime.NewScheme()
	_ = clientscheme.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	client, err := client.New(handle.KubeConfig(), client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	// Performance improvement when retrieving list of objects by namespace or we'll log 'index not exist' warning.
	fluxPodsInformer := handle.SharedInformerFactory().Core().V1().Pods().Informer()
	fluxPodsInformer.AddIndexers(cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})

	// PermitWaitingTimeSeconds is the waiting timeout in seconds.
	scheduleTimeDuration := time.Duration(permitWaitingTimeSeconds) * time.Second
	podGroupManager := fcore.NewPodGroupManager(
		client,
		handle.SnapshotSharedLister(),
		&scheduleTimeDuration,
		// Keep the podInformer (from frameworkHandle) as the single source of Pods.
		handle.SharedInformerFactory().Core().V1().Pods(),
		l,
	)

	// Event handlers to call on podGroupManager
	fluxPodsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: podGroupManager.UpdatePod,
		DeleteFunc: podGroupManager.DeletePod,
	})
	go fluxPodsInformer.Run(ctx.Done())

	backoffSeconds := time.Duration(podGroupBackoffSeconds) * time.Second
	plugin := &Fluence{
		frameworkHandler: handle,
		podGroupManager:  podGroupManager,
		scheduleTimeout:  &scheduleTimeDuration,
		log:              l,
		podGroupBackoff:  &backoffSeconds,
	}

	// TODO this is not supported yet
	// Account for resources in running cluster
	err = plugin.RegisterExisting(ctx)
	return plugin, err
}

func (fluence *Fluence) Name() string {
	return Name
}

// Fluence has added delete, although I wonder if update includes that signal
// and it's redundant?
func (fluence *Fluence) EventsToRegister() []framework.ClusterEventWithHint {
	// To register a custom event, follow the naming convention at:
	// https://github.com/kubernetes/kubernetes/pull/101394
	// Please follow: eventhandlers.go#L403-L410
	podGroupGVK := fmt.Sprintf("podgroups.v1alpha1.%v", scheduling.GroupName)
	return []framework.ClusterEventWithHint{
		{Event: framework.ClusterEvent{Resource: framework.Pod, ActionType: framework.Add | framework.Delete}},
		{Event: framework.ClusterEvent{Resource: framework.GVK(podGroupGVK), ActionType: framework.Add | framework.Update | framework.Delete}},
	}
}

// TODO we need to account for affinity here
func (fluence *Fluence) Filter(
	ctx context.Context,
	cycleState *framework.CycleState,
	pod *corev1.Pod,
	nodeInfo *framework.NodeInfo,
) *framework.Status {

	fluence.log.Verbose("[Fluence Filter] Filtering input node %s", nodeInfo.Node().Name)
	state, err := cycleState.Read(framework.StateKey(pod.Name))

	// No error means we retrieved the state
	if err == nil {

		// Try to convert the state to FluxStateDate
		value, ok := state.(*fcore.FluxStateData)

		// If we have state data that isn't equal to the current assignment, no go
		if ok && value.NodeName != nodeInfo.Node().Name {
			return framework.NewStatus(framework.Unschedulable, "pod is not permitted")
		} else {
			fluence.log.Info("[Fluence Filter] node %s selected for %s\n", value.NodeName, pod.Name)
		}
	}
	return framework.NewStatus(framework.Success)
}

// Less is used to sort pods in the scheduling queue in the following order.
// 1. Compare the priorities of Pods.
// 2. Compare the initialization timestamps of PodGroups or Pods.
// 3. Compare the keys of PodGroups/Pods: <namespace>/<podname>.
func (fluence *Fluence) Less(podInfo1, podInfo2 *framework.QueuedPodInfo) bool {
	prio1 := corev1helpers.PodPriority(podInfo1.Pod)
	prio2 := corev1helpers.PodPriority(podInfo2.Pod)
	if prio1 != prio2 {
		return prio1 > prio2
	}

	// Important: this GetPodGroup returns the first name as the Namespaced one,
	// which is what fluence needs to distinguish between namespaces. Just the
	// name could be replicated between different namespaces
	ctx := context.TODO()
	name1, podGroup1 := fluence.podGroupManager.GetPodGroup(ctx, podInfo1.Pod)
	name2, podGroup2 := fluence.podGroupManager.GetPodGroup(ctx, podInfo2.Pod)

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

// PreFilterExtensions allow for callbacks on filtered states
// This is required to be defined for a PreFilter plugin
// https://github.com/kubernetes/kubernetes/blob/master/pkg/scheduler/framework/interface.go#L383
func (fluence *Fluence) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

// PreFilter performs the following validations.
// 1. Whether the PodGroup that the Pod belongs to is on the deny list.
// 2. Whether the total number of pods in a PodGroup is less than its `minMember`.
func (fluence *Fluence) PreFilter(
	ctx context.Context,
	state *framework.CycleState,
	pod *corev1.Pod,
) (*framework.PreFilterResult, *framework.Status) {

	// Quick check if the pod is already scheduled
	fluence.mutex.Lock()
	node := fluence.podGroupManager.GetPodNode(pod)
	fluence.mutex.Unlock()
	if node != "" {
		fluence.log.Info("[Fluence PreFilter] assigned pod %s to node %s\n", pod.Name, node)
		result := framework.PreFilterResult{NodeNames: sets.New(node)}
		return &result, framework.NewStatus(framework.Success, "")
	}
	fluence.log.Info("[Fluence PreFilter] pod %s does not have a node assigned\n", pod.Name)

	// This will populate the node name into the pod group manager
	err := fluence.podGroupManager.PreFilter(ctx, pod, state)
	if err != nil {
		fluence.log.Error("[Fluence PreFilter] failed pod %s: %s", pod.Name, err.Error())
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	node = fluence.podGroupManager.GetPodNode(pod)
	result := framework.PreFilterResult{NodeNames: sets.New(node)}
	return &result, framework.NewStatus(framework.Success, "")
}

// PostFilter is used to reject a group of pods if a pod does not pass PreFilter or Filter.
func (fluence *Fluence) PostFilter(
	ctx context.Context,
	state *framework.CycleState,
	pod *corev1.Pod,
	filteredNodeStatusMap framework.NodeToStatusMap,
) (*framework.PostFilterResult, *framework.Status) {

	groupName, podGroup := fluence.podGroupManager.GetPodGroup(ctx, pod)
	if podGroup == nil {
		fluence.log.Info("Pod does not belong to any group, pod %s", pod.Name)
		return &framework.PostFilterResult{}, framework.NewStatus(framework.Unschedulable, "can not find pod group")
	}

	// This explicitly checks nodes, and we can skip scheduling another pod if we already
	// have the minimum. For fluence since we expect an exact size this likely is not needed
	assigned := fluence.podGroupManager.CalculateAssignedPods(podGroup.Name, pod.Namespace)
	if assigned >= int(podGroup.Spec.MinMember) {
		fluence.log.Info("Assigned pods podGroup %s is assigned %s", groupName, assigned)
		return &framework.PostFilterResult{}, framework.NewStatus(framework.Unschedulable)
	}

	// Took out percentage chcek here, doesn't make sense to me.

	// It's based on an implicit assumption: if the nth Pod failed,
	// it's inferrable other Pods belonging to the same PodGroup would be very likely to fail.
	fluence.frameworkHandler.IterateOverWaitingPods(func(waitingPod framework.WaitingPod) {
		if waitingPod.GetPod().Namespace == pod.Namespace && flabel.GetPodGroupLabel(waitingPod.GetPod()) == podGroup.Name {
			fluence.log.Info("PostFilter rejects the pod for podGroup %s and pod %s", groupName, waitingPod.GetPod().Name)
			waitingPod.Reject(fluence.Name(), "optimistic rejection in PostFilter")
		}
	})

	if fluence.podGroupBackoff != nil {
		pods, err := fluence.frameworkHandler.SharedInformerFactory().Core().V1().Pods().Lister().Pods(pod.Namespace).List(
			labels.SelectorFromSet(labels.Set{v1alpha1.PodGroupLabel: flabel.GetPodGroupLabel(pod)}),
		)
		if err == nil && len(pods) >= int(podGroup.Spec.MinMember) {
			fluence.podGroupManager.BackoffPodGroup(groupName, *fluence.podGroupBackoff)
		}
	}

	fluence.podGroupManager.DeletePermittedPodGroup(groupName)
	return &framework.PostFilterResult{}, framework.NewStatus(framework.Unschedulable,
		fmt.Sprintf("PodGroup %v gets rejected due to Pod %v is unschedulable even after PostFilter", groupName, pod.Name))
}

// Permit is the functions invoked by the framework at "Permit" extension point.
func (fluence *Fluence) Permit(
	ctx context.Context,
	state *framework.CycleState,
	pod *corev1.Pod,
	nodeName string,
) (*framework.Status, time.Duration) {

	fluence.log.Info("Checking permit for pod %s to node %s", pod.Name, nodeName)
	waitTime := *fluence.scheduleTimeout
	s := fluence.podGroupManager.Permit(ctx, state, pod)
	var retStatus *framework.Status
	switch s {
	case fcore.PodGroupNotSpecified:
		fluence.log.Info("Checking permit for pod %s to node %s: PodGroupNotSpecified", pod.Name, nodeName)
		return framework.NewStatus(framework.Success, ""), 0
	case fcore.PodGroupNotFound:
		fluence.log.Info("Checking permit for pod %s to node %s: PodGroupNotFound", pod.Name, nodeName)
		return framework.NewStatus(framework.Unschedulable, "PodGroup not found"), 0
	case fcore.Wait:
		fluence.log.Info("Pod %s is waiting to be scheduled to node %s", pod.Name, nodeName)
		_, podGroup := fluence.podGroupManager.GetPodGroup(ctx, pod)
		if wait := fgroup.GetWaitTimeDuration(podGroup, fluence.scheduleTimeout); wait != 0 {
			waitTime = wait
		}
		retStatus = framework.NewStatus(framework.Wait)

		// We will also request to move the sibling pods back to activeQ.
		fluence.podGroupManager.ActivateSiblings(pod, state)
	case fcore.Success:
		podGroupFullName := flabel.GetPodGroupFullName(pod)
		fluence.frameworkHandler.IterateOverWaitingPods(func(waitingPod framework.WaitingPod) {
			if flabel.GetPodGroupFullName(waitingPod.GetPod()) == podGroupFullName {
				fluence.log.Info("Permit allows pod %s", waitingPod.GetPod().Name)
				waitingPod.Allow(fluence.Name())
			}
		})
		fluence.log.Info("Permit allows pod %s", pod.Name)
		retStatus = framework.NewStatus(framework.Success)
		waitTime = 0
	}

	return retStatus, waitTime
}

// Reserve is the functions invoked by the framework at "reserve" extension point.
func (fluence *Fluence) Reserve(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	return nil
}

// Unreserve rejects all other Pods in the PodGroup when one of the pods in the group times out.
func (fluence *Fluence) Unreserve(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) {
	groupName, podGroup := fluence.podGroupManager.GetPodGroup(ctx, pod)
	if podGroup == nil {
		return
	}
	fluence.frameworkHandler.IterateOverWaitingPods(func(waitingPod framework.WaitingPod) {
		if waitingPod.GetPod().Namespace == pod.Namespace && flabel.GetPodGroupLabel(waitingPod.GetPod()) == podGroup.Name {
			fluence.log.Info("Unreserve rejects pod %s in group %s", waitingPod.GetPod().Name, groupName)
			waitingPod.Reject(fluence.Name(), "rejection in Unreserve")
		}
	})
	fluence.podGroupManager.DeletePermittedPodGroup(groupName)
}
