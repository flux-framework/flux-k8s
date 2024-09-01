/*
Copyright 2024 Lawrence Livermore National Security, LLC

(c.f. AUTHORS, NOTICE.LLNS, COPYING)
SPDX-License-Identifier: MIT
*/

// This file is not used, but maintained as the original addition of an OrasCache webhook

package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/scheduler-plugins/pkg/fluence/labels"
)

var (
	logger = ctrl.Log.WithName("setup")
)

// IMPORTANT: if you use the controller-runtime builder, it will derive this name automatically from the gvk (kind, version, etc. so find the actual created path in the logs)
// kubectl describe mutatingwebhookconfigurations.admissionregistration.k8s.io
// It will also only allow you to describe one object type with For()
// This is disabled so we manually manage it - multiple types to a list did not work: config/webhook/manifests.yaml
////kubebuilder:webhook:path=/mutate-v1-sidecar,mutating=true,failurePolicy=fail,sideEffects=None,groups=core;batch,resources=pods;jobs,verbs=create,versions=v1,name=morascache.kb.io,admissionReviewVersions=v1

// NewMutatingWebhook allows us to keep the sidecarInjector private
// If it's public it's exported and kubebuilder tries to add to zz_generated_deepcopy
// and you get all kinds of terrible errors about admission.Decoder missing DeepCopyInto
func NewMutatingWebhook(mgr manager.Manager) fluenceWatcher {
	return fluenceWatcher{decoder: admission.NewDecoder(mgr.GetScheme())}
}

// mutate-v1-fluence
type fluenceWatcher struct {
	decoder admission.Decoder
}

// Handle is the main handler for the webhook, which is looking for jobs and pods (in that order)
// If a job comes in (with a pod template) first, we add the labels there first (and they will
// not be added again).
func (hook fluenceWatcher) Handle(ctx context.Context, req admission.Request) admission.Response {

	logger.Info("Running webhook handle, determining pod wrapper abstraction...")

	job := &batchv1.Job{}
	err := hook.decoder.Decode(req, job)
	if err == nil {
		err = hook.EnsureGroupOnJob(job)
		if err != nil {
			logger.Error(err, "Issue adding PodGroup to Job")
			return admission.Errored(http.StatusBadRequest, err)
		}
		marshalledJob, err := json.Marshal(job)
		if err != nil {
			logger.Error(err, "Marshalling job error.")
			return admission.Errored(http.StatusInternalServerError, err)
		}
		logger.Info("Admission job success.")
		return admission.PatchResponseFromRaw(req.Object.Raw, marshalledJob)
	}

	pod := &corev1.Pod{}
	err = hook.decoder.Decode(req, pod)
	if err == nil {
		err = hook.EnsureGroup(pod)
		if err != nil {
			logger.Error(err, "Issue adding PodGroup to Pod")
			return admission.Errored(http.StatusBadRequest, err)
		}
		marshalledPod, err := json.Marshal(pod)
		if err != nil {
			logger.Error(err, "Marshalling pod error")
			return admission.Errored(http.StatusInternalServerError, err)
		}
		logger.Info("Admission pod success")
		return admission.PatchResponseFromRaw(req.Object.Raw, marshalledPod)
	}

	set := &appsv1.StatefulSet{}
	err = hook.decoder.Decode(req, set)
	if err == nil {
		err = hook.EnsureGroupStatefulSet(set)
		if err != nil {
			logger.Error(err, "Issue adding PodGroup to StatefulSet")
			return admission.Errored(http.StatusBadRequest, err)
		}
		marshalledSet, err := json.Marshal(set)
		if err != nil {
			logger.Error(err, "Marshalling StatefulSet error")
			return admission.Errored(http.StatusInternalServerError, err)
		}
		logger.Info("Admission StatefulSet success")
		return admission.PatchResponseFromRaw(req.Object.Raw, marshalledSet)
	}

	deployment := &appsv1.Deployment{}
	err = hook.decoder.Decode(req, deployment)
	if err == nil {
		err = hook.EnsureGroupDeployment(deployment)
		if err != nil {
			logger.Error(err, "Issue adding PodGroup to Deployment")
			return admission.Errored(http.StatusBadRequest, err)
		}
		marshalledD, err := json.Marshal(deployment)
		if err != nil {
			logger.Error(err, "Marshalling Deployment error")
			return admission.Errored(http.StatusInternalServerError, err)
		}
		logger.Info("Admission Deployment success")
		return admission.PatchResponseFromRaw(req.Object.Raw, marshalledD)
	}

	rset := &appsv1.ReplicaSet{}
	err = hook.decoder.Decode(req, rset)
	if err == nil {
		err = hook.EnsureGroupReplicaSet(rset)
		if err != nil {
			logger.Error(err, "Issue adding PodGroup to ReplicaSet")
			return admission.Errored(http.StatusBadRequest, err)
		}
		marshalledSet, err := json.Marshal(rset)
		if err != nil {
			logger.Error(err, "Marshalling StatefulSet error")
			return admission.Errored(http.StatusInternalServerError, err)
		}
		logger.Info("Admission StatefulSet success")
		return admission.PatchResponseFromRaw(req.Object.Raw, marshalledSet)
	}

	// We should not get down here
	return admission.Allowed("Object not known, this webhook does not validate beyond those.")

}

