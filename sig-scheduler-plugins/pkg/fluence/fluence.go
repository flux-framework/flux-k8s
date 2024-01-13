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
	"strconv"
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
	Name              = "Fluence"
	PodGroupNameLabel = "fluence.pod-group"
	PodGroupSizeLabel = "fluence.group-size"
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

	// Save the podLister to fluence to easily query for the group
	f.podLister = podInformer.Lister()

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

// getDefaultGroupName returns a group name based on the pod namespace and name
// We do this for pods that are not labeled, and treat them as a size 1 group
func (f *Fluence) getDefaultGroupName(pod *v1.Pod) string {
	return fmt.Sprintf("%s-%s", pod.Namespace, pod.Name)
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

	// If there isn't a group, we make one based on the pod name and namespace, of size 1
	// We do this so that we can handle them equivalently later
	// We assume a pod with no group has a size of 1.
	if groupName == "" {
		groupName = f.getDefaultGroupName(pod)
		groupSize = 1
	}
	klog.Infof("group name for %s is %s", pod.Name, groupName)
	klog.Infof("group size for %s is %d", pod.Name, groupSize)

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
		klog.Error("Parsing integer size for pod group")
	}
	return int32(intSize)
}

// getCreationTimestamp first tries the fluence group, then falls back to the initial attempt timestamp
func (f *Fluence) getCreationTimestamp(groupName string, podInfo *framework.QueuedPodInfo) metav1.Time {
	pg := fcore.GetPodGroup(groupName)

	// IsZero is an indicator if this was actually set
	if !pg.TimeCreated.IsZero() {
		klog.Infof("pod group %s was created at %s", groupName, pg.TimeCreated)
		return pg.TimeCreated
	}
	return metav1.NewTime(*podInfo.InitialAttemptTimestamp)
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

	var (
		err      error
		nodename string
	)
	klog.Infof("Examining the pod")

	// groupName will be named according to the single pod namespace / pod if there wasn't
	// a user defined group. This is a size 1 group we handle equivalently.
	pg := f.getPodsGroup(pod)

	klog.Infof("The group size %d", pg.Size)
	klog.Infof("group name is %s", pg.Name)

	// If we don't have nodes yet assigned to the group - get them from flux!
	// We don't use the pod list here because it could be empty
	if !pg.IsAllocated {
		klog.Infof("Getting listing of pods for pod group %s", pg.Name)
		err = f.AskFlux(ctx, pod, pg.Size)
		if err != nil {
			return nil, framework.NewStatus(framework.Unschedulable, err.Error())
		}
	}

	// Select the next available node for the allocation (one pod or more)
	// This will be empty (no nodes left) and IsAllocated true if already done
	// I don't know how we would get to this case, should watch for it
	nodename, err = fcore.GetNextNode(pg.Name)
	klog.Infof("Node Selected %s (%s:%s)", nodename, pod.Name, pg.Name)
	if err != nil {
		return nil, framework.NewStatus(framework.Unschedulable, err.Error())
	}

	// Create a fluxState (CycleState) with things that might be useful/
	// Do we want to add the timestamp created here?
	fluxState := fcore.NewFluxState(nodename, pg.Name, pg.Size)

	// TODO should we save more than the FluxStateData? Why not assignment, etc.?
	klog.Info("Node Selected: ", nodename)
	state.Write(framework.StateKey(pod.Name), fluxState)
	return nil, framework.NewStatus(framework.Success, "")

}

// getPodsGroup gets the pods group, if it exists.
func (f *Fluence) getPodsGroup(pod *v1.Pod) *fcore.PodGroupCache {
	groupName := f.ensureFluenceGroup(pod)
	return fcore.GetPodGroup(groupName)
}

