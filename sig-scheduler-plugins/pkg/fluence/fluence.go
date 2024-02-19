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
	klog "k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"sigs.k8s.io/controller-runtime/pkg/client"
	sched "sigs.k8s.io/scheduler-plugins/apis/scheduling/v1alpha1"
	coschedulingcore "sigs.k8s.io/scheduler-plugins/pkg/coscheduling/core"
	fcore "sigs.k8s.io/scheduler-plugins/pkg/fluence/core"
	pb "sigs.k8s.io/scheduler-plugins/pkg/fluence/fluxcli-grpc"
	fgroup "sigs.k8s.io/scheduler-plugins/pkg/fluence/group"
	"sigs.k8s.io/scheduler-plugins/pkg/fluence/utils"
)

type Fluence struct {
	mutex  sync.Mutex
	handle framework.Handle
	client client.Client

	// Store jobid on the level of a group (which can be a single pod)
	groupToJobId map[string]uint64
	pgMgr        coschedulingcore.Manager
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

	f := &Fluence{handle: handle, groupToJobId: make(map[string]uint64)}

	ctx := context.TODO()
	fcore.Init()

	fluxPodsInformer := handle.SharedInformerFactory().Core().V1().Pods().Informer()
	fluxPodsInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: f.updatePod,
		DeleteFunc: f.deletePod,
	})

	go fluxPodsInformer.Run(ctx.Done())

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
		klog.Errorf("ParseSelector failed %s", err)
		os.Exit(1)
	}

	informerFactory := informers.NewSharedInformerFactoryWithOptions(handle.ClientSet(), 0, informers.WithTweakListOptions(func(opt *metav1.ListOptions) {
		opt.FieldSelector = fieldSelector.String()
	}))
	podInformer := informerFactory.Core().V1().Pods()
	scheduleTimeDuration := time.Duration(500) * time.Second

	// https://github.com/kubernetes-sigs/scheduler-plugins/blob/master/pkg/coscheduling/core/core.go#L84
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

	klog.Info("Fluence scheduler plugin started")
	return f, nil
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
		return coschedulingcore.GetNamespacedName(podInfo1.Pod) < coschedulingcore.GetNamespacedName(podInfo2.Pod)
	}
	return creationTime1.Before(&creationTime2)
}

