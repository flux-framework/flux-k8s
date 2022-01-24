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

package main

import (
	"flag"
	"fmt"
	"jgf"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	fmt.Println("Testing jgf graph creation")
	var fluxgraph jgf.Fluxjgf
	fluxgraph = jgf.InitJGF()

	cluster := fluxgraph.MakeCluster("k8scluster")
	rack := fluxgraph.MakeRack(0)
	fluxgraph.MakeEdge(cluster, rack, "contains")
	fluxgraph.MakeEdge(rack, cluster, "in")

	// Connect to kubernetes cluster
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	nodes, err := clientset.CoreV1().Nodes().List(metav1.ListOptions{})

	fmt.Println("Number worker nodes ", len(nodes.Items))
	for node_index, node := range nodes.Items {
		fmt.Println("Node spec\n", node.Labels)
		freecpu, _ := node.Status.Allocatable.Cpu().AsInt64()
		fmt.Println("CPU avail ", freecpu)
		totalcpu, _ := node.Status.Capacity.Cpu().AsInt64()
		fmt.Println("CPU capacity ", totalcpu)
		freemem, _ := node.Status.Allocatable.Memory().AsInt64()
		fmt.Println("Memory avail ", freemem)
		totalmem, _ := node.Status.Capacity.Memory().AsInt64()
		fmt.Println("Memory capacity ", totalmem)
		freestorage, _ := node.Status.Allocatable.StorageEphemeral().AsInt64()
		fmt.Println("Storage avail ", freestorage)
		totalstorage, _ := node.Status.Capacity.StorageEphemeral().AsInt64()
		fmt.Println("Storage capacity ", totalstorage)

		workernode := fluxgraph.MakeNode(node_index, false, node.Name)
		fluxgraph.MakeEdge(rack, workernode, "contains")
		fluxgraph.MakeEdge(workernode, rack, "in")

		socket := fluxgraph.MakeSocket(0, "socket")
		fluxgraph.MakeEdge(workernode, socket, "contains")
		fluxgraph.MakeEdge(socket, workernode, "in")

		for index := 0; index < int(totalcpu); index++ {
			// MakeCore(index int, name string)
			core := fluxgraph.MakeCore(index, "core", &node.Labels)
			fluxgraph.MakeEdge(socket, core, "contains")
			fluxgraph.MakeEdge(core, socket, "in")
		}

		//  MakeMemory(index int, name string, unit string, size int
		mem := fluxgraph.MakeMemory(0, "memory", "KB", int(totalmem))
		fluxgraph.MakeEdge(socket, mem, "contains")
		fluxgraph.MakeEdge(mem, socket, "in")
	}

	fluxgraph.WriteJGF("testgraph.json")
}
