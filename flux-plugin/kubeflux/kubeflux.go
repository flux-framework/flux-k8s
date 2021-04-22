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

package kubeflux

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"k8s.io/klog/v2"
	"time"

	"fluxcli"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"sigs.k8s.io/scheduler-plugins/pkg/kubeflux/jgf"
	"sigs.k8s.io/scheduler-plugins/pkg/kubeflux/jobspec"
)

type KubeFlux struct {
	handle  framework.Handle
	fluxctx *fluxcli.ReapiCtx
}

var _ framework.ReservePlugin = &KubeFlux{}
var _ framework.PreFilterPlugin = &KubeFlux{}
var _ framework.PostFilterPlugin = &KubeFlux{}

var _ framework.PermitPlugin = &KubeFlux{}

// let's give it a name
const (
	Name = "KubeFlux"
)

func (kf *KubeFlux) Name() string {
	return Name
}

func (kf *KubeFlux) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) *framework.Status {
	klog.Infof("Examining the pod")

	fluxjbs := jobspec.InspectPodInfo(pod)
	filename := "/home/data/jobspecs/jobspec.yaml"
	jobspec.CreateJobSpecYaml(fluxjbs, filename)

	/*spec, err := ioutil.ReadFile(filename)
	if err != nil {
		err := fmt.Errorf("Error reading jobspec file")
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}

	reserved, allocated, at, overhead, jobid, fluxerr := fluxcli.ReapiCliMatchAllocate(kf.fluxctx, false, string(spec))
	if fluxerr != 0 {
		err := fmt.Errorf("Error in ReapiCliMatchAllocate")
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}
	printOutput(reserved, allocated, at, overhead, jobid, fluxerr)*/

	err := fmt.Errorf("Sending the pod to post filter phase")
	return framework.NewStatus(framework.Unschedulable, err.Error())

	//return framework.NewStatus(framework.Success, "")
}

func (kf *KubeFlux) PostFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, m framework.NodeToStatusMap) (*framework.PostFilterResult, *framework.Status) {
	nodename, err := kf.askFlux(ctx, pod)
	if err != nil {
		return &framework.PostFilterResult{}, framework.NewStatus(framework.Unschedulable,
			fmt.Sprintf("Pod %v is unschedulable by Flux", pod.Name))

	}
	fmt.Println("Node Selected: ", nodename)
	return &framework.PostFilterResult{NominatedNodeName: nodename}, framework.NewStatus(framework.Success)
}

func (kf *KubeFlux) askFlux(ctx context.Context, pod *v1.Pod) (string, error) {

	filename := "/home/data/jobspecs/jobspec.yaml"
	spec, err := ioutil.ReadFile(filename)
	if err != nil {
		// err := fmt.Errorf("Error reading jobspec file")
		return "", errors.New("Error reading jobspec")
	}

	reserved, allocated, at, overhead, jobid, fluxerr := fluxcli.ReapiCliMatchAllocate(kf.fluxctx, false, string(spec))
	if fluxerr != 0 {
		// err := fmt.Errorf("Error in ReapiCliMatchAllocate")
		return "", errors.New("Error in ReapiCliMatchAllocate")
	}
	printOutput(reserved, allocated, at, overhead, jobid, fluxerr)
	nodename := fluxcli.ReapiCliGetNode(kf.fluxctx)
	fmt.Println("nodename ", nodename)

	// waitingPod := kf.handle.GetWaitingPod(pod.UID)

	// waitingPod.Allow(kf.Name())

	return nodename, nil
}

func (kf *KubeFlux) Permit(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (*framework.Status, time.Duration) {

	kf.handle.IterateOverWaitingPods(func(waitingPod framework.WaitingPod) {
		fmt.Println("Permit allows the pod ", waitingPod.GetPod())
		waitingPod.Allow(kf.Name())
	})
	fmt.Println("Permit allows the pod: %v")
	return framework.NewStatus(framework.Success, ""), 0
}

func (kf *KubeFlux) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func (kf *KubeFlux) Reserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	fmt.Println("Reserving pod ", pod.Name)

	return framework.NewStatus(framework.Success, "")
}
func (kf *KubeFlux) Unreserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) {
	fmt.Println("Unreserve pod ", pod.Name)
}

// initialize and return a new Flux Plugin
func New(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	fctx := fluxcli.NewReapiCli()
	fmt.Println("Created cli context ", fctx)
	filename := "/home/data/jgf/singlenode.json"
	err := createJGF(handle, filename)
	if err != nil {
		return nil, err
	}

	jgf, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Println("Error reading JGF")
		return nil, err
	}
	// fmt.Println("JGF ", string(jgf))
	ret := fluxcli.ReapiCliInit(fctx, string(jgf))
	if ret != 0 {
		fmt.Println("Error while initializing ReapiCli")
		return nil, errors.New("Error while initializing ReapiCli")
	}
	klog.Infof("KubeFlux starts")

	// jobspec, err := ioutil.ReadFile("/home/data/jobspecs/yamlexample.yaml")
	// if err != nil {
	// 	fmt.Println("Error reading jobspec")
	// 	return nil, err
	// }

	return &KubeFlux{handle: handle, fluxctx: fctx}, nil
}

////// Utility functions
func printOutput(reserved bool, allocated string, at int64, overhead float64, jobid uint64, fluxerr int) {
	fmt.Println("\n\t----Match Allocate output---")
	fmt.Printf("jobid: %d\nreserved: %t\nallocated: %s\nat: %d\noverhead: %f\nerror: %d\n", jobid, reserved, allocated, at, overhead, fluxerr)
}

func createJGF(handle framework.Handle, filename string) error {
	ctx := context.Background()
	clientset := handle.ClientSet()
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	var fluxgraph jgf.Fluxjgf
	fluxgraph = jgf.InitJGF()

	cluster := fluxgraph.MakeCluster("k8scluster")
	rack := fluxgraph.MakeRack(0)
	fluxgraph.MakeEdge(cluster, rack, "contains")
	fluxgraph.MakeEdge(rack, cluster, "in")

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

	err = fluxgraph.WriteJGF(filename)
	if err != nil {
		return err
	}
	return nil

}