// PreFilter checks info about the Pod / checks conditions that the cluster or the Pod must meet.
// This comes after sort
func (f *Fluence) PreFilter(
	ctx context.Context,
	state *framework.CycleState,
	pod *v1.Pod,
) (*framework.PreFilterResult, *framework.Status) {

	klog.Infof("[Fluence] Examining pod %s", pod.Name)

	// groupName will be named according to the single pod namespace / pod if there wasn't
	// a user defined group. This is a size 1 group we handle equivalently.
	groupName, pg := f.pgMgr.GetPodGroup(ctx, pod)

	// If we don't have a pod group and it's here, it was asked to be scheduled by fluence
	// but the group isn't ready. Unshedulable for now.
	if pg == nil {
		klog.Infof("[Fluence] Group %s/%s does not have a pod group, not schedulable yet.", pod.Namespace, pod.Name)
		return nil, framework.NewStatus(framework.Unschedulable, "Missing podgroup")
	}
	klog.Infof("[Fluence] Pod %s is in group %s with minimum members %d", pod.Name, groupName, pg.Spec.MinMember)

	// Has this podgroup been seen by fluence yet? If yes, we will have it in the cache
	cache := fcore.GetFluenceCache(groupName)
	klog.Infof("[Fluence] cache %s", cache)

	// Fluence has never seen this before, we need to schedule an allocation
	// It also could have been seen, but was not able to get one.
	if cache == nil {
		klog.Infof("[Fluence] Does not have nodes for %s yet, asking Fluxion", groupName)

		// groupName is the namespaced name <namespace>/<name>
		err := f.AskFlux(ctx, pod, pg, groupName)
		if err != nil {
			klog.Infof("[Fluence] Fluxion returned an error %s, not schedulable", err.Error())
			return nil, framework.NewStatus(framework.Unschedulable, err.Error())
		}
	}

	// This is the next node in the list
	nodename, err := fcore.GetNextNode(groupName)
	if err != nil {
		return nil, framework.NewStatus(framework.Unschedulable, err.Error())
	}
	klog.Infof("Node Selected %s (pod %s:group %s)", nodename, pod.Name, groupName)

	// Create a fluxState (CycleState) with things that might be useful
	// This isn't a PodGroupCache, but a single node cache, which also
	// has group information, but just is for one node. Note that assigned
	// tasks is hard coded to 1 but this isn't necessarily the case - we should
	// eventually be able to GetNextNode for a number of tasks, for example
	// (unless task == pod in which case it is always 1)
	nodeCache := fcore.NodeCache{NodeName: nodename, GroupName: groupName, AssignedTasks: 1}
	state.Write(framework.StateKey(pod.Name), &fcore.FluxStateData{NodeCache: nodeCache})
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
	state, err := cycleState.Read(framework.StateKey(pod.Name))

	// No error means we retrieved the state
	if err == nil {

		// Try to convert the state to FluxStateDate
		value, ok := state.(*fcore.FluxStateData)

		// If we have state data that isn't equal to the current assignment, no go
		if ok && value.NodeCache.NodeName != nodeInfo.Node().Name {
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
func (f *Fluence) AskFlux(
	ctx context.Context,
	pod *v1.Pod,
	pg *sched.PodGroup,
	groupName string,
) error {

	// clean up previous match if a pod has already allocated previously
	f.mutex.Lock()
	_, isAllocated := f.groupToJobId[groupName]
	f.mutex.Unlock()

	// This case happens when there is some reason that an initial job pods partially allocated,
	// but then the job restarted, and new pods are present but fluence had assigned nodes to
	// the old ones (and there aren't enough). The job would have had to complete in some way,
	// and the PodGroup would have to then recreate, and have the same job id (the group name).
	// This happened when I cancalled a bunch of jobs and they didn't have the chance to
	// cancel in fluence. What we can do here is assume the previous pods are no longer running
	// and cancel the flux job to create again.
	if isAllocated {
		klog.Info("Warning - group %s was previously allocated and is requesting again, so must have completed.", groupName)
		f.mutex.Lock()
		f.cancelFluxJob(groupName)
		f.mutex.Unlock()
	}

	// IMPORTANT: this is a JobSpec for *one* pod, assuming they are all the same.
	// This obviously may not be true if we have a hetereogenous PodGroup.
	// We name it based on the group, since it will represent the group
	jobspec := utils.PreparePodJobSpec(pod, groupName)
	klog.Infof("[Fluence] Inspect pod info, jobspec: %s\n", jobspec)
	conn, err := grpc.Dial("127.0.0.1:4242", grpc.WithInsecure())

	// TODO change this to just return fmt.Errorf
	if err != nil {
		klog.Errorf("[Fluence] Error connecting to server: %v\n", err)
		return err
	}
	defer conn.Close()

	grpcclient := pb.NewFluxcliServiceClient(conn)
	_, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	request := &pb.MatchRequest{
		Ps:      jobspec,
		Request: "allocate",
		Count:   pg.Spec.MinMember,
	}

	// An error here is an error with making the request
	r, err := grpcclient.Match(context.Background(), request)
	if err != nil {
		klog.Errorf("[Fluence] did not receive any match response: %v\n", err)
		return err
	}

	// TODO GetPodID should be renamed, because it will reflect the group
	klog.Infof("[Fluence] Match response ID %s\n", r.GetPodID())

	// Get the nodelist and inspect
	nodes := r.GetNodelist()
	klog.Infof("[Fluence] Nodelist returned from Fluxion: %s\n", nodes)

	// Assign the nodelist - this sets the group name in the groupSeen cache
	// at this point, we can retrieve the cache and get nodes
	nodelist := fcore.CreateNodeList(nodes, groupName)

	jobid := uint64(r.GetJobID())
	klog.Infof("[Fluence] parsed node pods list %s for job id %d\n", nodelist, jobid)

	// TODO would be nice to actually be able to ask flux jobs -a to fluence
	// That way we can verify assignments, etc.
	f.mutex.Lock()
	f.groupToJobId[groupName] = jobid
	f.mutex.Unlock()
	return nil
}
