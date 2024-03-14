package core

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/labels"
	klog "k8s.io/klog/v2"
	pb "sigs.k8s.io/scheduler-plugins/pkg/fluence/fluxcli-grpc"
	fgroup "sigs.k8s.io/scheduler-plugins/pkg/fluence/group"

	"sigs.k8s.io/scheduler-plugins/apis/scheduling/v1alpha1"
	"sigs.k8s.io/scheduler-plugins/pkg/fluence/utils"

	corev1 "k8s.io/api/core/v1"
)

// AskFlux will ask flux for an allocation for nodes for the pod group.
// We return the list of nodes, and assign to the entire group!
func (pgMgr *PodGroupManager) AskFlux(
	ctx context.Context,
	pod corev1.Pod,
	pg *v1alpha1.PodGroup,
	groupName string,
) ([]string, error) {

	// clean up previous match if a pod has already allocated previously
	pgMgr.mutex.Lock()
	_, isAllocated := pgMgr.groupToJobId[groupName]
	pgMgr.mutex.Unlock()

	// This case happens when there is some reason that an initial job pods partially allocated,
	// but then the job restarted, and new pods are present but fluence had assigned nodes to
	// the old ones (and there aren't enough). The job would have had to complete in some way,
	// and the PodGroup would have to then recreate, and have the same job id (the group name).
	// This happened when I cancalled a bunch of jobs and they didn't have the chance to
	// cancel in fluence. What we can do here is assume the previous pods are no longer running
	// and cancel the flux job to create again.
	if isAllocated {
		klog.Info("Warning - group %s was previously allocated and is requesting again, so must have completed.", groupName)
		pgMgr.mutex.Lock()
		pgMgr.cancelFluxJob(groupName, &pod)
		pgMgr.mutex.Unlock()
	}
	nodes := []string{}

	// IMPORTANT: this is a JobSpec for *one* pod, assuming they are all the same.
	// This obviously may not be true if we have a hetereogenous PodGroup.
	// We name it based on the group, since it will represent the group
	jobspec := utils.PreparePodJobSpec(&pod, groupName)
	klog.Infof("[Fluence] Inspect pod info, jobspec: %s\n", jobspec)
	conn, err := grpc.Dial("127.0.0.1:4242", grpc.WithInsecure())

	// TODO change this to just return fmt.Errorf
	if err != nil {
		klog.Errorf("[Fluence] Error connecting to server: %v\n", err)
		return nodes, err
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
		return nodes, err
	}

	// TODO GetPodID should be renamed, because it will reflect the group
	klog.Infof("[Fluence] Match response ID %s\n", r.GetPodID())

	// Get the nodelist and inspect
	nodelist := r.GetNodelist()
	for _, node := range nodelist {
		nodes = append(nodes, node.NodeID)
	}
	jobid := uint64(r.GetJobID())
	klog.Infof("[Fluence] parsed node pods list %s for job id %d\n", nodes, jobid)

	// TODO would be nice to actually be able to ask flux jobs -a to fluence
	// That way we can verify assignments, etc.
	pgMgr.mutex.Lock()
	pgMgr.groupToJobId[groupName] = jobid
	pgMgr.mutex.Unlock()
	return nodes, nil
}

// cancelFluxJobForPod cancels the flux job for a pod.
// We assume that the cancelled job also means deleting the pod group
func (pgMgr *PodGroupManager) cancelFluxJob(groupName string, pod *corev1.Pod) error {

	jobid, ok := pgMgr.groupToJobId[groupName]

	// The job was already cancelled by another pod
	if !ok {
		klog.Infof("[Fluence] Request for cancel of group %s is already complete.", groupName)
		return nil
	}
	klog.Infof("[Fluence] Cancel flux job: %v for group %s", jobid, groupName)

	// This first error is about connecting to the server
	conn, err := grpc.Dial("127.0.0.1:4242", grpc.WithInsecure())
	if err != nil {
		klog.Errorf("[Fluence] Error connecting to server: %v", err)
		return err
	}
	defer conn.Close()

	grpcclient := pb.NewFluxcliServiceClient(conn)
	_, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	// This error reflects the success or failure of the cancel request
	request := &pb.CancelRequest{JobID: int64(jobid)}
	res, err := grpcclient.Cancel(context.Background(), request)
	if err != nil {
		klog.Errorf("[Fluence] did not receive any cancel response: %v", err)
		return err
	}
	klog.Infof("[Fluence] Job cancellation for group %s result: %d", groupName, res.Error)

	// And this error is if the cancel was successful or not
	if res.Error == 0 {
		klog.Infof("[Fluence] Successful cancel of flux job: %d for group %s", jobid, groupName)
		pgMgr.cleanup(pod, groupName)
	} else {
		klog.Warningf("[Fluence] Failed to cancel flux job %d for group %s", jobid, groupName)
	}
	return nil
}

