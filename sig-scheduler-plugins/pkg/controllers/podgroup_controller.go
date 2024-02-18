/*
Copyright 2023 The Kubernetes Authors.

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

package controllers

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	schedv1alpha1 "sigs.k8s.io/scheduler-plugins/apis/scheduling/v1alpha1"
	fluenceLabels "sigs.k8s.io/scheduler-plugins/pkg/fluence/labels"
	"sigs.k8s.io/scheduler-plugins/pkg/util"
)

// PodGroupReconciler reconciles a PodGroup object
type PodGroupReconciler struct {
	log      logr.Logger
	recorder record.EventRecorder

	client.Client
	Scheme  *runtime.Scheme
	Workers int
}

// +kubebuilder:rbac:groups=scheduling.x-k8s.io,resources=podgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.x-k8s.io,resources=podgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scheduling.x-k8s.io,resources=podgroups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PodGroup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *PodGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("reconciling flux-framework/fluence-controller for request")
	pg := &schedv1alpha1.PodGroup{}

	if err := r.Get(ctx, req.NamespacedName, pg); err != nil {

		// Case 1: if we get here and it's not found, assume not created
		if apierrs.IsNotFound(err) {
			log.Info("PodGroup", "Status", fmt.Sprintf("Pod group %s is not found, deleted.", req.NamespacedName))
			return ctrl.Result{}, nil
		}
		log.Error(err, fmt.Sprintf("Unable to retrieve pod group %s", req.NamespacedName))
		return ctrl.Result{}, err
	}

	// Grab all statuses (and groups of them) we are interested in
	schedulingOrPending := (pg.Status.Phase == schedv1alpha1.PodGroupScheduling || pg.Status.Phase == schedv1alpha1.PodGroupPending)
	twoDaysOld := pg.Status.ScheduleStartTime.Sub(pg.CreationTimestamp.Time) > 48*time.Hour
	finishedOrFailed := pg.Status.Phase == schedv1alpha1.PodGroupFinished || pg.Status.Phase == schedv1alpha1.PodGroupFailed

	// Finished or failed - clean up the group
	if finishedOrFailed {
		log.Info("PodGroup", "Status", fmt.Sprintf("Pod group %s is finished or failed.", req.NamespacedName))
		return ctrl.Result{}, nil
	}

	// If startScheduleTime - createTime > 2days,
	// do not reconcile again because pod may have been GCed
	if schedulingOrPending && pg.Status.Running == 0 && twoDaysOld {
		r.recorder.Event(pg, v1.EventTypeWarning, "Timeout", "schedule time longer than 48 hours")
		return ctrl.Result{}, nil
	}

	// We can get the podList and check for sizes here
	podList := &v1.PodList{}

	// Select based on the group name
	groupNameSelector := labels.Set(map[string]string{schedv1alpha1.PodGroupLabel: pg.Name}).AsSelector()
	err := r.List(ctx, podList, client.MatchingLabelsSelector{Selector: groupNameSelector})
	if err != nil {
		log.Error(err, "List pods for group failed")
		return ctrl.Result{}, err
	}

	// Inspect the size, set on the group if not done yet
	size := len(podList.Items)
	log.Info("PodGroup", "Name", pg.Name, "Size", size)

	// When first created, size should be unset (MinMember)
	if int(pg.Spec.MinMember) == 0 {
		log.Info("PodGroup", "Status", fmt.Sprintf("Pod group %s updating size to %d", pg.Name, size))
		return r.updatePodGroupSize(ctx, pg, int32(size))

	} else if int(pg.Spec.MinMember) != size {
		// TODO: Not clear what to do here. Arguably, we also want to check the label size
		// because (in the future) we can accept smaller sizes. But then we also need
		// to account for if the labels are different, do we take the smallest?
		log.Info("PodGroup", "Status", fmt.Sprintf("WARNING: Pod group current MinMember %s does not match %d", pg.Spec.MinMember, size))
	}

	// If we get here, we have a PodGroup with a set size and can inspect / update phase
	pods := podList.Items
	pgCopy := pg.DeepCopy()

	switch pgCopy.Status.Phase {
	case "":
		pgCopy.Status.Phase = schedv1alpha1.PodGroupPending
	case schedv1alpha1.PodGroupPending:
		if len(pods) >= int(pg.Spec.MinMember) {
			pgCopy.Status.Phase = schedv1alpha1.PodGroupScheduling

			// Always update owner references to be the first pod
			// E.g., if a job owns it, ensure the group is deleted with it
			updateOwnerReferences(pgCopy, &pods[0])
		}
	default:

		// Get updated counts of running, succeeded, and failed pods
		pgCopy.Status.Running, pgCopy.Status.Succeeded, pgCopy.Status.Failed = getCurrentPodStats(pods)

		// If for some reason we weren't pending and now have fewer than min required, flip back to pending
		if len(pods) < int(pg.Spec.MinMember) {
			pgCopy.Status.Phase = schedv1alpha1.PodGroupPending
			break
		}

		// A pod with succeeded + running STILL less than the minimum required is scheduling
		if pgCopy.Status.Succeeded+pgCopy.Status.Running < pg.Spec.MinMember {
			pgCopy.Status.Phase = schedv1alpha1.PodGroupScheduling
		}

		// A pod with succeeded + running >= the minimum required is running!
		if pgCopy.Status.Succeeded+pgCopy.Status.Running >= pg.Spec.MinMember {
			pgCopy.Status.Phase = schedv1alpha1.PodGroupRunning
		}

		// We have non zero failed, and the total of failed, running amd succeeded > min member
		// Final state of pod group is FAILED womp womp
		if pgCopy.Status.Failed != 0 &&
			pgCopy.Status.Failed+pgCopy.Status.Running+pgCopy.Status.Succeeded >= pg.Spec.MinMember {
			pgCopy.Status.Phase = schedv1alpha1.PodGroupFailed
		}

		// Finished! This is where we want to get :)
		// TODO: ideally the owning higher level object deletion will delete here,
		// but that won't always work for one of pods - need a new strategy
		if pgCopy.Status.Succeeded >= pg.Spec.MinMember {
			pgCopy.Status.Phase = schedv1alpha1.PodGroupFinished
		}
	}

	// TODO need better handling here of cleanup, etc. This mostly handles status changes
	return r.patchPodGroup(ctx, pg, pgCopy)
}

// newPodGroup creates a new podGroup object, capturing the creation time
// This should be followed by a request to reconsile it
func (r *PodGroupReconciler) newPodGroup(
	ctx context.Context,
	name, namespace string,
	groupSize int32,
) (*schedv1alpha1.PodGroup, error) {

	pg := &schedv1alpha1.PodGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		// Note that we don't know the size yet
		// The most important thing here is the MicroTime!
		Spec: schedv1alpha1.PodGroupSpec{
			MinMember: groupSize,
		},
		Status: schedv1alpha1.PodGroupStatus{
			ScheduleStartTime: metav1.NewMicroTime(time.Now()),
		},
	}
	// TODO need to set a controller reference?
	// ctrl.SetControllerReference(cluster, job, r.Scheme)
	err := r.Create(ctx, pg)
	if err != nil {
		r.log.Error(err, "Failed to create new PodGroup", "Namespace:", pg.Namespace, "Name:", pg.Name)
		return pg, err
	}
	// Successful - return and requeue
	return pg, nil

}

// patchPodGroup is a halper function to run a patch and then return the correct result / error for the reconciler
func (r *PodGroupReconciler) patchPodGroup(ctx context.Context, old, new *schedv1alpha1.PodGroup) (ctrl.Result, error) {
	patch := client.MergeFrom(old)
	if err := r.Status().Patch(ctx, new, patch); err != nil {
		r.log.Error(err, "Issue patching PodGroup", "Namespace:", old.Namespace, "Name:", old.Name)
		return ctrl.Result{}, err
	}
	err := r.Patch(ctx, new, patch)
	if err != nil {
		r.log.Error(err, "Issue patching PodGroup", "Namespace:", old.Namespace, "Name:", old.Name)
	}
	return ctrl.Result{}, err
}

// updatePodGroup does an update with reconcile instead of a patch request
func (r *PodGroupReconciler) updatePodGroupSize(
	ctx context.Context,
	old *schedv1alpha1.PodGroup,
	size int32,
) (ctrl.Result, error) {

	patch := client.MergeFrom(old.DeepCopy())
	old.Spec.MinMember = size

	// Apply the patch to update the size
	r.Status().Update(ctx, old)
	err := r.Patch(ctx, old, patch)
	return ctrl.Result{Requeue: true}, err
}

// getCurrentPodStats gets the number of running, succeeded, and failed
// We use these to populate the PodGroup
func getCurrentPodStats(pods []v1.Pod) (int32, int32, int32) {
	if len(pods) == 0 {
		return 0, 0, 0
	}
	var (
		running   int32 = 0
		succeeded int32 = 0
		failed    int32 = 0
	)

	// Loop and count things.
	for _, pod := range pods {
		switch pod.Status.Phase {
		case v1.PodRunning:
			running++
		case v1.PodSucceeded:
			succeeded++
		case v1.PodFailed:
			failed++
		}
	}
	return running, succeeded, failed
}

// updateOwnerReferences ensures the group is always owned by the same entity that owns the pod
// This ensures that, for example, a job that is wrapping pods is the owner.
func updateOwnerReferences(pg *schedv1alpha1.PodGroup, pod *v1.Pod) {

	// Case 1: The pod itself doesn't have owner references. YOLO
	if len(pod.OwnerReferences) == 0 {
		return
	}

	var refs []string
	for _, ownerRef := range pod.OwnerReferences {
		refs = append(refs, fmt.Sprintf("%s/%s", pod.Namespace, ownerRef.Name))
	}
	if len(refs) != 0 {
		sort.Strings(refs)
		pg.Status.OccupiedBy = strings.Join(refs, ",")
	}
}

// SetupWithManager sets up the controller with the Manager.
// We watch the events channel, which is going to trigger from the mutating webhook
// to send over when a pod group is created (hopefully preceeding schedule).
func (r *PodGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("PodGroupController")
	r.log = mgr.GetLogger()
	r.log.Info("setup with manager flux-framework/fluence-controller")

	return ctrl.NewControllerManagedBy(mgr).
		Watches(&v1.Pod{}, handler.EnqueueRequestsFromMapFunc(r.ensurePodGroup)).
		For(&schedv1alpha1.PodGroup{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.Workers}).
		Complete(r)
}

func (r *PodGroupReconciler) ensurePodGroup(ctx context.Context, obj client.Object) []ctrl.Request {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		return nil
	}
	groupName := util.GetPodGroupLabel(pod)

	// This case only happens when something is not scheduled by fluence
	if len(groupName) == 0 {
		r.log.Info("Pod: ", "Name", pod.Name, "Status", pod.Status.Phase, "Action", "Not fluence owned")
		return nil
	}

	// If we are watching the Pod and it's beyond pending, we hopefully already made a group
	// and that group should be in the reconcile process.
	if pod.Status.Phase != v1.PodPending {
		r.log.Info("Pod: ", "Name", pod.Name, "Status", pod.Status.Phase, "Action", "Skipping reconcile")
		return nil
	}

	// At this point we should have a group size (string) set by the webhook
	rawSize := pod.Labels[fluenceLabels.PodGroupSizeLabel]
	groupSize, err := strconv.ParseInt(rawSize, 10, 32)
	if err != nil {
		r.log.Error(err, "Parsing PodGroup size.")
		return nil
	}

	namespacedName := types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      groupName,
	}

	// Create the pod group if the pod is pending
	pg := &schedv1alpha1.PodGroup{}
	if err := r.Get(ctx, namespacedName, pg); err != nil {

		// Case 1: if we get here and it's not found, assume not created
		if apierrs.IsNotFound(err) {
			r.log.Info("Pod: ", "Status", pod.Status.Phase, "Name", pod.Name, "Group", groupName, "Namespace", pod.Namespace, "Action", "Creating PodGroup")

			// TODO should an owner be set here? Setting to a specific pod seems risky/wrong in case deleted.
			err, _ := r.newPodGroup(ctx, groupName, pod.Namespace, int32(groupSize))
			if err != nil {
				return []ctrl.Request{{NamespacedName: namespacedName}}
			}
			r.log.Info("Pod: ", "Status", pod.Status.Phase, "Name", pod.Name, "Group", groupName, "Namespace", pod.Namespace, "Action", "Issue Creating PodGroup")
		}
	}
	return nil
}
