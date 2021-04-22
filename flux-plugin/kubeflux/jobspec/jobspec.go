/*
Copyright © 2021 IBM Corporation

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
package jobspec

import (
	"fmt"
	"log"
	"math"
	"os"

	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
)

type PodRequest struct {
	ID string
	// Pod        *v1.Pod
	Containers []v1.Container
	CPU        []int64
	Memory     []int64
	Gpu        []int64
	Storage    []int64
}

func InspectPodInfo(pod *v1.Pod) *PodRequest {
	pr := new(PodRequest)
	pr.ID = pod.Name
	// pr.Pod = pod
	pr.Containers = pod.Spec.Containers
	numcontainers := len(pr.Containers)
	pr.CPU = make([]int64, numcontainers)
	pr.Memory = make([]int64, numcontainers)
	pr.Gpu = make([]int64, numcontainers)
	pr.Storage = make([]int64, numcontainers)

	for i, cont := range pr.Containers {
		// fmt.Printf("[FML] \n%v\n", cont)
		specRequests := cont.Resources.Requests
		specLimits := cont.Resources.Limits

		if specRequests.Cpu().Value() == 0 {
			pr.CPU[i] = 1
		} else {
			pr.CPU[i] = specRequests.Cpu().Value()
		}
		if specRequests.Memory().Value() == 0 {
			pr.Memory[i] = 10000
		} else {
			pr.Memory[i] = specRequests.Memory().Value()
		}
		gpu := specLimits["nvidia.com/gpu"]
		pr.Gpu[i] = gpu.Value()
		pr.Storage[i] = specRequests.StorageEphemeral().Value()
		//for key := range pod.Spec.Containers[0].Resources.Requests {
		//	fmt.Printf("Requests key %v %p\n", key, key)
		//}

		fmt.Printf("[Jobspec] Pod spec: CPU %v/%v-milli, memory %v/%v-milli, GPU %v, storage %v\n", pr.CPU[i], specRequests.Cpu().MilliValue(),
			pr.Memory[i], specRequests.Memory().MilliValue(), pr.Gpu[i], pr.Storage[i])
	}

	fmt.Println("[Jobspec] Node selector: ", pod.Spec.NodeSelector)
	return pr
}

/*
Ps: &pb.PodSpec{
			Id:        pod_jobspec.ID,
			Container: pod_jobspec.Containers[0].Image,
			MilliCPU:  pod_jobspec.MilliCPU[0],
			Memory:    pod_jobspec.Memory[0],
			Gpu:       pod_jobspec.Gpu[0],
			Storage:   pod_jobspec.Storage[0],
		},
*/

func CreateJobSpecYaml(pr *PodRequest) (filename string) {
	for i := 0; i < len(pr.Containers); i++ {
		socket_resources := make([]Resource, 2)
		command := pr.Containers[i].Command
		fmt.Printf("[JobSpec] Required memory %v/%v\n", pr.Memory[i], toGB(pr.Memory[i]))
		socket_resources[0] = Resource{Type: "core", Count: pr.CPU[i]}
		socket_resources[1] = Resource{Type: "memory", Count: pr.Memory[i]}
		if pr.Gpu[i] > 0 {
			socket_resources = append(socket_resources, Resource{Type: "gpu", Count: pr.Gpu[i]})
		}

		js := JobSpec{
			Version: Version{
				Version: 1,
				Resources: []Resource{
					{
						Type:  "node",
						Count: 1,
						With: []Resource{
							{
								Type:  "socket",
								Count: 1,
								With: []Resource{
									{
										Type:  "slot",
										Count: 1,
										Label: "default",
										With:  socket_resources,
									},
								},
							},
						},
					},
				},
			},
			Attributes: Attribute{
				System{
					Duration: 3600,
				},
			},
			Tasks: []Task{
				{
					Command: command,
					Slot:    "default",
					Counts: Count{
						PerSlot: 1,
					},
				},
			},
		}
		yamlbytes, err := yaml.Marshal(&js)
		if err != nil {
			log.Fatalf("[JobSpec] yaml.Marshal failed with '%s'\n", err)
		}
		fmt.Printf("[JobSpec] JobSpec in YAML:\n%s\n", string(yamlbytes))
		filename = "yamlexample.yaml"
		f, err := os.Create(filename)
		if err != nil {
			log.Fatalf("[JobSpec] Couldn't create yaml file!!\n")
		}
		defer f.Close()

		_, err = f.Write(yamlbytes)
		if err != nil {
			log.Fatalf("[JobSpec] Couldn't write yaml file!!\n")
		}

		_, err = f.WriteString("\n")
		if err != nil {
			log.Fatalf("[JobSpec] Couldn't write yaml file!!\n")
		}
	}
	return filename
}

func toGB(bytes int64) int64 {
	res := float64(bytes) / math.Pow(10, 9)
	return int64(res)
}
