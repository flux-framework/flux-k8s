package utils

import (
	"context"
	"fmt"
	// "strings"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	"github.com/flux-framework/flux-k8s/flux-plugin/fluence/jgf"
	"encoding/json"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/kubernetes"
	"k8s.io/apimachinery/pkg/fields"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"
	"k8s.io/apimachinery/pkg/api/resource"
)


func CreateJGF(filename string, label *string) error {
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
		// _, worker := node.Labels["node-role.kubernetes.io/worker"]
		if *label != "" {
			_, fluxnode := node.Labels[*label]
			if !fluxnode {
				fmt.Println("Skipping node ",  node.GetName())
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
			sdnCount = sdnCount+1
			fluxgraph.MakeEdge(cluster, subnet, "contains")
			fluxgraph.MakeEdge(subnet, cluster, "in")
	
		
			reqs := computeTotalRequests(pods)
			cpuReqs := reqs[corev1.ResourceCPU]
			memReqs := reqs[corev1.ResourceMemory]
			
			avail := node.Status.Allocatable.Cpu().MilliValue()
			totalcpu := int64((avail-cpuReqs.MilliValue())/1000) //- 1
			fmt.Println("Node ", node.GetName(), " flux cpu ", totalcpu)
			totalAllocCpu = totalAllocCpu+totalcpu
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
			fluxgraph.MakeEdge(workernode, subnet, "in") // this is rack otherwise

			// socket := fluxgraph.MakeSocket(0, "socket")
			// fluxgraph.MakeEdge(workernode, socket, "contains")
			// fluxgraph.MakeEdge(socket, workernode, "in")
			
			if hasGpuAllocatable {
				fmt.Println("GPU Resource quantity ", gpuAllocatable.Value())
				//MakeGPU(index int, name string, size int) string {
				for index :=0; index < int(gpuAllocatable.Value()); index++ {
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
			for i:=0; i < /*int(totalcpu)*/int(fractionmem); i++ {
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
	Type 		string
	Name 		string
	Basename 	string
	CoreCount	int
}

func ParseAllocResult(allocated string) []allocation{
	var dat map[string]interface{}
	result := make([]allocation, 0)
	corecount := 0
	if err := json.Unmarshal([]byte(allocated), &dat); err != nil {
        panic(err)
    }
	// fmt.Println("PRINTING DATA:\n", dat)
	// graph := dat["graph"]
	// fmt.Println("GET GRAPH:\n ", graph)
	nodes := dat["graph"].(interface{})
	str1 := nodes.(map[string]interface {})
    // fmt.Println("GET NODES:\n", str1["nodes"])
	str2 := str1["nodes"].([]interface {})
	// fmt.Println("NODES:\n", len(str2))
	for _, item := range str2 {
		// fmt.Println("ITEM: ", item)
		str1 = item.(map[string]interface {})
		metadata := str1["metadata"].(map[string]interface{})
		// fmt.Println("TYPE: ", metadata["type"])
		if metadata["type"].(string) == "core" {
			corecount = corecount + 1
		}
		// fmt.Println("BASENAME: ", metadata["basename"])
		if metadata["type"].(string) == "node" {
			result = append(result, allocation{
				Type : metadata["type"].(string),
				Name : metadata["name"].(string),
				Basename : metadata["basename"].(string),
				CoreCount : corecount,
			})
			corecount = 0
			// result.Type = metadata["type"].(string)
			// result.Name = metadata["name"].(string)
			// result.Basename = metadata["basename"].(string)
			// return result
		}
	}
	fmt.Println("FINAL NODE RESULT:\n", result)
	return result
}

////// Utility functions
func PrintOutput(reserved bool, allocated string, at int64, overhead float64, jobid uint64, err error) {
	fmt.Println("\n\t----Match Allocate output---")
	fmt.Printf("jobid: %d\nreserved: %t\nallocated: %s\nat: %d\noverhead: %f\nerror: %s\n", jobid, reserved, allocated, at, overhead, err)
}
