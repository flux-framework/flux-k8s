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

package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	gochache "github.com/patrickmn/go-cache"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	informerv1 "k8s.io/client-go/informers/core/v1"
	listerv1 "k8s.io/client-go/listers/core/v1"
	klog "k8s.io/klog/v2"

	"k8s.io/kubernetes/pkg/scheduler/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/scheduler-plugins/apis/scheduling/v1alpha1"
	"sigs.k8s.io/scheduler-plugins/pkg/logger"
	"sigs.k8s.io/scheduler-plugins/pkg/util"
)

type Status string

const (
	// PodGroupNotSpecified denotes no PodGroup is specified in the Pod spec.
	PodGroupNotSpecified Status = "PodGroup not specified"
	// PodGroupNotFound denotes the specified PodGroup in the Pod spec is
	// not found in API server.
	PodGroupNotFound Status = "PodGroup not found"
	Success          Status = "Success"
	Wait             Status = "Wait"

	permitStateKey = "PermitFluence"
)

// TODO should eventually store group name here to reassociate on reload
type FluxStateData struct {
	NodeName string
}

type PermitState struct {
	Activate bool
}

func (s *PermitState) Clone() framework.StateData {
	return &PermitState{Activate: s.Activate}
}

func (s *FluxStateData) Clone() framework.StateData {
	clone := &FluxStateData{
		NodeName: s.NodeName,
	}
	return clone
}

// Manager defines the interfaces for PodGroup management.
type Manager interface {
	PreFilter(context.Context, *corev1.Pod, *framework.CycleState) error
	GetPodNode(*corev1.Pod) string
	GetPodGroup(context.Context, *corev1.Pod) (string, *v1alpha1.PodGroup)
	GetCreationTimestamp(*corev1.Pod, time.Time) time.Time
	DeletePermittedPodGroup(string)
	Permit(context.Context, *framework.CycleState, *corev1.Pod) Status
	CalculateAssignedPods(string, string) int
	ActivateSiblings(pod *corev1.Pod, state *framework.CycleState)
	BackoffPodGroup(string, time.Duration)
}

// PodGroupManager defines the scheduling operation called
type PodGroupManager struct {
	// client is a generic controller-runtime client to manipulate both core resources and PodGroups.
	client client.Client
	// snapshotSharedLister is pod shared list
	snapshotSharedLister framework.SharedLister
	// scheduleTimeout is the default timeout for podgroup scheduling.
	// If podgroup's scheduleTimeoutSeconds is set, it will be used.
	scheduleTimeout *time.Duration
	// permittedpodGroup stores the podgroup name which has passed the pre resource check.
	permittedpodGroup *gochache.Cache
	// backedOffpodGroup stores the podgorup name which failed scheduling recently.
	backedOffpodGroup *gochache.Cache
	// podLister is pod lister
	podLister listerv1.PodLister

	// This isn't great to save state, but we can improve upon it
	// we should have a way to load jobids into this if fluence is recreated
	// If we can annotate them in fluxion and query for that, we can!
	groupToJobId map[string]uint64
	podToNode    map[string]string

	// Probably should just choose one... oh well
	sync.RWMutex
	mutex sync.Mutex
	log   *logger.DebugLogger
}

// NewPodGroupManager creates a new operation object.
func NewPodGroupManager(
	client client.Client,
	snapshotSharedLister framework.SharedLister,
	scheduleTimeout *time.Duration,
	podInformer informerv1.PodInformer,
	log *logger.DebugLogger,
) *PodGroupManager {
	podGroupManager := &PodGroupManager{
		client:               client,
		snapshotSharedLister: snapshotSharedLister,
		scheduleTimeout:      scheduleTimeout,
		podLister:            podInformer.Lister(),
		permittedpodGroup:    gochache.New(3*time.Second, 3*time.Second),
		backedOffpodGroup:    gochache.New(10*time.Second, 10*time.Second),
		groupToJobId:         map[string]uint64{},
		podToNode:            map[string]string{},
		log:                  log,
	}
	return podGroupManager
}

func (podGroupManager *PodGroupManager) BackoffPodGroup(groupName string, backoff time.Duration) {
	if backoff == time.Duration(0) {
		return
	}
	podGroupManager.backedOffpodGroup.Add(groupName, nil, backoff)
}

