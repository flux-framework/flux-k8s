/*
Copyright 2020 The Kubernetes Authors.

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

package labels

import (
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Labels to be shared between different components

const (
	// We use the same label to be consistent
	// https://github.com/kubernetes-sigs/scheduler-plugins/blob/master/apis/scheduling/v1alpha1/types.go#L109
	PodGroupLabel = "scheduling.x-k8s.io/pod-group"

	// TODO add more labels here, to be discovered used later
	//PodGroupNameLabel = "fluence.pod-group"
	PodGroupSizeLabel = "fluence.group-size"

	// Internal use (not used yet)
	PodGroupTimeCreated = "flunce.created-at"
)

// GetPodGroupLabel get pod group name from pod labels
func GetPodGroupLabel(pod *v1.Pod) string {
	return pod.Labels[PodGroupLabel]
}

// GetPodGroupFullName get namespaced group name from pod labels
func GetPodGroupFullName(pod *v1.Pod) string {
	pgName := GetPodGroupLabel(pod)
	if len(pgName) == 0 {
		return ""
	}
	return fmt.Sprintf("%v/%v", pod.Namespace, pgName)
}

// GetPodGroupSize gets the pod group size from the label
func GetPodGroupSize(pod *v1.Pod) string {
	return pod.Labels[PodGroupSizeLabel]
}

// getTimeCreated returns the timestamp when we saw the object
func GetTimeCreated() string {

	// Set the time created for a label
	createdAt := metav1.NewMicroTime(time.Now())

	// If we get an error here, the reconciler will set the time
	var timestamp string
	timeCreated, err := createdAt.MarshalJSON()
	if err == nil {
		timestamp = string(timeCreated)
	}
	return timestamp
}
