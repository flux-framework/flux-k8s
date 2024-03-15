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

// TODO should eventually store group name here to reassociate on reload
type FluxStateData struct {
	NodeName string
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
	// permittedPG stores the podgroup name which has passed the pre resource check.
	permittedPG *gochache.Cache
	// backedOffPG stores the podgorup name which failed scheudling recently.
	backedOffPG *gochache.Cache
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
	pgMgr := &PodGroupManager{
		client:               client,
		snapshotSharedLister: snapshotSharedLister,
		scheduleTimeout:      scheduleTimeout,
		podLister:            podInformer.Lister(),
		permittedPG:          gochache.New(3*time.Second, 3*time.Second),
		backedOffPG:          gochache.New(10*time.Second, 10*time.Second),
		groupToJobId:         map[string]uint64{},
		podToNode:            map[string]string{},
		log:                  log,
	}
	return pgMgr
}

// GetStatuses string (of all pods) to show for debugging purposes
func (pgMgr *PodGroupManager) GetStatuses(pods []*corev1.Pod) string {
	statuses := ""
	for _, pod := range pods {
		statuses += " " + fmt.Sprintf("%s", pod.Status.Phase)
	}
	return statuses
}

// GetPodNode is a quick lookup to see if we have a node
func (pgMgr *PodGroupManager) GetPodNode(pod *corev1.Pod) string {
	node, _ := pgMgr.podToNode[pod.Name]
	return node
}

// PreFilter filters out a pod if
// 1. it belongs to a podgroup that was recently denied or
// 2. the total number of pods in the podgroup is less than the minimum number of pods
// that is required to be scheduled.
func (pgMgr *PodGroupManager) PreFilter(
	ctx context.Context,
	pod *corev1.Pod,
	state *framework.CycleState,
) error {

	pgMgr.log.Info("[PodGroup PreFilter] pod %s", klog.KObj(pod))
	pgFullName, pg := pgMgr.GetPodGroup(ctx, pod)
	if pg == nil {
		return nil
	}

	_, exist := pgMgr.backedOffPG.Get(pgFullName)
	if exist {
		return fmt.Errorf("podGroup %v failed recently", pgFullName)
	}

	pods, err := pgMgr.podLister.Pods(pod.Namespace).List(
		labels.SelectorFromSet(labels.Set{v1alpha1.PodGroupLabel: util.GetPodGroupLabel(pod)}),
	)
	if err != nil {
		return fmt.Errorf("podLister list pods failed: %w", err)
	}

	// Get statuses to show for debugging
	statuses := pgMgr.GetStatuses(pods)

	// This shows us the number of pods we have in the set and their states
	pgMgr.log.Info("[PodGroup PreFilter] group: %s pods: %s MinMember: %d Size: %d", pgFullName, statuses, pg.Spec.MinMember, len(pods))
	if len(pods) < int(pg.Spec.MinMember) {
		return fmt.Errorf("pre-filter pod %v cannot find enough sibling pods, "+
			"current pods number: %v, minMember of group: %v", pod.Name, len(pods), pg.Spec.MinMember)
	}

	// TODO we likely can take advantage of these resources or other custom
	// attributes we add. For now ignore and calculate based on pod needs (above)
	// if pg.Spec.MinResources == nil {
	//	fmt.Printf("Fluence Min resources are null, skipping PreFilter")
	//	return nil
	// }

	// This is from coscheduling.
	// TODO(cwdsuzhou): This resource check may not always pre-catch unschedulable pod group.
	// It only tries to PreFilter resource constraints so even if a PodGroup passed here,
	// it may not necessarily pass Filter due to other constraints such as affinity/taints.
	_, ok := pgMgr.permittedPG.Get(pgFullName)
	if ok {
		return nil
	}

	// TODO: right now we ask Fluxion for a podspec based on ONE pod, but
	// we have the whole group! We can handle different pod needs now :)
	repPod := pods[0]
	nodes, err := pgMgr.AskFlux(ctx, *repPod, pg, pgFullName)
	if err != nil {
		pgMgr.log.Info("[PodGroup PreFilter] Fluxion returned an error %s, not schedulable", err.Error())
		return err
	}
	pgMgr.log.Info("Node Selected %s (pod group %s)", nodes, pgFullName)

	// Some reason fluxion gave us the wrong size?
	if len(nodes) != len(pods) {
		pgMgr.log.Warning("[PodGroup PreFilter] group %s needs %d nodes but Fluxion returned the wrong number nodes %d.", pgFullName, len(pods), len(nodes))
		pgMgr.mutex.Lock()
		pgMgr.cancelFluxJob(pgFullName, repPod)
		pgMgr.mutex.Unlock()
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
		pgMgr.podToNode[pod.Name] = node
	}
	pgMgr.permittedPG.Add(pgFullName, pgFullName, *pgMgr.scheduleTimeout)
	return nil
}

// GetCreationTimestamp returns the creation time of a podGroup or a pod.
func (pgMgr *PodGroupManager) GetCreationTimestamp(pod *corev1.Pod, ts time.Time) time.Time {
	pgName := util.GetPodGroupLabel(pod)
	if len(pgName) == 0 {
		return ts
	}
	var pg v1alpha1.PodGroup
	if err := pgMgr.client.Get(context.TODO(), types.NamespacedName{Namespace: pod.Namespace, Name: pgName}, &pg); err != nil {
		return ts
	}
	return pg.CreationTimestamp.Time
}

// DeletePermittedPodGroup deletes a podGroup that passes Pre-Filter but reaches PostFilter.
func (pgMgr *PodGroupManager) DeletePermittedPodGroup(pgFullName string) {
	pgMgr.permittedPG.Delete(pgFullName)
}

// GetPodGroup returns the PodGroup that a Pod belongs to in cache.
func (pgMgr *PodGroupManager) GetPodGroup(ctx context.Context, pod *corev1.Pod) (string, *v1alpha1.PodGroup) {
	pgName := util.GetPodGroupLabel(pod)
	if len(pgName) == 0 {
		return "", nil
	}
	var pg v1alpha1.PodGroup
	if err := pgMgr.client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: pgName}, &pg); err != nil {
		return fmt.Sprintf("%v/%v", pod.Namespace, pgName), nil
	}
	return fmt.Sprintf("%v/%v", pod.Namespace, pgName), &pg
}

// GetNamespacedName returns the namespaced name.
func GetNamespacedName(obj metav1.Object) string {
	return fmt.Sprintf("%v/%v", obj.GetNamespace(), obj.GetName())
}
