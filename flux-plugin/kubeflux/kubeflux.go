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
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/metrics"
	"sigs.k8s.io/scheduler-plugins/pkg/kubeflux/jobspec"
	"sigs.k8s.io/scheduler-plugins/pkg/kubeflux/utils"
	"sync"
	"time"
)

type KubeFlux struct {
	mutex          sync.Mutex
	handle         framework.Handle
	fluxctx        *fluxcli.ReapiCtx
	podNameToJobId map[string]uint64
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

	kf.mutex.Lock()
	kf.podNameToJobId[pod.Name] = jobid
	kf.mutex.Unlock()

	fmt.Println("Check job set:")
	fmt.Println(kf.podNameToJobId)

	return nodename, nil
}

// initialize and return a new Flux Plugin
func New(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {

	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Println("Error getting InClusterConfig")
		return nil, err
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Println("Error getting ClientSet")
		return nil, err
	}

	// TODO: create informer
	factory := informers.NewSharedInformerFactory(clientset, 0)
	podInformer := factory.Core().V1().Pods().Informer()
	nodeInformer := factory.Core().V1().Nodes().Informer()

	fctx := fluxcli.NewReapiCli()
	fmt.Println("Created cli context ", fctx)
	fmt.Printf("%+v\n", fctx)
	filename := "/home/data/jgf/kubecluster.json"
	err = utils.CreateJGF(handle, filename)
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

	kf := &KubeFlux{handle: handle, fluxctx: fctx, podNameToJobId: make(map[string]uint64)}

	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    kf.addPod,
		UpdateFunc: kf.updatePod,
		DeleteFunc: kf.deletePod,
	})

	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    kf.addNode,
		UpdateFunc: kf.updateNode,
		DeleteFunc: kf.deleteNode,
	})

	// TODO: this informer can not be stopped politely
	stopPodInformer := make(chan struct{})
	go podInformer.Run(stopPodInformer)
	stopNodeInformer := make(chan struct{})
	go nodeInformer.Run(stopNodeInformer)

	return kf, nil
}

////// EventHandlers

func (kf *KubeFlux) addPod(obj interface{}) {
	fmt.Println("add Pod event handler")
	pod := obj.(*v1.Pod)
	fmt.Println(pod)
	fmt.Println(pod.Name, pod.Status)
}

func (kf *KubeFlux) updatePod(oldObj, newObj interface{}) {
	fmt.Println("update Pod event handler")
	oldPod := oldObj.(*v1.Pod)
	newPod := newObj.(*v1.Pod)
	fmt.Println(oldPod)
	fmt.Println(newPod)
	fmt.Println(oldPod.Name, oldPod.Status)
	fmt.Println(newPod.Name, newPod.Status)

	if newPod.Namespace == "default" && newPod.Status.Phase == v1.PodSucceeded {
		fmt.Printf("Must tell kubeflux %s finished\n", newPod.Name)

		kf.mutex.Lock()
		defer kf.mutex.Unlock()

		if jobid, ok := kf.podNameToJobId[newPod.Name]; ok {
			// possbile typo in https://github.com/cmisale/flux-sched/blob/gobind-dev/resource/hlapi/bindings/go/src/fluxcli/reapi_cli.go
			err := fluxcli.ReapiCliCancel(kf.fluxctx, int64(jobid), false)

			if err == 0 {
				delete(kf.podNameToJobId, newPod.Name)
			} else {
				fmt.Printf("Failed to delete pod %s from the pdoname-jobid map.\n", newPod.Name)
			}

			fmt.Printf("Job cancellation result: %d\n", err)
			fmt.Println("Check job set: after delete")
			fmt.Println(kf.podNameToJobId)
		} else {
			fmt.Printf("Succeeded pod %s/%s doesn't have flux jobid\n", newPod.Namespace, newPod.Name)

		}

	}

}

func (kf *KubeFlux) deletePod(obj interface{}) {
	fmt.Println("delete Pod event handler")
	pod := obj.(*v1.Pod)
	fmt.Println(pod)
	fmt.Println(pod.Name, pod.Status)
}

func (kf *KubeFlux) addNode(obj interface{}) {
	fmt.Println("add Node event handler")
	node := obj.(*v1.Node)
	fmt.Println(node.Name)
	fmt.Println(node)
}

func (kf *KubeFlux) updateNode(oldObj, newObj interface{}) {
	fmt.Println("update Node event handler")
	oldNode := oldObj.(*v1.Node)
	newNode := newObj.(*v1.Node)
	fmt.Println(newNode.Name)
	fmt.Println(oldNode)
	fmt.Println(newNode)
}

func (kf *KubeFlux) deleteNode(obj interface{}) {
	fmt.Println("delete Node event handler")
	node := obj.(*v1.Node)
	fmt.Println(node.Name)
	fmt.Println(node)
}

////// Utility functions
func printOutput(reserved bool, allocated string, at int64, pre uint32, post uint32, overhead float64, jobid uint64, fluxerr int) {
	fmt.Println("\n\t----Match Allocate output---")
	fmt.Printf("jobid: %d\nreserved: %t\nallocated: %s\nat: %d\npreorder visit count: %d\npostorder visit count: %d\noverhead: %f\nerror: %d\n", jobid, reserved, allocated, at, pre, post, overhead, fluxerr)
}
