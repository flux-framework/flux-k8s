/*
Copyright 2022 The Kubernetes Authors.

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
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	corev1helpers "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/metrics"

	corelisters "k8s.io/client-go/listers/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	sched "sigs.k8s.io/scheduler-plugins/apis/scheduling/v1alpha1"
	coschedulingcore "sigs.k8s.io/scheduler-plugins/pkg/coscheduling/core"
	fcore "sigs.k8s.io/scheduler-plugins/pkg/fluence/core"
	pb "sigs.k8s.io/scheduler-plugins/pkg/fluence/fluxcli-grpc"
	"sigs.k8s.io/scheduler-plugins/pkg/fluence/utils"
)

type Fluence struct {
	mutex          sync.Mutex
	handle         framework.Handle
	client         client.Client
	podNameToJobId map[string]uint64
	pgMgr          coschedulingcore.Manager

	// The pod group manager has a lister, but it's private
	podLister corelisters.PodLister
}

// Name is the name of the plugin used in the Registry and configurations.
// Note that this would do better as an annotation (fluence.flux-framework.org/pod-group)
// But we cannot use them as selectors then!
const (
	Name = "Fluence"
)

var (
	_ framework.QueueSortPlugin = &Fluence{}
	_ framework.PreFilterPlugin = &Fluence{}
	_ framework.FilterPlugin    = &Fluence{}
)

func (f *Fluence) Name() string {
	return Name
}

// Initialize and return a new Fluence Custom Scheduler Plugin
// This class and functions are analogous to:
// https://github.com/kubernetes-sigs/scheduler-plugins/blob/master/pkg/coscheduling/coscheduling.go#L63
func New(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {

	f := &Fluence{handle: handle, podNameToJobId: make(map[string]uint64)}

	klog.Info("Create plugin")
	ctx := context.TODO()
	fcore.Init()

	fluxPodsInformer := handle.SharedInformerFactory().Core().V1().Pods().Informer()
	fluxPodsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: f.updatePod,
		DeleteFunc: f.deletePod,
	})

	go fluxPodsInformer.Run(ctx.Done())
	klog.Info("Create generic pod informer")

	scheme := runtime.NewScheme()
	clientscheme.AddToScheme(scheme)
	v1.AddToScheme(scheme)
	sched.AddToScheme(scheme)
	k8scli, err := client.New(handle.KubeConfig(), client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	// Save the kubernetes client for fluence to interact with cluster objects
	f.client = k8scli

	fieldSelector, err := fields.ParseSelector(",status.phase!=" + string(v1.PodSucceeded) + ",status.phase!=" + string(v1.PodFailed))
	if err != nil {
		klog.ErrorS(err, "ParseSelector failed")
		os.Exit(1)
	}

	informerFactory := informers.NewSharedInformerFactoryWithOptions(handle.ClientSet(), 0, informers.WithTweakListOptions(func(opt *metav1.ListOptions) {
		opt.FieldSelector = fieldSelector.String()
	}))
	podInformer := informerFactory.Core().V1().Pods()
	scheduleTimeDuration := time.Duration(500) * time.Second

	pgMgr := coschedulingcore.NewPodGroupManager(
		k8scli,
		handle.SnapshotSharedLister(),
		&scheduleTimeDuration,
		podInformer,
	)
	f.pgMgr = pgMgr

	// stopCh := make(chan struct{})
	// defer close(stopCh)
	// informerFactory.Start(stopCh)
	informerFactory.Start(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), podInformer.Informer().HasSynced) {
		err := fmt.Errorf("WaitForCacheSync failed")
		klog.ErrorS(err, "Cannot sync caches")
		return nil, err
	}

	klog.Info("Fluence start")
	return f, nil
}

// Less is used to sort pods in the scheduling queue in the following order.
// 1. Compare the priorities of Pods.
// 2. Compare the initialization timestamps of fluence pod groups
// 3. Fall back, sort by namespace/name
// See 	https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/
// Less is part of Sort, which is the earliest we can see a pod unless we use gate
// IMPORTANT: Less sometimes is not called for smaller sizes, not sure why.
// To get around this we call it during PreFilter too.
func (f *Fluence) Less(podInfo1, podInfo2 *framework.QueuedPodInfo) bool {
	klog.Infof("ordering pods from Coscheduling")

	// ensure we have a PodGroup no matter what
	klog.Infof("ensuring fluence groups")
	podGroup1 := f.ensureFluenceGroup(podInfo1.Pod)
	podGroup2 := f.ensureFluenceGroup(podInfo2.Pod)

	// First preference to priority, but only if they are different
	prio1 := corev1helpers.PodPriority(podInfo1.Pod)
	prio2 := corev1helpers.PodPriority(podInfo2.Pod)

	// ...and only allow this to sort if they aren't the same
	// The assumption here is that pods with priority are ignored by fluence
	if prio1 != prio2 {
		return prio1 > prio2
	}

	// Fluence can only compare if we have two known groups.
	// This tries for that first, and falls back to the initial attempt timestamp
	creationTime1 := f.getCreationTimestamp(podGroup1, podInfo1)
	creationTime2 := f.getCreationTimestamp(podGroup2, podInfo2)

	// If they are the same, fall back to sorting by name.
	if creationTime1.Equal(&creationTime2) {
		return coschedulingcore.GetNamespacedName(podInfo1.Pod) < coschedulingcore.GetNamespacedName(podInfo2.Pod)
	}
	return creationTime1.Before(&creationTime2)
}

// PreFilter checks info about the Pod / checks conditions that the cluster or the Pod must meet.
// This still comes after sort
func (f *Fluence) PreFilter(
	ctx context.Context,
	state *framework.CycleState,
	pod *v1.Pod,
) (*framework.PreFilterResult, *framework.Status) {

	klog.Infof("Examining the pod")

	// groupName will be named according to the single pod namespace / pod if there wasn't
	// a user defined group. This is a size 1 group we handle equivalently.
	pg := f.getPodsGroup(pod)

	klog.Infof("The group size %d", pg.Size)
	klog.Infof("group name is %s", pg.Name)

	// Note that it is always the case we have a group
	// We have not yet derived a node list
	if !pg.HavePodNodes() {
		klog.Infof("Getting a pod group")
		err := f.AskFlux(ctx, pod, int(pg.Size))
		if err != nil {
			return nil, framework.NewStatus(framework.Unschedulable, err.Error())
		}
	}
	nodename, err := fcore.GetNextNode(pg.Name)
	klog.Infof("Node Selected %s (%s:%s)", nodename, pod.Name, pg.Name)
	if err != nil {
		return nil, framework.NewStatus(framework.Unschedulable, err.Error())
	}

	// Create a fluxState (CycleState) with things that might be useful/
	klog.Info("Node Selected: ", nodename)
	cache := fcore.NodeCache{NodeName: nodename}
	state.Write(framework.StateKey(pod.Name), &fcore.FluxStateData{NodeCache: cache})
	return nil, framework.NewStatus(framework.Success, "")
}

// TODO we need to account for affinity here
func (f *Fluence) Filter(
	ctx context.Context,
	cycleState *framework.CycleState,
	pod *v1.Pod,
	nodeInfo *framework.NodeInfo,
) *framework.Status {

	klog.Info("Filtering input node ", nodeInfo.Node().Name)
	if v, e := cycleState.Read(framework.StateKey(pod.Name)); e == nil {
		if value, ok := v.(*fcore.FluxStateData); ok && value.NodeCache.NodeName != nodeInfo.Node().Name {
			return framework.NewStatus(framework.Unschedulable, "pod is not permitted")
		} else {
			klog.Infof("Filter: node %s selected for %s\n", value.NodeCache.NodeName, pod.Name)
		}
	}
	return framework.NewStatus(framework.Success)
}

// PreFilterExtensions allow for callbacks on filtered states
// https://github.com/kubernetes/kubernetes/blob/master/pkg/scheduler/framework/interface.go#L383
func (f *Fluence) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

// AskFlux will ask flux for an allocation for nodes for the pod group.
func (f *Fluence) AskFlux(ctx context.Context, pod *v1.Pod, count int) error {
	// clean up previous match if a pod has already allocated previously
	f.mutex.Lock()
	_, isPodAllocated := f.podNameToJobId[pod.Name]
	f.mutex.Unlock()

	if isPodAllocated {
		klog.Info("Clean up previous allocation")
		f.mutex.Lock()
		f.cancelFluxJobForPod(pod)
		f.mutex.Unlock()
	}

	jobspec := utils.InspectPodInfo(pod)
	conn, err := grpc.Dial("127.0.0.1:4242", grpc.WithInsecure())

	if err != nil {
		klog.Errorf("[FluxClient] Error connecting to server: %v", err)
		return err
	}
	defer conn.Close()

	grpcclient := pb.NewFluxcliServiceClient(conn)
	_, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	request := &pb.MatchRequest{
		Ps:      jobspec,
		Request: "allocate",
		Count:   int32(count)}

	// Question from vsoch; Why return err instead of err2 here?
	// err would return a nil value, but we need to return non nil,
	// otherwise it's going to try to use the allocation (but there is none)
	r, err := grpcclient.Match(context.Background(), request)
	if err != nil {
		klog.Errorf("[FluxClient] did not receive any match response: %v", err)
		return err
	}

	klog.Infof("[FluxClient] response podID %s", r.GetPodID())

	// Presence of a podGroup is indicated by a groupName
	// Flag that the group is allocated (yes we also have the job id, testing for now)
	pg := f.getPodsGroup(pod)

	nodelist := fcore.CreateNodePodsList(r.GetNodelist(), pg.Name)
	klog.Infof("[FluxClient] response nodeID %s", r.GetNodelist())
	klog.Info("[FluxClient] Parsed Nodelist ", nodelist)
	jobid := uint64(r.GetJobID())

	f.mutex.Lock()
	f.podNameToJobId[pod.Name] = jobid
	klog.Info("Check job set: ", f.podNameToJobId)
	f.mutex.Unlock()
	return nil
}

// cancelFluxJobForPod cancels the flux job for a pod.
func (f *Fluence) cancelFluxJobForPod(pod *v1.Pod) error {
	jobid := f.podNameToJobId[pod.Name]

	klog.Infof("Cancel flux job: %v for pod %s", jobid, pod.Name)

	start := time.Now()

	conn, err := grpc.Dial("127.0.0.1:4242", grpc.WithInsecure())

	if err != nil {
		klog.Errorf("[FluxClient] Error connecting to server: %v", err)
		return err
	}
	defer conn.Close()

	grpcclient := pb.NewFluxcliServiceClient(conn)
	_, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	request := &pb.CancelRequest{
		JobID: int64(jobid),
	}

	res, err := grpcclient.Cancel(context.Background(), request)
	if err != nil {
		klog.Errorf("[FluxClient] did not receive any cancel response: %v", err)
		return err
	}

	if res.Error == 0 {
		delete(f.podNameToJobId, pod.Name)
	} else {
		klog.Warningf("Failed to delete pod %s from the podname-jobid map.", pod.Name)
	}

	// If we are successful, clear the group allocated nodes
	pg := f.getPodsGroup(pod)
	pg.CancelAllocation()

	elapsed := metrics.SinceInSeconds(start)
	klog.Info("Time elapsed (Cancel Job) :", elapsed)

	klog.Infof("Job cancellation for pod %s result: %d", pod.Name, err)
	if klog.V(2).Enabled() {
		klog.Info("Check job set: after delete")
		klog.Info(f.podNameToJobId)
	}
	return nil
}

// EventHandlers updatePod handles cleaning up resources
func (f *Fluence) updatePod(oldObj, newObj interface{}) {
	// klog.Info("Update Pod event handler")
	newPod := newObj.(*v1.Pod)

	klog.Infof("Processing event for pod %s", newPod.Name)

	switch newPod.Status.Phase {
	case v1.PodPending:
		// in this state we don't know if a pod is going to be running, thus we don't need to update job map
	case v1.PodRunning:
		// if a pod is start running, we can add it state to the delta graph if it is scheduled by other scheduler
	case v1.PodSucceeded:
		klog.Infof("Pod %s succeeded, Fluence needs to free the resources", newPod.Name)

		f.mutex.Lock()
		defer f.mutex.Unlock()

		if _, ok := f.podNameToJobId[newPod.Name]; ok {
			f.cancelFluxJobForPod(newPod)
		} else {
			klog.Infof("Succeeded pod %s/%s doesn't have flux jobid", newPod.Namespace, newPod.Name)
		}
	case v1.PodFailed:
		// a corner case need to be tested, the pod exit code is not 0, can be created with segmentation fault pi test
		klog.Warningf("Pod %s failed, Fluence needs to free the resources", newPod.Name)

		f.mutex.Lock()
		defer f.mutex.Unlock()

		if _, ok := f.podNameToJobId[newPod.Name]; ok {
			f.cancelFluxJobForPod(newPod)
		} else {
			klog.Errorf("Failed pod %s/%s doesn't have flux jobid", newPod.Namespace, newPod.Name)
		}
	case v1.PodUnknown:
		// don't know how to deal with it as it's unknown phase
	default:
		// shouldn't enter this branch
	}
}

// deletePod handles the delete event handler
// TODO when should we clear group from the cache?
func (f *Fluence) deletePod(podObj interface{}) {
	klog.Info("Delete Pod event handler")

	pod := podObj.(*v1.Pod)
	klog.Info("Pod status: ", pod.Status.Phase)
	switch pod.Status.Phase {
	case v1.PodSucceeded:
	case v1.PodPending:
		klog.Infof("Pod %s completed and is Pending termination, Fluence needs to free the resources", pod.Name)

		f.mutex.Lock()
		defer f.mutex.Unlock()

		if _, ok := f.podNameToJobId[pod.Name]; ok {
			f.cancelFluxJobForPod(pod)
		} else {
			klog.Infof("Terminating pod %s/%s doesn't have flux jobid", pod.Namespace, pod.Name)
		}
	case v1.PodRunning:
		f.mutex.Lock()
		defer f.mutex.Unlock()

		if _, ok := f.podNameToJobId[pod.Name]; ok {
			f.cancelFluxJobForPod(pod)
		} else {
			klog.Infof("Deleted pod %s/%s doesn't have flux jobid", pod.Namespace, pod.Name)
		}
	}
}
