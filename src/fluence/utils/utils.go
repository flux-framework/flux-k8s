package utils

import (
	"context"
	"fmt"

	"encoding/json"

	"github.com/flux-framework/flux-k8s/flux-plugin/fluence/jgf"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"
)

var (
	controlPlaneLabel = "node-role.kubernetes.io/control-plane"
)

// CreateJGF creates the Json Graph Format
func CreateJGF(filename string, skipLabel *string) error {
	ctx := context.Background()
	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Println("Error getting InClusterConfig")
		return err
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Println("Error getting ClientSet")
		return err
	}
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	var fluxgraph jgf.Fluxjgf
	fluxgraph = jgf.InitJGF()

	// TODO it looks like we can add more to the graph here -
	// let's remember to consider what else we can.
	// subnets := make(map[string]string)

	cluster := fluxgraph.MakeCluster("k8scluster")

	// Rack needs to be disabled when using subnets
	// rack := fluxgraph.MakeRack(0)

	// fluxgraph.MakeEdge(cluster, rack, "contains")
	// fluxgraph.MakeEdge(rack, cluster, "in")

	vcores := 0
	fmt.Println("Number nodes ", len(nodes.Items))
	var totalAllocCpu, totalmem int64
	totalAllocCpu = 0
	sdnCount := 0

	for node_index, node := range nodes.Items {

		// We should not be scheduling to the control plane
		_, ok := node.Labels[controlPlaneLabel]
		if ok {
			fmt.Println("Skipping control plane node ", node.GetName())
			continue
		}

		// Anything labeled with "skipLabel" meaning it is present,
		// should be skipped
		if *skipLabel != "" {
			_, ok := node.Labels[*skipLabel]
			if ok {
				fmt.Println("Skipping node ", node.GetName())
				continue
			}
		}

		fmt.Println("node in flux group ", node.GetName())
		if !node.Spec.Unschedulable {
			fieldselector, err := fields.ParseSelector("spec.nodeName=" + node.GetName() + ",status.phase!=" + string(corev1.PodSucceeded) + ",status.phase!=" + string(corev1.PodFailed))
			if err != nil {
				return err
			}
			pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
				FieldSelector: fieldselector.String(),
			})
			if err != nil {
				return err
			}

			// fmt.Println("Node ", node.GetName(), " has pods ", pods)
			// Check if subnet already exists
			// Here we build subnets according to topology.kubernetes.io/zone label
			subnetName := node.Labels["topology.kubernetes.io/zone"]
			subnet := fluxgraph.MakeSubnet(sdnCount, subnetName)
			sdnCount = sdnCount + 1
			fluxgraph.MakeEdge(cluster, subnet, "contains")
			fluxgraph.MakeEdge(subnet, cluster, "in")

			reqs := computeTotalRequests(pods)
			cpuReqs := reqs[corev1.ResourceCPU]
			memReqs := reqs[corev1.ResourceMemory]

			avail := node.Status.Allocatable.Cpu().MilliValue()
			totalcpu := int64((avail - cpuReqs.MilliValue()) / 1000) //- 1
			fmt.Println("Node ", node.GetName(), " flux cpu ", totalcpu)
			totalAllocCpu = totalAllocCpu + totalcpu
			totalmem = node.Status.Allocatable.Memory().Value() - memReqs.Value()
			fmt.Println("Node ", node.GetName(), " total mem ", totalmem)
			gpuAllocatable, hasGpuAllocatable := node.Status.Allocatable["nvidia.com/gpu"]

			// reslist := node.Status.Allocatable
			// resources := make([]corev1.ResourceName, 0, len(reslist))
			// for resource := range reslist {
			// 	fmt.Println("resource ", resource)
			// 	resources = append(resources, resource)
			// }
			// for _, resource := range resources {
			// 	value := reslist[resource]

			// 	fmt.Printf(" %s:\t%s\n", resource, value.String())
			// }

			workernode := fluxgraph.MakeNode(node_index, false, node.Name)
			fluxgraph.MakeEdge(subnet, workernode, "contains") // this is rack otherwise
			fluxgraph.MakeEdge(workernode, subnet, "in")       // this is rack otherwise

			// socket := fluxgraph.MakeSocket(0, "socket")
			// fluxgraph.MakeEdge(workernode, socket, "contains")
			// fluxgraph.MakeEdge(socket, workernode, "in")

			if hasGpuAllocatable {
				fmt.Println("GPU Resource quantity ", gpuAllocatable.Value())
				//MakeGPU(index int, name string, size int) string {
				for index := 0; index < int(gpuAllocatable.Value()); index++ {
					gpu := fluxgraph.MakeGPU(index, "nvidiagpu", 1)
					fluxgraph.MakeEdge(workernode, gpu, "contains") // workernode was socket
					fluxgraph.MakeEdge(gpu, workernode, "in")
				}

			}

			for index := 0; index < int(totalcpu); index++ {
				// MakeCore(index int, name string)
				core := fluxgraph.MakeCore(index, "core")
				fluxgraph.MakeEdge(workernode, core, "contains") // workernode was socket
				fluxgraph.MakeEdge(core, workernode, "in")

				// Question from Vanessa:
				// How can we get here and have vcores ever not equal to zero?
				if vcores == 0 {
					fluxgraph.MakeNFDProperties(core, index, "cpu-", &node.Labels)
					// fluxgraph.MakeNFDProperties(core, index, "netmark-", &node.Labels)
				} else {
					for vc := 0; vc < vcores; vc++ {
						vcore := fluxgraph.MakeVCore(core, vc, "vcore")
						fluxgraph.MakeNFDProperties(vcore, index, "cpu-", &node.Labels)
					}
				}
			}

			// MakeMemory(index int, name string, unit string, size int)
			fractionmem := totalmem >> 30
			// fractionmem := (totalmem/totalcpu) >> 20
			// fmt.Println("Creating ", fractionmem, " vertices with ", 1<<10, " MB of mem")
			for i := 0; i < /*int(totalcpu)*/ int(fractionmem); i++ {
				mem := fluxgraph.MakeMemory(i, "memory", "MB", int(1<<10))
				fluxgraph.MakeEdge(workernode, mem, "contains")
				fluxgraph.MakeEdge(mem, workernode, "in")
			}
		}
	}
	fmt.Println("Can request at most ", totalAllocCpu, " exclusive cpu")
	err = fluxgraph.WriteJGF(filename)
	if err != nil {
		return err
	}
	return nil

}

