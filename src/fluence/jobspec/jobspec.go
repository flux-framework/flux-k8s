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

	pb "github.com/flux-framework/flux-k8s/flux-plugin/fluence/fluxcli-grpc"
	"gopkg.in/yaml.v2"
)



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

func CreateJobSpecYaml(pr *pb.PodSpec, count int32, filename string) error {
		socket_resources := make([]Resource, 1)
		command := []string{pr.Container}
		socket_resources[0] = Resource{Type: "core", Count: int64(pr.Cpu)}
		if pr.Memory > 0 {
			toMB := pr.Memory >> 20
			socket_resources = append(socket_resources, Resource{Type: "memory", Count: toMB})
		}

		if pr.Gpu > 0 {
			socket_resources = append(socket_resources, Resource{Type: "gpu", Count: pr.Gpu})
		}

		fmt.Println("Labels ", pr.Labels, " ", len(pr.Labels))

		js := JobSpec{
			Version: Version{
				Version: 9999,
			},
			Attributes: Attribute{
				System{
					Duration: 3600,
				},
			},
			Tasks: []Task{
				{
					// Command: "[\""+command+"\"]",
					Command: command,
					Slot:    "default",
					Counts: Count{
						PerSlot: 1,
					},
				},
			},
		}

		slot_resource := make([]Resource, 1)
		slot_resource[0] = Resource{
			Type: "slot",
			Count: int64(count),
			Label: "default",
			With: socket_resources,
		}

		if len(pr.Labels) > 0 {
			for _, label := range pr.Labels {
				if label == "zone" {
					node_resource := make([]Resource, 1)
					node_resource[0] = Resource{
						Type: "subnet", 
						Count: 1,
						With: []Resource{
							{
								Type: "node", 
								Count: 1,
								With: slot_resource, /*[]Resource{
									{
									Type: "socket", 
									Count: 1,
									With: slot_resource,
									},
								},*/
							},
						},
					}
					js.Version.Resources = node_resource
				}
				
			}
		
		}  else {
			fmt.Println("No labels, going with plain JobSpec")
			js.Version.Resources = slot_resource
		}
		
		// js := JobSpec{
		// 	Version: Version{
		// 		Version: 9999,
		// 		Resources: []Resource{
		// 			{
		// 				Type:  "node",
		// 				Count: 1,
		// 				With: []Resource{
		// 					{
		// 						Type:  "socket",
		// 						Count: 1,
		// 						With: []Resource{
		// 							{
		// 								Type:  "slot",
		// 								Count: int64(count),
		// 								Label: "default",
		// 								With:  socket_resources,
		// 							},
		// 						},
		// 					},
		// 				},
		// 			},
		// 		},
		// 	},
		// 	Attributes: Attribute{
		// 		System{
		// 			Duration: 3600,
		// 		},
		// 	},
		// 	Tasks: []Task{
		// 		{
		// 			// Command: "[\""+command+"\"]",
		// 			Command: command,
		// 			Slot:    "default",
		// 			Counts: Count{
		// 				PerSlot: 1,
		// 			},
		// 		},
		// 	},
		// }
		yamlbytes, err := yaml.Marshal(&js)
		if err != nil {
			log.Fatalf("[JobSpec] yaml.Marshal failed with '%s'\n", err)
			return err
		}
		fmt.Printf("[JobSpec] JobSpec in YAML:\n%s\n", string(yamlbytes))
		f, err := os.Create(filename)
		if err != nil {
			log.Fatalf("[JobSpec] Couldn't create yaml file!!\n")
			return err
		}
		defer f.Close()

		_, err = f.Write(yamlbytes)
		if err != nil {
			log.Fatalf("[JobSpec] Couldn't write yaml file!!\n")
			return err
		}

		_, err = f.WriteString("\n")
		if err != nil {
			log.Fatalf("[JobSpec] Couldn't write yaml file!!\n")
			return err
		}
	return nil
}

func toGB(bytes int64) int64 {
	res := float64(bytes) / math.Pow(10, 9)
	return int64(res)
}
