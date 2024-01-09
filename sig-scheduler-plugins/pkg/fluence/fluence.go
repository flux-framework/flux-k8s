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
	"sort"
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
	"sigs.k8s.io/scheduler-plugins/apis/scheduling/v1alpha1"
	coschedulingcore "sigs.k8s.io/scheduler-plugins/pkg/coscheduling/core"
	fcore "sigs.k8s.io/scheduler-plugins/pkg/fluence/core"
	pb "sigs.k8s.io/scheduler-plugins/pkg/fluence/fluxcli-grpc"
	"sigs.k8s.io/scheduler-plugins/pkg/fluence/utils"
)

type Fluence struct {
	mutex          sync.Mutex
	handle         framework.Handle
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
	v1alpha1.AddToScheme(scheme)
	k8scli, err := client.New(handle.KubeConfig(), client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

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

// getFluenceGroup uses a selector and lister to return the group of
// pods associated with a particular fluence label
func (f *Fluence) getFluenceGroup(groupName string) ([]*v1.Pod, error) {

	pods := []*v1.Pod{}

	// Prepare a label selector
	ls := metav1.LabelSelector{MatchLabels: map[string]string{PodGroupNameLabel: groupName}}

	// Get all Pods that potentially belong to this Deployment.
	selector, err := metav1.LabelSelectorAsSelector(&ls)
	if err != nil {
		return pods, err
	}
	pods, err = f.podLister.Pods("").List(selector)
	if err != nil {
		return pods, err
	}
	klog.Infof("Found %d pods", len(pods))
	return pods, nil
}

// getFluenceGroupTimestamp gets the time for the earliest created fluence group pod
// If the pod isn't in a Fluence group, we fall back it it's time
func (f *Fluence) getFluenceGroupTimestamp(pod *v1.Pod) time.Time {

	// This would be the "zero" equivalent of time we can check with time.isZero()
	ts := time.Time{}

	// First try to get the groupname. If no group, return pod creation timestamp
	groupName := f.getFluenceGroupName(pod)
	klog.Infof("group name is %s", groupName)
	if groupName == "" {
		return ts
	}

	// Now query for all pods in the group
	groupPods, err := f.getFluenceGroup(groupName)
	if err != nil {
		klog.Infof("get fluence group generated error: %s", err)
		return ts
	}
	// Sort based on creation time
	sort.Slice(groupPods, func(p1, p2 int) bool {
		return groupPods[p1].CreationTimestamp.Before(&groupPods[p2].CreationTimestamp)
	})

	klog.Infof("first time: %s", groupPods[0].CreationTimestamp.Time)
	klog.Infof("last time: %s", groupPods[len(groupPods)-1].CreationTimestamp.Time)
	return groupPods[0].CreationTimestamp.Time
}

// getFluenceGroup looks for the group to indicate a fluence group, and returns it
func (f *Fluence) getFluenceGroupName(pod *v1.Pod) string {
	groupName, _ := pod.Labels[PodGroupNameLabel]
	return groupName
}

// Less is used to sort pods in the scheduling queue in the following order.
// 1. Compare the priorities of Pods.
// 2. Compare the initialization timestamps of PodGroups or Pods.
// 3. Compare the keys of PodGroups/Pods: <namespace>/<podname>.
func (f *Fluence) Less(podInfo1, podInfo2 *framework.QueuedPodInfo) bool {
	klog.Infof("ordering pods from Coscheduling")

	// First preference to priority, but only if they are different
	prio1 := corev1helpers.PodPriority(podInfo1.Pod)
	prio2 := corev1helpers.PodPriority(podInfo2.Pod)

	// ...and only allow this to sort if they aren't the same
	if prio1 != prio2 {
		return prio1 > prio2
	}

	// Second priority goes to the fluence group name, which must be defined for both
	// We select the earliest time from the created pods in the group. It's OK if not
	// all are created because the earliest one will be.
	// UPDATE: this approach won't work for indexed jobs that complete
	fluenceTime1 := f.getFluenceGroupTimestamp(podInfo1.Pod)
	fluenceTime2 := f.getFluenceGroupTimestamp(podInfo2.Pod)
	klog.Infof("fluence time 1: %s", fluenceTime1)
	klog.Infof("fluence time 2: %s", fluenceTime2)

	// only comparable if not equal!
	if !fluenceTime1.Equal(fluenceTime2) {
		return fluenceTime1.Before(fluenceTime2)
	}

	// PodGroup is implemented in upstream but is a bit stale, but still support it if used
	// A pod without a PodGroup returns a timestamp of 0, so we either schedule any groups first
	creationTime1 := f.pgMgr.GetCreationTimestamp(podInfo1.Pod, *podInfo1.InitialAttemptTimestamp)
	creationTime2 := f.pgMgr.GetCreationTimestamp(podInfo2.Pod, *podInfo2.InitialAttemptTimestamp)

	// Or if they are both 0, fall back to sorting by name.
	if creationTime1.Equal(creationTime2) {
		return coschedulingcore.GetNamespacedName(podInfo1.Pod) < coschedulingcore.GetNamespacedName(podInfo2.Pod)
	}
	return creationTime1.Before(creationTime2)
}

// PreFilter
func (f *Fluence) PreFilter(
	ctx context.Context,
	state *framework.CycleState,
	pod *v1.Pod,
) (*framework.PreFilterResult, *framework.Status) {

	klog.Infof("Examining the pod")
	var err error
	var nodename string
	if pgname, ok := f.isGroup(ctx, pod); ok {
		if !fcore.HaveList(pgname) {
			klog.Infof("Getting a pod group")
			groupSize, _ := f.groupPreFilter(ctx, pod)
			if _, err = f.AskFlux(ctx, pod, groupSize); err != nil {
				return nil, framework.NewStatus(framework.Unschedulable, err.Error())
			}
		}
		nodename, err = fcore.GetNextNode(pgname)
		klog.Infof("Node Selected %s (%s:%s)", nodename, pod.Name, pgname)
		if err != nil {
			return nil, framework.NewStatus(framework.Unschedulable, err.Error())
		}
	} else {
		nodename, err = f.AskFlux(ctx, pod, 1)
		if err != nil {
			return nil, framework.NewStatus(framework.Unschedulable, err.Error())
		}
	}

	klog.Info("Node Selected: ", nodename)
	state.Write(framework.StateKey(pod.Name), &fcore.FluxStateData{NodeName: nodename})
	return nil, framework.NewStatus(framework.Success, "")

}

// isGroup determines if the pod belongs to a group known by fluence
// If a flux-framework/fluence.pod-group: <size> label is not provided,
func (f *Fluence) isGroup(ctx context.Context, pod *v1.Pod) (string, bool) {
	pgFullName, pg := f.pgMgr.GetPodGroup(ctx, pod)
	if pg == nil {
		klog.InfoS("Not in group", "pod", klog.KObj(pod))
		return "", false
	}
	return pgFullName, true
}

func (f *Fluence) groupPreFilter(ctx context.Context, pod *v1.Pod) (int, error) {
	// klog.InfoS("Flux Pre-Filter", "pod", klog.KObj(pod))
	klog.InfoS("Flux Pre-Filter", "pod labels", pod.Labels)
	_, pg := f.pgMgr.GetPodGroup(ctx, pod)
	if pg == nil {
		klog.InfoS("Not in group", "pod", klog.KObj(pod))
		return 0, nil
	}

	klog.Info("pod group members ", pg.Spec.MinMember)
	return int(pg.Spec.MinMember), nil
}

func (f *Fluence) Filter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	klog.Info("Filtering input node ", nodeInfo.Node().Name)
	if v, e := cycleState.Read(framework.StateKey(pod.Name)); e == nil {
		if value, ok := v.(*fcore.FluxStateData); ok && value.NodeName != nodeInfo.Node().Name {
			return framework.NewStatus(framework.Unschedulable, "pod is not permitted")
		} else {
			klog.Info("Filter: node selected by Flux ", value.NodeName)
		}
	}

	return framework.NewStatus(framework.Success)
}

func (f *Fluence) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func (f *Fluence) AskFlux(ctx context.Context, pod *v1.Pod, count int) (string, error) {
	// clean up previous match if a pod has already allocated previously
	f.mutex.Lock()
	_, isPodAllocated := f.podNameToJobId[pod.Name]
	f.mutex.Unlock()

	if isPodAllocated {
		klog.Info("Clean up previous allocation")
		f.mutex.Lock()
		f.cancelFluxJobForPod(pod.Name)
		f.mutex.Unlock()
	}

	jobspec := utils.InspectPodInfo(pod)
	conn, err := grpc.Dial("127.0.0.1:4242", grpc.WithInsecure())

	if err != nil {
		klog.Errorf("[FluxClient] Error connecting to server: %v", err)
		return "", err
	}
	defer conn.Close()

	grpcclient := pb.NewFluxcliServiceClient(conn)
	_, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	request := &pb.MatchRequest{
		Ps:      jobspec,
		Request: "allocate",
		Count:   int32(count)}

	r, err2 := grpcclient.Match(context.Background(), request)
	if err2 != nil {
		klog.Errorf("[FluxClient] did not receive any match response: %v", err2)
		return "", err
	}

	klog.Infof("[FluxClient] response podID %s", r.GetPodID())

	_, ok := f.isGroup(ctx, pod)
	if count > 1 || ok {
		pgFullName, _ := f.pgMgr.GetPodGroup(ctx, pod)
		nodelist := fcore.CreateNodePodsList(r.GetNodelist(), pgFullName)
		klog.Infof("[FluxClient] response nodeID %s", r.GetNodelist())
		klog.Info("[FluxClient] Parsed Nodelist ", nodelist)
		jobid := uint64(r.GetJobID())

		f.mutex.Lock()
		f.podNameToJobId[pod.Name] = jobid
		klog.Info("Check job set: ", f.podNameToJobId)
		f.mutex.Unlock()
	} else {
		nodename := r.GetNodelist()[0].GetNodeID()
		jobid := uint64(r.GetJobID())

		f.mutex.Lock()
		f.podNameToJobId[pod.Name] = jobid
		klog.Info("Check job set: ", f.podNameToJobId)
		f.mutex.Unlock()

		return nodename, nil
	}

	return "", nil
}

func (f *Fluence) cancelFluxJobForPod(podName string) error {
	jobid := f.podNameToJobId[podName]

	klog.Infof("Cancel flux job: %v for pod %s", jobid, podName)

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
		delete(f.podNameToJobId, podName)
	} else {
		klog.Warningf("Failed to delete pod %s from the podname-jobid map.", podName)
	}

	elapsed := metrics.SinceInSeconds(start)
	klog.Info("Time elapsed (Cancel Job) :", elapsed)

	klog.Infof("Job cancellation for pod %s result: %d", podName, err)
	if klog.V(2).Enabled() {
		klog.Info("Check job set: after delete")
		klog.Info(f.podNameToJobId)
	}
	return nil
}

