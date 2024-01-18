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

package utils

import (
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	pb "sigs.k8s.io/scheduler-plugins/pkg/fluence/fluxcli-grpc"
)

// TODO this package should be renamed something related to a PodSpec Info

// getPodJobpsecLabels looks across labels and returns those relevant
// to a jobspec
func getPodJobspecLabels(pod *v1.Pod) []string {
	labels := []string{}
	for label, value := range pod.Labels {
		if strings.Contains(label, "jobspec") {
			labels = append(labels, value)
		}
	}
	return labels
}

// InspectPodInfo takes a pod object and returns the pod.spec
// Note from vsoch - I updated this to calculate containers across the pod
// if that's wrong we can change it back.
func InspectPodInfo(pod *v1.Pod) *pb.PodSpec {
	ps := new(pb.PodSpec)
	ps.Id = pod.Name

	// Note from vsoch - there was an if check here to see if we had labels,
	// I don't think there is risk to adding an empty list but we can add
	// the check back if there is
	ps.Labels = getPodJobspecLabels(pod)

	// Note that Container gets use for the JobSpec, so we provide
	// the pod name (to be associated with tasks) for it. We are making
	// the assumption that this one container represents the group,
	// which is OK for now, but might not always be true!
	ps.Container = fmt.Sprintf("%s-%s", pod.Namespace, pod.Name)

	// Create accumulated requests for cpu and limits
	// CPU and memory are summed across containers
	// GPU cannot be shared across containers, but we
	// take a count for the pod for the PodSpec
	var cpus int32 = 0
	var memory int64 = 0
	var gpus int64 = 0

	// I think we are OK to sum this too
	// https://github.com/kubernetes/kubectl/blob/master/pkg/describe/describe.go#L4211-L4213
	var storage int64 = 0

	for _, container := range pod.Spec.Containers {

		// Add on Cpu, Memory, GPU from container requests
		// This is a limited set of resources owned by the pod
		specRequests := container.Resources.Requests
		cpus += int32(specRequests.Cpu().Value())
		memory += specRequests.Memory().Value()
		storage += specRequests.StorageEphemeral().Value()

		specLimits := container.Resources.Limits
		gpuSpec := specLimits["nvidia.com/gpu"]
		gpus += gpuSpec.Value()

	}

	// If we have zero cpus, assume 1
	// We could use math.Max here, but it is expecting float64
	if cpus == 0 {
		cpus = 1
	}
	ps.Cpu = cpus
	ps.Gpu = gpus
	ps.Memory = memory
	ps.Storage = storage

	// I removed specRequests.Cpu().MilliValue() but we can add back some derivative if desired
	klog.Infof("[Jobspec] Pod spec: CPU %v, memory %v, GPU %v, storage %v", ps.Cpu, ps.Memory, ps.Gpu, ps.Storage)
	return ps
}
