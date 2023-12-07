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

package core

import (
	"fmt"

	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	pb "sigs.k8s.io/scheduler-plugins/pkg/fluence/fluxcli-grpc"
)

type FluxStateData struct {
	NodeName string
}

func (s *FluxStateData) Clone() framework.StateData {
	clone := &FluxStateData{
		NodeName: s.NodeName,
	}
	return clone
}

type NodePodsCount struct {
	NodeName string
	Count    int
}

var podgroupMap map[string][]NodePodsCount

func Init() {
	podgroupMap = make(map[string][]NodePodsCount, 0)
}

func (n *NodePodsCount) Clone() framework.StateData {
	return &NodePodsCount{
		NodeName: n.NodeName,
		Count:    n.Count,
	}
}

func CreateNodePodsList(nodelist []*pb.NodeAlloc, pgname string) (nodepods []NodePodsCount) {
	nodepods = make([]NodePodsCount, len(nodelist))
	for i, v := range nodelist {
		nodepods[i] = NodePodsCount{
			NodeName: v.GetNodeID(),
			Count:    int(v.GetTasks()),
		}
	}
	podgroupMap[pgname] = nodepods
	klog.Info("MAP ", podgroupMap)

	return
}

func HaveList(pgname string) bool {
	_, exists := podgroupMap[pgname]
	return exists
}

func GetNextNode(pgname string) (string, error) {
	entry, ok := podgroupMap[pgname]
	if !ok {
		err := fmt.Errorf("Map is empty")
		return "", err
	}
	if len(entry) == 0 {
		err := fmt.Errorf("Error while getting a node")
		return "", err
	}

	nodename := entry[0].NodeName

	if entry[0].Count == 1 {
		slice := entry[1:]
		if len(slice) == 0 {
			delete(podgroupMap, pgname)
			return nodename, nil
		}
		podgroupMap[pgname] = slice
		return nodename, nil
	}
	entry[0].Count = entry[0].Count - 1
	return nodename, nil
}
