package fluence

import (
	"context"
	"time"

	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	klog "k8s.io/klog/v2"

	pb "sigs.k8s.io/scheduler-plugins/pkg/fluence/fluxcli-grpc"
)

// Events are associated with inforers, typically on pods, e.g.,
// delete: deletion of a pod
// update: update of a pod!
// For both of the above, there are cases to cancel the flux job
//  associated with the group id

// cancelFluxJobForPod cancels the flux job for a pod.
// We assume that the cancelled job also means deleting the pod group
func (f *Fluence) cancelFluxJob(groupName string) error {

	jobid, ok := f.groupToJobId[groupName]

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
		delete(f.groupToJobId, groupName)
	} else {
		klog.Warningf("[Fluence] Failed to cancel flux job %d for group %s", jobid, groupName)
	}
	return nil
}

// updatePod is called on an update, and the old and new object are presented
func (f *Fluence) updatePod(oldObj, newObj interface{}) {

	oldPod := oldObj.(*v1.Pod)
	newPod := newObj.(*v1.Pod)

	// a pod is updated, get the group
	// TODO should we be checking group / size for old vs new?
	groupName, _ := f.pgMgr.GetPodGroup(context.TODO(), oldPod)

	klog.Infof("[Fluence] Processing event for pod %s in group %s from %s to %s", newPod.Name, groupName, newPod.Status.Phase, oldPod.Status.Phase)

	switch newPod.Status.Phase {
	case v1.PodPending:
		// in this state we don't know if a pod is going to be running, thus we don't need to update job map
	case v1.PodRunning:
		// if a pod is start running, we can add it state to the delta graph if it is scheduled by other scheduler
	case v1.PodSucceeded:
		klog.Infof("[Fluence] Pod %s succeeded, Fluence needs to free the resources", newPod.Name)

		f.mutex.Lock()
		defer f.mutex.Unlock()

		// Do we have the group id in our cache? If yes, we haven't deleted the jobid yet
		// I am worried here that if some pods are succeeded and others pending, this could
		// be a mistake - fluence would schedule it again
		_, ok := f.groupToJobId[groupName]
		if ok {
			f.cancelFluxJob(groupName)
		} else {
			klog.Infof("[Fluence] Succeeded pod %s/%s in group %s doesn't have flux jobid", newPod.Namespace, newPod.Name, groupName)
		}

	case v1.PodFailed:

		// a corner case need to be tested, the pod exit code is not 0, can be created with segmentation fault pi test
		klog.Warningf("[Fluence] Pod %s in group %s failed, Fluence needs to free the resources", newPod.Name, groupName)

		f.mutex.Lock()
		defer f.mutex.Unlock()

		_, ok := f.groupToJobId[groupName]
		if ok {
			f.cancelFluxJob(groupName)
		} else {
			klog.Errorf("[Fluence] Failed pod %s/%s in group %s doesn't have flux jobid", newPod.Namespace, newPod.Name, groupName)
		}
	case v1.PodUnknown:
		// don't know how to deal with it as it's unknown phase
	default:
		// shouldn't enter this branch
	}
}

// deletePod handles the delete event handler
func (f *Fluence) deletePod(podObj interface{}) {
	klog.Info("[Fluence] Delete Pod event handler")
	pod := podObj.(*v1.Pod)
	groupName, _ := f.pgMgr.GetPodGroup(context.TODO(), pod)

	klog.Infof("[Fluence] Delete pod %s in group %s has status %s", pod.Status.Phase, pod.Name, groupName)
	switch pod.Status.Phase {
	case v1.PodSucceeded:
	case v1.PodPending:
		klog.Infof("[Fluence] Pod %s completed and is Pending termination, Fluence needs to free the resources", pod.Name)

		f.mutex.Lock()
		defer f.mutex.Unlock()

		_, ok := f.groupToJobId[groupName]
		if ok {
			f.cancelFluxJob(groupName)
		} else {
			klog.Infof("[Fluence] Terminating pod %s/%s in group %s doesn't have flux jobid", pod.Namespace, pod.Name, groupName)
		}
	case v1.PodRunning:
		f.mutex.Lock()
		defer f.mutex.Unlock()

		_, ok := f.groupToJobId[groupName]
		if ok {
			f.cancelFluxJob(groupName)
		} else {
			klog.Infof("[Fluence] Deleted pod %s/%s in group %s doesn't have flux jobid", pod.Namespace, pod.Name, groupName)
		}
	}
}