// EventHandlers
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
			f.cancelFluxJobForPod(newPod.Name)
		} else {
			klog.Infof("Succeeded pod %s/%s doesn't have flux jobid", newPod.Namespace, newPod.Name)
		}
	case v1.PodFailed:
		// a corner case need to be tested, the pod exit code is not 0, can be created with segmentation fault pi test
		klog.Warningf("Pod %s failed, Fluence needs to free the resources", newPod.Name)

		f.mutex.Lock()
		defer f.mutex.Unlock()

		if _, ok := f.podNameToJobId[newPod.Name]; ok {
			f.cancelFluxJobForPod(newPod.Name)
		} else {
			klog.Errorf("Failed pod %s/%s doesn't have flux jobid", newPod.Namespace, newPod.Name)
		}
	case v1.PodUnknown:
		// don't know how to deal with it as it's unknown phase
	default:
		// shouldn't enter this branch
	}
}

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
			f.cancelFluxJobForPod(pod.Name)
		} else {
			klog.Infof("Terminating pod %s/%s doesn't have flux jobid", pod.Namespace, pod.Name)
		}
	case v1.PodRunning:
		f.mutex.Lock()
		defer f.mutex.Unlock()

		if _, ok := f.podNameToJobId[pod.Name]; ok {
			f.cancelFluxJobForPod(pod.Name)
		} else {
			klog.Infof("Deleted pod %s/%s doesn't have flux jobid", pod.Namespace, pod.Name)
		}
	}
}
