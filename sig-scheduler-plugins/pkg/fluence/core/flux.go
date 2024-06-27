package core

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/labels"
	pb "sigs.k8s.io/scheduler-plugins/pkg/fluence/fluxcli-grpc"
	fgroup "sigs.k8s.io/scheduler-plugins/pkg/fluence/group"

	"sigs.k8s.io/scheduler-plugins/apis/scheduling/v1alpha1"
	"sigs.k8s.io/scheduler-plugins/pkg/fluence/podspec"

	corev1 "k8s.io/api/core/v1"
)

// AskFlux will ask flux for an allocation for nodes for the pod group.
// We return the list of nodes, and assign to the entire group!
func (podGroupManager *PodGroupManager) AskFlux(
	ctx context.Context,
	pod corev1.Pod,
	podGroup *v1alpha1.PodGroup,
	groupName string,
) ([]string, error) {

	// clean up previous match if a pod has already allocated previously
	podGroupManager.mutex.Lock()
	_, isAllocated := podGroupManager.groupToJobId[groupName]
	podGroupManager.mutex.Unlock()

	// This case happens when there is some reason that an initial job pods partially allocated,
	// but then the job restarted, and new pods are present but fluence had assigned nodes to
	// the old ones (and there aren't enough). The job would have had to complete in some way,
	// and the PodGroup would have to then recreate, and have the same job id (the group name).
	// This happened when I cancalled a bunch of jobs and they didn't have the chance to
	// cancel in fluence. What we can do here is assume the previous pods are no longer running
	// and cancel the flux job to create again.
	if isAllocated {
		podGroupManager.log.Warning("[PodGroup AskFlux] group %s was previously allocated and is requesting again, so must have completed.", groupName)
		podGroupManager.mutex.Lock()
		podGroupManager.cancelFluxJob(groupName, &pod)
		podGroupManager.mutex.Unlock()
	}
	nodes := []string{}

	// IMPORTANT: this is a JobSpec for *one* pod, assuming they are all the same.
	// This obviously may not be true if we have a hetereogenous PodGroup.
	// We name it based on the group, since it will represent the group
	jobspec := podspec.PreparePodJobSpec(&pod, groupName)
	podGroupManager.log.Info("[PodGroup AskFlux] Inspect pod info, jobspec: %s\n", jobspec)
	conn, err := grpc.Dial("127.0.0.1:4242", grpc.WithInsecure())

	// TODO change this to just return fmt.Errorf
	if err != nil {
		podGroupManager.log.Error("[PodGroup AskFlux] Error connecting to server: %v\n", err)
		return nodes, err
	}
	defer conn.Close()

	grpcclient := pb.NewFluxcliServiceClient(conn)
	_, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	request := &pb.MatchRequest{
		Ps:      jobspec,
		Request: "allocate",
		Count:   podGroup.Spec.MinMember,
	}

	// An error here is an error with making the request
	response, err := grpcclient.Match(context.Background(), request)
	if err != nil {
		podGroupManager.log.Warning("[PodGroup AskFlux] did not receive any match response: %v\n", err)
		return nodes, err
	}

	// TODO GetPodID should be renamed, because it will reflect the group
	podGroupManager.log.Info("[PodGroup AskFlux] Match response ID %s\n", response.GetPodID())

	// Get the nodelist and inspect
	nodelist := response.GetNodelist()
	for _, node := range nodelist {
		nodes = append(nodes, node.NodeID)
	}
	jobid := uint64(response.GetJobID())
	podGroupManager.log.Info("[PodGroup AskFlux] parsed node pods list %s for job id %d\n", nodes, jobid)

	// TODO would be nice to actually be able to ask flux jobs -a to fluence
	// That way we can verify assignments, etc.
	podGroupManager.mutex.Lock()
	podGroupManager.groupToJobId[groupName] = jobid
	podGroupManager.mutex.Unlock()
	return nodes, nil
}

// cancelFluxJobForPod cancels the flux job for a pod.
// We assume that the cancelled job also means deleting the pod group
func (podGroupManager *PodGroupManager) cancelFluxJob(groupName string, pod *corev1.Pod) error {

	jobid, exists := podGroupManager.groupToJobId[groupName]

	// The job was already cancelled by another pod
	if !exists {
		podGroupManager.log.Info("[PodGroup cancelFluxJob] Request for cancel of group %s is already complete.", groupName)
		return nil
	}
	podGroupManager.log.Info("[PodGroup cancelFluxJob] Cancel flux job: %v for group %s", jobid, groupName)

	// This first error is about connecting to the server
	conn, err := grpc.Dial("127.0.0.1:4242", grpc.WithInsecure())
	if err != nil {
		podGroupManager.log.Error("[PodGroup cancelFluxJob] Error connecting to server: %v", err)
		return err
	}
	defer conn.Close()

	grpcclient := pb.NewFluxcliServiceClient(conn)
	_, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	// This error reflects the success or failure of the cancel request
	request := &pb.CancelRequest{JobID: int64(jobid)}
	response, err := grpcclient.Cancel(context.Background(), request)
	if err != nil {
		podGroupManager.log.Error("[PodGroup cancelFluxJob] did not receive any cancel response: %v", err)
		return err
	}
	podGroupManager.log.Info("[PodGroup cancelFluxJob] Job cancellation for group %s result: %d", groupName, response.Error)

	// And this error is if the cancel was successful or not
	if response.Error == 0 {
		podGroupManager.log.Info("[PodGroup cancelFluxJob] Successful cancel of flux job: %d for group %s", jobid, groupName)
		podGroupManager.cleanup(pod, groupName)
	} else {
		podGroupManager.log.Warning("[PodGroup cancelFluxJob] Failed to cancel flux job %d for group %s", jobid, groupName)
	}
	return nil
}