// ActivateSiblings stashes the pods belonging to the same PodGroup of the given pod
// in the given state, with a reserved key "kubernetes.io/pods-to-activate".
func (podGroupManager *PodGroupManager) ActivateSiblings(pod *corev1.Pod, state *framework.CycleState) {
	groupName := util.GetPodGroupLabel(pod)
	if groupName == "" {
		return
	}

	// Only proceed if it's explicitly requested to activate sibling pods.
	if c, err := state.Read(permitStateKey); err != nil {
		return
	} else if s, ok := c.(*PermitState); !ok || !s.Activate {
		return
	}

	pods, err := podGroupManager.podLister.Pods(pod.Namespace).List(
		labels.SelectorFromSet(labels.Set{v1alpha1.PodGroupLabel: groupName}),
	)
	if err != nil {
		klog.ErrorS(err, "Failed to obtain pods belong to a PodGroup", "podGroup", groupName)
		return
	}

	for i := range pods {
		if pods[i].UID == pod.UID {
			pods = append(pods[:i], pods[i+1:]...)
			break
		}
	}

	if len(pods) != 0 {
		if c, err := state.Read(framework.PodsToActivateKey); err == nil {
			if s, ok := c.(*framework.PodsToActivate); ok {
				s.Lock()
				for _, pod := range pods {
					namespacedName := GetNamespacedName(pod)
					s.Map[namespacedName] = pod
				}
				s.Unlock()
			}
		}
	}
}

// GetStatuses string (of all pods) to show for debugging purposes
func (podGroupManager *PodGroupManager) GetStatuses(
	pods []*corev1.Pod,
) string {
	statuses := ""

	// We need to distinguish 0 from the default and not finding anything
	for _, pod := range pods {
		statuses += " " + fmt.Sprintf("%s", pod.Status.Phase)
	}
	return statuses
}

// GetPodNode is a quick lookup to see if we have a node
func (podGroupManager *PodGroupManager) GetPodNode(pod *corev1.Pod) string {
	node, _ := podGroupManager.podToNode[pod.Name]
	return node
}

// Permit permits a pod to run, if the minMember match, it would send a signal to chan.
func (podGroupManager *PodGroupManager) Permit(ctx context.Context, state *framework.CycleState, pod *corev1.Pod) Status {
	groupName, podGroup := podGroupManager.GetPodGroup(ctx, pod)
	if groupName == "" {
		return PodGroupNotSpecified
	}
	if podGroup == nil {
		// A Pod with a podGroup name but without a PodGroup found is denied.
		return PodGroupNotFound
	}

	assigned := podGroupManager.CalculateAssignedPods(podGroup.Name, podGroup.Namespace)
	// The number of pods that have been assigned nodes is calculated from the snapshot.
	// The current pod in not included in the snapshot during the current scheduling cycle.
	if int32(assigned)+1 >= podGroup.Spec.MinMember {
		return Success
	}

	if assigned == 0 {
		// Given we've reached Permit(), it's mean all PreFilter checks (minMember & minResource)
		// already pass through, so if assigned == 0, it could be due to:
		// - minResource get satisfied
		// - new pods added
		// In either case, we should and only should use this 0-th pod to trigger activating
		// its siblings.
		// It'd be in-efficient if we trigger activating siblings unconditionally.
		// See https://github.com/kubernetes-sigs/scheduler-plugins/issues/682
		state.Write(permitStateKey, &PermitState{Activate: true})
	}

	return Wait
}

