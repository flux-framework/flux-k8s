/*
Copyright 2023 Lawrence Livermore National Security, LLC

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
func NewMutatingWebhook(mgr manager.Manager) *fluenceWatcher {
	return &fluenceWatcher{decoder: admission.NewDecoder(mgr.GetScheme())}
}

// mutate-v1-fluence
type fluenceWatcher struct {
	decoder *admission.Decoder
}

// Handle is the main handler for the webhook, which is looking for jobs and pods (in that order)
// If a job comes in (with a pod template) first, we add the labels there first (and they will
// not be added again).
func (a *fluenceWatcher) Handle(ctx context.Context, req admission.Request) admission.Response {

	logger.Info("Running webhook handle")

	// Try for a job first, which would be created before pods
	job := &batchv1.Job{}
	err := a.decoder.Decode(req, job)
	if err != nil {

		// Assume we operate on the level of pods for now
		pod := &corev1.Pod{}
		err := a.decoder.Decode(req, pod)
		if err != nil {
			logger.Error(err, "Admission error.")
			return admission.Errored(http.StatusBadRequest, err)
		}

		// If we get here, we decoded a pod
		err = a.EnsureGroup(pod)
		if err != nil {
			logger.Error(err, "Issue adding PodGroup to pod.")
			return admission.Errored(http.StatusBadRequest, err)
		}

		logger.Info("Admission pod success.")

		marshalledPod, err := json.Marshal(pod)
		if err != nil {
			logger.Error(err, "Marshalling pod error.")
			return admission.Errored(http.StatusInternalServerError, err)
		}

		logger.Info("Admission job success.")
		return admission.PatchResponseFromRaw(req.Object.Raw, marshalledPod)
	}

	// If we get here, err was nil and we have a Job!
	err = a.EnsureGroupOnJob(job)
	if err != nil {
		logger.Error(err, "Issue adding PodGroup to job.")
		return admission.Errored(http.StatusBadRequest, err)
	}

	logger.Info("Admission job success.")
	marshalledJob, err := json.Marshal(job)
	if err != nil {
		logger.Error(err, "Marshalling job error.")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	logger.Info("Admission job success.")
	return admission.PatchResponseFromRaw(req.Object.Raw, marshalledJob)
}

// Default is the expected entrypoint for a webhook...
// I don't remember if this is even called...
func (a *fluenceWatcher) Default(ctx context.Context, obj runtime.Object) error {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("expected a Pod or Job but got a %T", obj)
	}
	logger.Info(fmt.Sprintf("Pod %s is marked for fluence.", pod.Name))
	return a.EnsureGroup(pod)
}

// EnsureGroup adds pod group label and size if not present
// This ensures that every pod passing through is part of a group.
// Note that we need to do similar for Job.
// A pod without a job wrapper, and without metadata is a group
// of size 1.
func (a *fluenceWatcher) EnsureGroup(pod *corev1.Pod) error {

	// Add labels if we don't have anything. Everything is a group!
	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}

	// Do we have a group name?
	groupName, ok := pod.Labels[labels.PodGroupNameLabel]

	// If we don't have a fluence group, create one under fluence namespace
	if !ok {
		groupName = fmt.Sprintf("fluence-group-%s-%s", pod.Namespace, pod.Name)
		pod.Labels[labels.PodGroupNameLabel] = groupName
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
// Since we have the size of the job (paramllism) we can use that for the size
func (a *fluenceWatcher) EnsureGroupOnJob(job *batchv1.Job) error {

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
	groupName := getJobLabel(job, labels.PodGroupNameLabel, defaultName)

	// Wherever we find it, make sure the pod group name is on the pod spec template
	job.Spec.Template.ObjectMeta.Labels[labels.PodGroupNameLabel] = groupName

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