// Filter finds the node(s) where it's feasible to schedule a pod
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
			klog.Infof("Filter: node selected by Flux %s for pod %s/%s\n", value.NodeCache.NodeName, pod.Namespace, pod.Name)
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
func (f *Fluence) AskFlux(ctx context.Context, pod *v1.Pod, groupSize int32) error {

	// clean up previous match if a pod has already allocated previously
	f.mutex.Lock()
	_, isPodAllocated := f.podNameToJobId[pod.Name]
	f.mutex.Unlock()

	if isPodAllocated {
		klog.Info("Cleaning up previous allocation for pod %s", pod.Name)
		f.mutex.Lock()
		f.cancelFluxJobForPod(pod)
		f.mutex.Unlock()
	}

	jobspec := utils.InspectPodInfo(pod)

	klog.Infof("[JOBSPEC]: %s", jobspec)
	conn, err := grpc.Dial("127.0.0.1:4242", grpc.WithInsecure())

	if err != nil {
		klog.Errorf("[FluxClient] Error connecting to server: %v", err)
		return err
	}
	defer conn.Close()

	// TODO do we want to expose this timeout as a variable somewhere?
	grpcclient := pb.NewFluxcliServiceClient(conn)
	_, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	request := &pb.MatchRequest{
		Ps:      jobspec,
		Request: "allocate",
		Count:   groupSize,
	}

	// Note this used to be err2 and then return err, want to double check why
	r, err := grpcclient.Match(context.Background(), request)
	klog.Infof("[FluxClient] error %s", err)
	if err != nil {
		klog.Errorf("[FluxClient] did not receive any match response: %v", err)
		return err
	}
	klog.Infof("[FluxClient] response podID %s", r.GetPodID())

	// Presence of a podGroup is indicated by a groupName
	// Flag that the group is allocated (yes we also have the job id, testing for now)
	pg := f.getPodsGroup(pod)
	pg.IsAllocated = true

	// Pod groups defined by user and standalone pods are treated the same
	// as we create a group of size one for the latter.
	nodeList := r.GetNodelist()
	nodePods := fcore.CreateNodePodsList(nodeList, pg.Name)

	klog.Infof("[FluxClient] response nodeID %s", nodeList)
	klog.Info("[FluxClient] Parsed Nodelist ", nodePods)
	jobid := uint64(r.GetJobID())

	f.mutex.Lock()
	f.podNameToJobId[pod.Name] = jobid
	klog.Info("Check job set: ", f.podNameToJobId)
	f.mutex.Unlock()
	return nil
}

// cancelFluxJobForPod cancels the flux job for a pod.
func (f *Fluence) cancelFluxJobForPod(pod *v1.Pod) error {

	// TODO should this also be indexing by namespace?
	// I assume fluence can schedule across them?
	jobid := f.podNameToJobId[pod.Name]
	pg := f.getPodsGroup(pod)

	klog.Infof("Cancel flux job: %d for pod %s", jobid, pod.Name)

	start := time.Now()

	// connect to Flux via GRPC
	conn, err := grpc.Dial("127.0.0.1:4242", grpc.WithInsecure())
	if err != nil {
		klog.Errorf("[FluxClient] Error connecting to server: %v", err)
		return err
	}
	defer conn.Close()

	grpcclient := pb.NewFluxcliServiceClient(conn)
	_, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	// Prepare and make the request to the GRPC client
	request := &pb.CancelRequest{JobID: int64(jobid)}
	res, err := grpcclient.Cancel(context.Background(), request)
	if err != nil {
		klog.Errorf("[FluxClient] did not receive any cancel response: %v", err)
		return err
	}

	// Return code 0 is a successful cancel, and we can delete the record of the job
	// running on the pod, and the nodes.
	if res.Error == 0 {
		delete(f.podNameToJobId, pod.Name)
		pg.CancelAllocation()
	} else {
		klog.Warningf("Failed to delete pod %s from the podname-jobid map.", pod.Name)
	}

	elapsed := metrics.SinceInSeconds(start)
	klog.Info("Time elapsed (Cancel Job) :", elapsed)

	// We only make it down here if the cancel is successful
	klog.Infof("Job cancellation for pod %s was successful", pod.Name)
	if klog.V(2).Enabled() {
		klog.Info("Check job set: after delete")
		klog.Info(f.podNameToJobId)
	}
	return nil
}

// EventHandlers updatePod handles cleaning up resources
func (f *Fluence) updatePod(oldObj, newObj interface{}) {

	newPod := newObj.(*v1.Pod)
	klog.Infof("Processing event for pod %s", newPod)

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

// deletePod handles the case that the pod is deleted
// TODO where do we put to delete the pod group? Normally it would be done way after
func (f *Fluence) deletePod(podObj interface{}) {
	klog.Info("Delete Pod event handler")

	pod := podObj.(*v1.Pod)
	klog.Infof("Pod status: %s", pod.Status.Phase)
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