// Default is the expected entrypoint for a webhook...
func (hook fluenceWatcher) Default(ctx context.Context, obj runtime.Object) error {

	switch obj.(type) {
	case *batchv1.Job:
		job := obj.(*batchv1.Job)
		return hook.EnsureGroupOnJob(job)

	case *corev1.Pod:
		pod := obj.(*corev1.Pod)
		return hook.EnsureGroup(pod)

	case *appsv1.StatefulSet:
		set := obj.(*appsv1.StatefulSet)
		return hook.EnsureGroupStatefulSet(set)

	case *appsv1.Deployment:
		deployment := obj.(*appsv1.Deployment)
		return hook.EnsureGroupDeployment(deployment)

	case *appsv1.ReplicaSet:
		set := obj.(*appsv1.ReplicaSet)
		return hook.EnsureGroupReplicaSet(set)

	default:
		// no match
	}
	return nil
}

// EnsureGroup adds pod group label and size if not present
// This ensures that every pod passing through is part of a group.
// Note that we need to do similar for Job.
// A pod without a job wrapper, and without metadata is a group
// of size 1.
func (hook fluenceWatcher) EnsureGroup(pod *corev1.Pod) error {

	// Add labels if we don't have anything. Everything is a group!
	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}

	// Do we have a group name?
	groupName, ok := pod.Labels[labels.PodGroupLabel]

	// If we don't have a fluence group, create one under fluence namespace
	if !ok {
		groupName = fmt.Sprintf("fluence-group-%s", pod.Name)
		pod.Labels[labels.PodGroupLabel] = groupName
	}

	// Do we have a group size? This will be parsed as a string, likely
	groupSize, ok := pod.Labels[labels.PodGroupSizeLabel]
	if !ok {
		groupSize = "1"
		pod.Labels[labels.PodGroupSizeLabel] = groupSize
	}
	return nil
}

// getJobLabel takes a label name and default and returns the value
// We look on both the job and underlying pod spec template
func getJobLabel(job *batchv1.Job, labelName, defaultLabel string) string {

	value, ok := job.Labels[labelName]
	if !ok {
		value, ok = job.Spec.Template.ObjectMeta.Labels[labelName]
		if !ok {
			value = defaultLabel
		}
	}
	return value
}