// PreFilter filters out a pod if
// 1. it belongs to a podgroup that was recently denied or
// 2. the total number of pods in the podgroup is less than the minimum number of pods
// that is required to be scheduled.
func (podGroupManager *PodGroupManager) PreFilter(
	ctx context.Context,
	pod *corev1.Pod,
	state *framework.CycleState,
) error {

	podGroupManager.log.Info("[PodGroup PreFilter] pod %s", klog.KObj(pod))
	groupName, podGroup := podGroupManager.GetPodGroup(ctx, pod)
	if podGroup == nil {
		return nil
	}

	_, exist := podGroupManager.backedOffpodGroup.Get(groupName)
	if exist {
		return fmt.Errorf("podGroup %v failed recently", groupName)
	}

	pods, err := podGroupManager.podLister.Pods(pod.Namespace).List(
		labels.SelectorFromSet(labels.Set{v1alpha1.PodGroupLabel: util.GetPodGroupLabel(pod)}),
	)
	if err != nil {
		return fmt.Errorf("podLister list pods failed: %w", err)
	}

	// Only allow scheduling the first in the group so the others come after

	// Get statuses to show for debugging
	statuses := podGroupManager.GetStatuses(pods)

	// This shows us the number of pods we have in the set and their states
	podGroupManager.log.Info("[PodGroup PreFilter] group: %s pods: %s MinMember: %d Size: %d", groupName, statuses, podGroup.Spec.MinMember, len(pods))
	if len(pods) < int(podGroup.Spec.MinMember) {
		return fmt.Errorf("pre-filter pod %v cannot find enough sibling pods, "+
			"current pods number: %v, minMember of group: %v", pod.Name, len(pods), podGroup.Spec.MinMember)
	}

	// TODO we likely can take advantage of these resources or other custom
	// attributes we add. For now ignore and calculate based on pod needs (above)
	// if podGroup.Spec.MinResources == nil {
	//	fmt.Printf("Fluence Min resources are null, skipping PreFilter")
	//	return nil
	// }

	// This is from coscheduling.
	// TODO(cwdsuzhou): This resource check may not always pre-catch unschedulable pod group.
	// It only tries to PreFilter resource constraints so even if a PodGroup passed here,
	// it may not necessarily pass Filter due to other constraints such as affinity/taints.
	_, ok := podGroupManager.permittedpodGroup.Get(groupName)
	if ok {
		podGroupManager.log.Info("[PodGroup PreFilter] Pod Group %s is already admitted", groupName)
		return nil
	}

	// TODO: right now we ask Fluxion for a podspec based on ONE representative pod, but
	// we have the whole group! We can handle different pod needs now :)
	repPod := pods[0]
	nodes, err := podGroupManager.AskFlux(ctx, *repPod, podGroup, groupName)
	if err != nil {
		podGroupManager.log.Info("[PodGroup PreFilter] Fluxion returned an error %s, not schedulable", err.Error())
		return err
	}
	podGroupManager.log.Info("Node Selected %s (pod group %s)", nodes, groupName)

	// Some reason fluxion gave us the wrong size?
	if len(nodes) != len(pods) {
		podGroupManager.log.Warning("[PodGroup PreFilter] group %s needs %d nodes but Fluxion returned the wrong number nodes %d.", groupName, len(pods), len(nodes))
		podGroupManager.mutex.Lock()
		podGroupManager.cancelFluxJob(groupName, repPod)
		podGroupManager.mutex.Unlock()
	}

	// Create a fluxState (CycleState) with all nodes - this is used to retrieve
	// the specific node assigned to the pod in Filter, which returns a node
	// Note that this probably is not useful beyond the pod we are in the context
	// of, but why not do it.
	for i, node := range nodes {
		pod := pods[i]
		stateData := FluxStateData{NodeName: node}
		state.Write(framework.StateKey(pod.Name), &stateData)
		// Also save to the podToNode lookup
		podGroupManager.mutex.Lock()
		podGroupManager.podToNode[pod.Name] = node
		podGroupManager.mutex.Unlock()
	}
	podGroupManager.permittedpodGroup.Add(groupName, groupName, *podGroupManager.scheduleTimeout)
	return nil
}

// GetCreationTimestamp returns the creation time of a podGroup or a pod.
func (podGroupManager *PodGroupManager) GetCreationTimestamp(pod *corev1.Pod, ts time.Time) time.Time {
	groupName := util.GetPodGroupLabel(pod)
	if len(groupName) == 0 {
		return ts
	}
	var podGroup v1alpha1.PodGroup
	if err := podGroupManager.client.Get(context.TODO(), types.NamespacedName{Namespace: pod.Namespace, Name: groupName}, &podGroup); err != nil {
		return ts
	}
	return podGroup.CreationTimestamp.Time
}

// CalculateAssignedPods returns the number of pods that has been assigned nodes: assumed or bound.
func (podGroupManager *PodGroupManager) CalculateAssignedPods(podGroupName, namespace string) int {
	nodeInfos, err := podGroupManager.snapshotSharedLister.NodeInfos().List()
	if err != nil {
		podGroupManager.log.Error("Cannot get nodeInfos from frameworkHandle: %s", err)
		return 0
	}
	var count int
	for _, nodeInfo := range nodeInfos {
		for _, podInfo := range nodeInfo.Pods {
			pod := podInfo.Pod
			if util.GetPodGroupLabel(pod) == podGroupName && pod.Namespace == namespace && pod.Spec.NodeName != "" {
				count++
			}
		}
	}
	return count
}

// DeletePermittedPodGroup deletes a podGroup that passes Pre-Filter but reaches PostFilter.
func (podGroupManager *PodGroupManager) DeletePermittedPodGroup(groupName string) {
	podGroupManager.permittedpodGroup.Delete(groupName)
}

// GetPodGroup returns the PodGroup that a Pod belongs to in cache.
func (podGroupManager *PodGroupManager) GetPodGroup(ctx context.Context, pod *corev1.Pod) (string, *v1alpha1.PodGroup) {
	groupName := util.GetPodGroupLabel(pod)
	if len(groupName) == 0 {
		return "", nil
	}
	var podGroup v1alpha1.PodGroup
	if err := podGroupManager.client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: groupName}, &podGroup); err != nil {
		return fmt.Sprintf("%v/%v", pod.Namespace, groupName), nil
	}
	return fmt.Sprintf("%v/%v", pod.Namespace, groupName), &podGroup
}

// GetNamespacedName returns the namespaced name.
func GetNamespacedName(obj metav1.Object) string {
	return fmt.Sprintf("%v/%v", obj.GetNamespace(), obj.GetName())
}