func computeTotalRequests(podList *corev1.PodList) (total map[corev1.ResourceName]resource.Quantity) {
	total = map[corev1.ResourceName]resource.Quantity{}
	for _, pod := range podList.Items {
		podReqs, _ := resourcehelper.PodRequestsAndLimits(&pod)
		for podReqName, podReqValue := range podReqs {
			if v, ok := total[podReqName]; !ok {
				total[podReqName] = podReqValue
			} else {
				v.Add(podReqValue)
				total[podReqName] = v
			}
		}
		// for podLimitName, podLimitValue := range podLimits {
		// 	if v, ok := total[podLimitName]; !ok {
		// 		total[podLimitName] = podLimitValue
		// 	} else {
		// 		v.Add(podLimitValue)
		// 		total[podLimitName] = v
		// 	}
		// }
	}
	return
}

type allocation struct {
	Type      string
	Name      string
	Basename  string
	CoreCount int
}

// ParseAllocResult takes an allocated (string) and parses into a list of allocation
// We include the pod namespace/name for debugging later
func ParseAllocResult(allocated, podName string) []allocation {
	var dat map[string]interface{}
	result := []allocation{}

	// Keep track of total core count across allocated
	corecount := 0

	// This should not happen - the string we get back should parse.
	if err := json.Unmarshal([]byte(allocated), &dat); err != nil {
		panic(err)
	}
	// Parse graph and nodes into interfaces
	// TODO look at github.com/mitchellh/mapstructure
	// that might make this easier
	nodes := dat["graph"].(interface{})
	str1 := nodes.(map[string]interface{})
	str2 := str1["nodes"].([]interface{})

	for _, item := range str2 {
		str1 = item.(map[string]interface{})
		metadata := str1["metadata"].(map[string]interface{})
		if metadata["type"].(string) == "core" {
			corecount = corecount + 1
		}
		if metadata["type"].(string) == "node" {
			result = append(result, allocation{
				Type:      metadata["type"].(string),
				Name:      metadata["name"].(string),
				Basename:  metadata["basename"].(string),
				CoreCount: corecount,
			})

			// Reset the corecount once we've added to a node
			corecount = 0
		}
	}
	fmt.Printf("Final node result for %s\n", podName)
	for i, alloc := range result {
		fmt.Printf("Node %d: %s\n", i, alloc.Name)
		fmt.Printf("  Type: %s\n  Name: %s\n  Basename: %s\n  CoreCount: %d\n",
			alloc.Type, alloc.Name, alloc.Basename, alloc.CoreCount)

	}
	return result
}

// Utility functions
func PrintOutput(reserved bool, allocated string, at int64, overhead float64, jobid uint64, fluxerr error) {
	fmt.Println("\n\t----Match Allocate output---")
	fmt.Printf("jobid: %d\nreserved: %t\nallocated: %s\nat: %d\noverhead: %f\n", jobid, reserved, allocated, at, overhead)

	// Only print error if we had one
	if fluxerr != nil {
		fmt.Printf("error: %w\n", fluxerr)
	}
}