// cleanup deletes the group name from groupToJobId, and pods names from the node lookup
func (podGroupManager *PodGroupManager) cleanup(pod *corev1.Pod, groupName string) {

	delete(podGroupManager.groupToJobId, groupName)

	// Clean up previous pod->node assignments
	pods, err := podGroupManager.podLister.Pods(pod.Namespace).List(
		labels.SelectorFromSet(labels.Set{v1alpha1.PodGroupLabel: groupName}),
	)
	// TODO need to handle this / understand why it's the case
	if err != nil {
		return
	}
	for _, pod := range pods {
		delete(podGroupManager.podToNode, pod.Name)
	}
}

// UpdatePod is called on an update, and the old and new object are presented
func (podGroupManager *PodGroupManager) UpdatePod(oldObj, newObj interface{}) {

	oldPod := oldObj.(*corev1.Pod)
	newPod := newObj.(*corev1.Pod)

	// a pod is updated, get the group
	// TODO should we be checking group / size for old vs new?
	groupName, podGroup := podGroupManager.GetPodGroup(context.TODO(), oldPod)

	// If PodGroup is nil, still try to look up a faux name
	// TODO need to check if this might be problematic
	if podGroup == nil {
		podGroup = fgroup.CreateFakeGroup(oldPod)
		groupName = podGroup.Name
	}

	podGroupManager.log.Verbose("[PodGroup UpdatePod] Processing event for pod %s in group %s from %s to %s", newPod.Name, groupName, oldPod.Status.Phase, newPod.Status.Phase)

	switch newPod.Status.Phase {
	case corev1.PodPending:
		// in this state we don't know if a pod is going to be running, thus we don't need to update job map
	case corev1.PodRunning:
		// if a pod is start running, we can add it state to the delta graph if it is scheduled by other scheduler
	case corev1.PodSucceeded:
		podGroupManager.log.Info("[PodGroup UpdatePod] Pod %s succeeded, Fluence needs to free the resources", newPod.Name)

		podGroupManager.mutex.Lock()
		defer podGroupManager.mutex.Unlock()

		// Do we have the group id in our cache? If yes, we haven't deleted the jobid yet
		// I am worried here that if some pods are succeeded and others pending, this could
		// be a mistake - fluence would schedule it again
		_, exists := podGroupManager.groupToJobId[groupName]
		if exists {
			podGroupManager.cancelFluxJob(groupName, oldPod)
		} else {
			podGroupManager.log.Verbose("[PodGroup UpdatePod] Succeeded pod %s/%s in group %s doesn't have flux jobid", newPod.Namespace, newPod.Name, groupName)
		}

	case corev1.PodFailed:

		// a corner case need to be tested, the pod exit code is not 0, can be created with segmentation fault pi test
		podGroupManager.log.Warning("[PodGroup UpdatePod] Pod %s in group %s failed, Fluence needs to free the resources", newPod.Name, groupName)

		podGroupManager.mutex.Lock()
		defer podGroupManager.mutex.Unlock()

		_, exists := podGroupManager.groupToJobId[groupName]
		if exists {
			podGroupManager.cancelFluxJob(groupName, oldPod)
		} else {
			podGroupManager.log.Error("[PodGroup UpdatePod] Failed pod %s/%s in group %s doesn't have flux jobid", newPod.Namespace, newPod.Name, groupName)
		}
	case corev1.PodUnknown:
		// don't know how to deal with it as it's unknown phase
	default:
		// shouldn't enter this branch
	}
}

// DeletePod handles the delete event handler
func (podGroupManager *PodGroupManager) DeletePod(podObj interface{}) {
	pod := podObj.(*corev1.Pod)
	groupName, podGroup := podGroupManager.GetPodGroup(context.TODO(), pod)

	// If PodGroup is nil, still try to look up a faux name
	if podGroup == nil {
		podGroup = fgroup.CreateFakeGroup(pod)
		groupName = podGroup.Name
	}

	podGroupManager.log.Verbose("[PodGroup DeletePod] Delete pod %s in group %s has status %s", pod.Status.Phase, pod.Name, groupName)
	switch pod.Status.Phase {
	case corev1.PodSucceeded:
	case corev1.PodPending:
		podGroupManager.log.Verbose("[PodGroup DeletePod] Pod %s completed and is Pending termination, Fluence needs to free the resources", pod.Name)

		podGroupManager.mutex.Lock()
		defer podGroupManager.mutex.Unlock()

		_, exists := podGroupManager.groupToJobId[groupName]
		if exists {
			podGroupManager.cancelFluxJob(groupName, pod)
		} else {
			podGroupManager.log.Info("[PodGroup DeletePod] Terminating pod %s/%s in group %s doesn't have flux jobid", pod.Namespace, pod.Name, groupName)
		}
	case corev1.PodRunning:
		podGroupManager.mutex.Lock()
		defer podGroupManager.mutex.Unlock()

		_, exists := podGroupManager.groupToJobId[groupName]
		if exists {
			podGroupManager.cancelFluxJob(groupName, pod)
		} else {
			podGroupManager.log.Info("[PodGroup DeletePod] Deleted pod %s/%s in group %s doesn't have flux jobid", pod.Namespace, pod.Name, groupName)
		}
	}
}