// EnsureGroupOnJob looks for fluence labels (size and name) on both the job
// and the pod template. We ultimately put on the pod, the lowest level unit.
// Since we have the size of the job (parallelism) we can use that for the size
func (a fluenceWatcher) EnsureGroupOnJob(job *batchv1.Job) error {

	// Be forgiving - allow the person to specify it on the job directly or on the Podtemplate
	// We will ultimately put the metadata on the Pod.
	if job.Spec.Template.ObjectMeta.Labels == nil {
		job.Spec.Template.ObjectMeta.Labels = map[string]string{}
	}
	if job.Labels == nil {
		job.Labels = map[string]string{}
	}

	/// First get the name for the pod group (also setting on the pod template)
	defaultName := fmt.Sprintf("fluence-group-%s-%s", job.Namespace, job.Name)
	groupName := getJobLabel(job, labels.PodGroupLabel, defaultName)

	// Wherever we find it, make sure the pod group name is on the pod spec template
	job.Spec.Template.ObjectMeta.Labels[labels.PodGroupLabel] = groupName

	// Now do the same for the size, but the size is the size of the job
	jobSize := *job.Spec.Parallelism
	if jobSize == int32(0) {
		jobSize = int32(1)
	}
	labelSize := fmt.Sprintf("%d", jobSize)
	groupSize := getJobLabel(job, labels.PodGroupSizeLabel, labelSize)
	job.Spec.Template.ObjectMeta.Labels[labels.PodGroupSizeLabel] = groupSize
	return nil
}

// EnsureGroupStatefulSet creates a PodGroup for a StatefulSet
func (hook fluenceWatcher) EnsureGroupStatefulSet(set *appsv1.StatefulSet) error {

	// StatefulSet requires on top level explicitly
	if set.Labels == nil {
		set.Labels = map[string]string{}
	}
	defaultName := fmt.Sprintf("fluence-group-%s-%s", set.Namespace, set.Name)
	groupName, ok := set.Labels[labels.PodGroupLabel]
	if !ok {
		groupName = defaultName
	}
	set.Spec.Template.ObjectMeta.Labels[labels.PodGroupLabel] = groupName

	// Now do the same for the size, but the size is the size of the job
	size := *set.Spec.Replicas
	if size == int32(0) {
		size = int32(1)
	}
	labelSize := fmt.Sprintf("%d", size)
	groupSize, ok := set.Labels[labels.PodGroupSizeLabel]
	if !ok {
		groupSize = labelSize
	}
	set.Spec.Template.ObjectMeta.Labels[labels.PodGroupSizeLabel] = groupSize
	return nil
}

// EnsureGroupStatefulSet creates a PodGroup for a StatefulSet
func (a fluenceWatcher) EnsureGroupReplicaSet(set *appsv1.ReplicaSet) error {

	// StatefulSet requires on top level explicitly
	if set.Labels == nil {
		set.Labels = map[string]string{}
	}
	defaultName := fmt.Sprintf("fluence-group-%s-%s", set.Namespace, set.Name)
	groupName, ok := set.Labels[labels.PodGroupLabel]
	if !ok {
		groupName = defaultName
	}
	set.Spec.Template.ObjectMeta.Labels[labels.PodGroupLabel] = groupName

	// Now do the same for the size, but the size is the size of the job
	size := *set.Spec.Replicas
	if size == int32(0) {
		size = int32(1)
	}
	labelSize := fmt.Sprintf("%d", size)
	groupSize, ok := set.Labels[labels.PodGroupSizeLabel]
	if !ok {
		groupSize = labelSize
	}
	set.Spec.Template.ObjectMeta.Labels[labels.PodGroupSizeLabel] = groupSize
	return nil
}

// EnsureGroupDeployment creates a PodGroup for a Deployment
// This is redundant, can refactor later
func (a fluenceWatcher) EnsureGroupDeployment(d *appsv1.Deployment) error {

	// StatefulSet requires on top level explicitly
	if d.Labels == nil {
		d.Labels = map[string]string{}
	}
	defaultName := fmt.Sprintf("fluence-group-%s-%s", d.Namespace, d.Name)
	groupName, ok := d.Labels[labels.PodGroupLabel]
	if !ok {
		groupName = defaultName
	}
	d.Spec.Template.ObjectMeta.Labels[labels.PodGroupLabel] = groupName

	// Now do the same for the size, but the size is the size of the job
	size := *d.Spec.Replicas
	if size == int32(0) {
		size = int32(1)
	}
	labelSize := fmt.Sprintf("%d", size)
	groupSize, ok := d.Labels[labels.PodGroupSizeLabel]
	if !ok {
		groupSize = labelSize
	}
	d.Spec.Template.ObjectMeta.Labels[labels.PodGroupSizeLabel] = groupSize
	return nil
}
