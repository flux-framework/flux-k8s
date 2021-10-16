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
	"fluxcli"
	"fmt"
	"io/ioutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/metrics"
	"sigs.k8s.io/scheduler-plugins/pkg/kubeflux/jobspec"
	"sigs.k8s.io/scheduler-plugins/pkg/kubeflux/utils"
	"time"
)

type KubeFlux struct {
	handle  framework.Handle
	fluxctx *fluxcli.ReapiCtx
}

var _ framework.PreFilterPlugin = &KubeFlux{}
var _ framework.FilterPlugin = &KubeFlux{}

// let's give it a name
const (
	Name = "KubeFlux"
)

func (kf *KubeFlux) Name() string {
	return Name
}

type fluxStateData struct {
	nodeName string
}

func (s *fluxStateData) Clone() framework.StateData {
	clone := &fluxStateData{
		nodeName: s.nodeName,
	}
	return clone
}

func (kf *KubeFlux) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) *framework.Status {
	klog.Infof("Examining the pod")

	fluxjbs := jobspec.InspectPodInfo(pod)
	currenttime := time.Now()
	filename := fmt.Sprintf("/home/data/jobspecs/jobspec-%s-%s.yaml", currenttime.Format(time.RFC3339Nano), pod.Name)
	jobspec.CreateJobSpecYaml(fluxjbs, filename)

	nodename, err := kf.askFlux(ctx, pod, filename)
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, err.Error())
	}

	if nodename == "NONE" {
		fmt.Println("Pod cannot be scheduled by KubeFlux, nodename ", nodename)
		return framework.NewStatus(framework.Unschedulable, "Pod cannot be scheduled by KubeFlux, nodename "+nodename)
	}

	fmt.Println("Node Selected: ", nodename)

	state.Write(framework.StateKey(pod.Name), &fluxStateData{nodeName: nodename})

	return framework.NewStatus(framework.Success, "")
}

func (kf *KubeFlux) Filter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	fmt.Println("Filtering input node ", nodeInfo.Node().Name)
	if v, e := cycleState.Read(framework.StateKey(pod.Name)); e == nil {
		if value, ok := v.(*fluxStateData); ok && value.nodeName != nodeInfo.Node().Name {
			return framework.NewStatus(framework.Unschedulable, "pod is not permitted")
		} else {
			fmt.Println("Filter: node selected by Flux ", value.nodeName)
		}
	}

	return framework.NewStatus(framework.Success)
}

func (kf *KubeFlux) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func (kf *KubeFlux) askFlux(ctx context.Context, pod *v1.Pod, filename string) (string, error) {

	spec, err := ioutil.ReadFile(filename)
	if err != nil {
		// err := fmt.Errorf("Error reading jobspec file")
		return "", errors.New("Error reading jobspec")
	}
	start := time.Now()
	reserved, allocated, at, pre, post, overhead, jobid, fluxerr := fluxcli.ReapiCliMatchAllocate(kf.fluxctx, false, string(spec))
	elapsed := metrics.SinceInSeconds(start)
	fmt.Println("Time elapsed: ", elapsed)
	if fluxerr != 0 {
		// err := fmt.Errorf("Error in ReapiCliMatchAllocate")
		return "", errors.New("Error in ReapiCliMatchAllocate")
	}

	if allocated == "" {
		return "NONE", nil
	}

	printOutput(reserved, allocated, at, pre, post, overhead, jobid, fluxerr)
	nodename := fluxcli.ReapiCliGetNode(kf.fluxctx)
	fmt.Println("nodename ", nodename)

	return nodename, nil
}

// initialize and return a new Flux Plugin
func New(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	fctx := fluxcli.NewReapiCli()
	fmt.Println("Created cli context ", fctx)
	fmt.Printf("%+v\n", fctx)
	filename := "/home/data/jgf/kubecluster.json"
	err := utils.CreateJGF(handle, filename)
	if err != nil {
		return nil, err
	}

	jgf, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Println("Error reading JGF")
		return nil, err
	}

	fluxcli.ReapiCliInit(fctx, string(jgf))

	// if ret != 0 {
	// 	fmt.Println("Error while initializing ReapiCli")
	// 	return nil, errors.New("Error while initializing ReapiCli")
	// }
	klog.Infof("KubeFlux starts")

	return &KubeFlux{handle: handle, fluxctx: fctx}, nil
}

////// Utility functions
func printOutput(reserved bool, allocated string, at int64, pre uint32, post uint32, overhead float64, jobid uint64, fluxerr int) {
	fmt.Println("\n\t----Match Allocate output---")
	fmt.Printf("jobid: %d\nreserved: %t\nallocated: %s\nat: %d\npreorder visit count: %d\npostorder visit count: %d\noverhead: %f\nerror: %d\n", jobid, reserved, allocated, at, pre, post, overhead, fluxerr)
}