// cleanup deletes the group name from groupToJobId, and pods names from the node lookup
func (pgMgr *PodGroupManager) cleanup(pod *corev1.Pod, groupName string) {

	delete(pgMgr.groupToJobId, groupName)

	// Clean up previous pod->node assignments
	pods, err := pgMgr.podLister.Pods(pod.Namespace).List(
		labels.SelectorFromSet(labels.Set{v1alpha1.PodGroupLabel: groupName}),
	)
	// TODO need to handle this / understand why it's the case
	if err != nil {
		return
	}
	for _, pod := range pods {
		delete(pgMgr.podToNode, pod.Name)
	}
}

// UpdatePod is called on an update, and the old and new object are presented
func (pgMgr *PodGroupManager) UpdatePod(oldObj, newObj interface{}) {

	oldPod := oldObj.(*corev1.Pod)
	newPod := newObj.(*corev1.Pod)

	// a pod is updated, get the group
	// TODO should we be checking group / size for old vs new?
	groupName, pg := pgMgr.GetPodGroup(context.TODO(), oldPod)

	// If PodGroup is nil, still try to look up a faux name
	// TODO need to check if this might be problematic
	if pg == nil {
		pg = fgroup.CreateFakeGroup(oldPod)
		groupName = pg.Name
	}

	klog.Infof("[Fluence] Processing event for pod %s in group %s from %s to %s", newPod.Name, groupName, oldPod.Status.Phase, newPod.Status.Phase)

	switch newPod.Status.Phase {
	case corev1.PodPending:
		// in this state we don't know if a pod is going to be running, thus we don't need to update job map
	case corev1.PodRunning:
		// if a pod is start running, we can add it state to the delta graph if it is scheduled by other scheduler
	case corev1.PodSucceeded:
		klog.Infof("[Fluence] Pod %s succeeded, Fluence needs to free the resources", newPod.Name)

		pgMgr.mutex.Lock()
		defer pgMgr.mutex.Unlock()

		// Do we have the group id in our cache? If yes, we haven't deleted the jobid yet
		// I am worried here that if some pods are succeeded and others pending, this could
		// be a mistake - fluence would schedule it again
		_, ok := pgMgr.groupToJobId[groupName]
		if ok {
			pgMgr.cancelFluxJob(groupName, oldPod)
		} else {
			klog.Infof("[Fluence] Succeeded pod %s/%s in group %s doesn't have flux jobid", newPod.Namespace, newPod.Name, groupName)
		}

	case corev1.PodFailed:

		// a corner case need to be tested, the pod exit code is not 0, can be created with segmentation fault pi test
		klog.Warningf("[Fluence] Pod %s in group %s failed, Fluence needs to free the resources", newPod.Name, groupName)

		pgMgr.mutex.Lock()
		defer pgMgr.mutex.Unlock()

		_, ok := pgMgr.groupToJobId[groupName]
		if ok {
			pgMgr.cancelFluxJob(groupName, oldPod)
		} else {
			klog.Errorf("[Fluence] Failed pod %s/%s in group %s doesn't have flux jobid", newPod.Namespace, newPod.Name, groupName)
		}
	case corev1.PodUnknown:
		// don't know how to deal with it as it's unknown phase
	default:
		// shouldn't enter this branch
	}
}

// DeletePod handles the delete event handler
func (pgMgr *PodGroupManager) DeletePod(podObj interface{}) {
	klog.Info("[Fluence] Delete Pod event handler")
	pod := podObj.(*corev1.Pod)
	groupName, pg := pgMgr.GetPodGroup(context.TODO(), pod)

	// If PodGroup is nil, still try to look up a faux name
	if pg == nil {
		pg = fgroup.CreateFakeGroup(pod)
		groupName = pg.Name
	}

	klog.Infof("[Fluence] Delete pod %s in group %s has status %s", pod.Status.Phase, pod.Name, groupName)
	switch pod.Status.Phase {
	case corev1.PodSucceeded:
	case corev1.PodPending:
		klog.Infof("[Fluence] Pod %s completed and is Pending termination, Fluence needs to free the resources", pod.Name)

		pgMgr.mutex.Lock()
		defer pgMgr.mutex.Unlock()

		_, ok := pgMgr.groupToJobId[groupName]
		if ok {
			pgMgr.cancelFluxJob(groupName, pod)
		} else {
			klog.Infof("[Fluence] Terminating pod %s/%s in group %s doesn't have flux jobid", pod.Namespace, pod.Name, groupName)
		}
	case corev1.PodRunning:
		pgMgr.mutex.Lock()
		defer pgMgr.mutex.Unlock()

		_, ok := pgMgr.groupToJobId[groupName]
		if ok {
			pgMgr.cancelFluxJob(groupName, pod)
		} else {
			klog.Infof("[Fluence] Deleted pod %s/%s in group %s doesn't have flux jobid", pod.Namespace, pod.Name, groupName)
		}
	}
}
