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

Structure of the PodSpec that needs to be generated, for reference
Ps: &pb.PodSpec{
			Id:        pod_jobspec.ID,
			Container: pod_jobspec.Containers[0].Image,
			MilliCPU:  pod_jobspec.MilliCPU[0],
			Memory:    pod_jobspec.Memory[0],
			Gpu:       pod_jobspec.Gpu[0],
			Storage:   pod_jobspec.Storage[0],
		},
*/

// CreateJobSpecYaml writes the protobuf jobspec into a yaml file
func CreateJobSpecYaml(spec *pb.PodSpec, count int32, filename string) error {

	command := []string{spec.Container}
	fmt.Println("Labels ", spec.Labels, " ", len(spec.Labels))

	js := JobSpec{
		Version:    Version{Version: 9999},
		Attributes: Attribute{System{Duration: 3600}},

		// The name of the task likely needs to correspond with the pod
		// Since we can't easily change the proto file, for now it is
		// storing the pod namespaced name.
		Tasks: []Task{
			{
				Command: command,
				Slot:    "default",
				Counts:  Count{PerSlot: 1},
			},
		},
	}

	// Assemble resources!
	socketResources := createSocketResources(spec)
	js.Version.Resources = createResources(spec, socketResources, count)

	// Write bytes to file
	yamlbytes, err := yaml.Marshal(&js)
	if err != nil {
		log.Fatalf("[JobSpec] yaml.Marshal failed with '%s'\n", err)
		return err
	}
	return writeBytes(yamlbytes, filename)
}

// WriteBytes writes a byte string to file
func writeBytes(bytelist []byte, filename string) error {
	fmt.Printf("[JobSpec] Preparing to write:\n%s\n", string(bytelist))
	f, err := os.Create(filename)
	if err != nil {
		log.Fatalf("[JobSpec] Couldn't create file!!\n")
		return err
	}
	defer f.Close()

	_, err = f.Write(bytelist)
	if err != nil {
		log.Fatalf("[JobSpec] Couldn't write file!!\n")
		return err
	}

	// Not sure why this is here, but will keep for now
	_, err = f.WriteString("\n")
	if err != nil {
		log.Fatalf("[JobSpec] Couldn't append newline to file!!\n")
	}
	return err
}

func toGB(bytes int64) int64 {
	res := float64(bytes) / math.Pow(10, 9)
	return int64(res)
}

// createSocketResources creates the socket resources for the JobSpec
func createSocketResources(spec *pb.PodSpec) []Resource {

	socketResources := []Resource{
		{
			Type: "core", Count: int64(spec.Cpu),
		},
	}

	// TODO double check what we are converting from -> to
	if spec.Memory > 0 {
		toMB := spec.Memory >> 20
		socketResources = append(socketResources, Resource{Type: "memory", Count: toMB})
	}

	if spec.Gpu > 0 {
		socketResources = append(socketResources, Resource{Type: "gpu", Count: spec.Gpu})
	}
	return socketResources
}

// createResources assembles the list of JobSpec resources
func createResources(spec *pb.PodSpec, socketResources []Resource, count int32) []Resource {

	slotResource := []Resource{
		{
			Type:  "slot",
			Count: int64(count),
			Label: "default",
			With:  socketResources,
		},
	}

	// Presence of the zone label means we need to add a subnet
	if len(spec.Labels) > 0 {
		for _, label := range spec.Labels {
			if label == "zone" {
				nodeResource := []Resource{
					{
						Type:  "subnet",
						Count: 1,
						With: []Resource{
							{
								Type:  "node",
								Count: 1,
								With:  slotResource,
							},
						},
					},
				}
				return nodeResource
			}
		}
	}
	return slotResource
}
