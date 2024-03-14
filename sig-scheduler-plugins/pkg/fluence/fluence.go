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

	"k8s.io/apimachinery/pkg/util/sets"
	klog "k8s.io/klog/v2"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"

	fgroup "sigs.k8s.io/scheduler-plugins/pkg/fluence/group"

	corev1helpers "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/scheduler-plugins/apis/config"
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
	pgMgr            fcore.Manager
	scheduleTimeout  *time.Duration
}

var (
	_ framework.QueueSortPlugin = &Fluence{}
	_ framework.PreFilterPlugin = &Fluence{}
	_ framework.FilterPlugin    = &Fluence{}
)

const (
	// Name is the name of the plugin used in Registry and configurations.
	Name = "Fluence"
)

// Initialize and return a new Fluence Custom Scheduler Plugin
func New(obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {

	// Keep these empty for now, use defaults
	args := config.CoschedulingArgs{}
	ctx := context.TODO()

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
	scheduleTimeDuration := time.Duration(args.PermitWaitingTimeSeconds) * time.Second
	pgMgr := fcore.NewPodGroupManager(
		client,
		handle.SnapshotSharedLister(),
		&scheduleTimeDuration,
		// Keep the podInformer (from frameworkHandle) as the single source of Pods.
		handle.SharedInformerFactory().Core().V1().Pods(),
	)

	// Event handlers to call on pgMgr
	fluxPodsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: pgMgr.UpdatePod,
		DeleteFunc: pgMgr.DeletePod,
	})
	go fluxPodsInformer.Run(ctx.Done())

	plugin := &Fluence{
		frameworkHandler: handle,
		pgMgr:            pgMgr,
		scheduleTimeout:  &scheduleTimeDuration,
	}
	return plugin, nil
}

func (f *Fluence) Name() string {
	return Name
}

// Fluence has added delete, although I wonder if update includes that signal
// and it's redundant?
func (f *Fluence) EventsToRegister() []framework.ClusterEventWithHint {
	// TODO I have not redone this yet, not sure what it does (it might replace our informer above)
	// To register a custom event, follow the naming convention at:
	// https://git.k8s.io/kubernetes/pkg/scheduler/eventhandlers.go#L403-L410
	pgGVK := fmt.Sprintf("podgroups.v1alpha1.%v", scheduling.GroupName)
	return []framework.ClusterEventWithHint{
		{Event: framework.ClusterEvent{Resource: framework.Pod, ActionType: framework.Add | framework.Delete}},
		{Event: framework.ClusterEvent{Resource: framework.GVK(pgGVK), ActionType: framework.Add | framework.Update | framework.Delete}},
	}
}

// TODO we need to account for affinity here
func (f *Fluence) Filter(
	ctx context.Context,
	cycleState *framework.CycleState,
	pod *corev1.Pod,
	nodeInfo *framework.NodeInfo,
) *framework.Status {

	klog.Info("Filtering input node ", nodeInfo.Node().Name)
	state, err := cycleState.Read(framework.StateKey(pod.Name))

	// No error means we retrieved the state
	if err == nil {

		// Try to convert the state to FluxStateDate
		value, ok := state.(*fcore.FluxStateData)

		// If we have state data that isn't equal to the current assignment, no go
		if ok && value.NodeName != nodeInfo.Node().Name {
			return framework.NewStatus(framework.Unschedulable, "pod is not permitted")
		} else {
			klog.Infof("Filter: node %s selected for %s\n", value.NodeName, pod.Name)
		}
	}
	return framework.NewStatus(framework.Success)
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

// PreFilterExtensions allow for callbacks on filtered states
// This is required to be defined for a PreFilter plugin
// https://github.com/kubernetes/kubernetes/blob/master/pkg/scheduler/framework/interface.go#L383
func (f *Fluence) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

// PreFilter performs the following validations.
// 1. Whether the PodGroup that the Pod belongs to is on the deny list.
// 2. Whether the total number of pods in a PodGroup is less than its `minMember`.
func (f *Fluence) PreFilter(
	ctx context.Context,
	state *framework.CycleState,
	pod *corev1.Pod,
) (*framework.PreFilterResult, *framework.Status) {

	// Quick check if the pod is already scheduled
	f.mutex.Lock()
	node := f.pgMgr.GetPodNode(pod)
	f.mutex.Unlock()
	if node != "" {
		result := framework.PreFilterResult{NodeNames: sets.New(node)}
		return &result, framework.NewStatus(framework.Success, "")
	}
	// This will populate the node name into the pod group manager
	err := f.pgMgr.PreFilter(ctx, pod, state)
	if err != nil {
		klog.ErrorS(err, "PreFilter failed", "pod", klog.KObj(pod))
		return nil, framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	node = f.pgMgr.GetPodNode(pod)
	result := framework.PreFilterResult{NodeNames: sets.New(node)}
	return &result, framework.NewStatus(framework.Success, "")
}
