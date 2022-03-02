package utils

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubeflux/jgf"
	"encoding/json"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/kubernetes"
)


func CreateJGF(filename string) error {
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
	// clientset := handle.ClientSet()
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	var fluxgraph jgf.Fluxjgf
	fluxgraph = jgf.InitJGF()
	// subnets := make(map[string]string)
	cluster := fluxgraph.MakeCluster("k8scluster")
	rack := fluxgraph.MakeRack(0)

	fluxgraph.MakeEdge(cluster, rack, "contains")
	fluxgraph.MakeEdge(rack, cluster, "in")

	vcores := 0
	fmt.Println("Number nodes ", len(nodes.Items))
	// sdnCount := 0
	for node_index, node := range nodes.Items {
		_, master := node.Labels["node-role.kubernetes.io/master"]
		if !master {

			// Check if subnet already exists
			// Here we build subnets according to IP addresses of nodes.
			// This was for GROMACS, therefore I comment that out and go back
			// to build racks. One day, this will be customized by users.

			// fmt.Println("Node Addresses ", node.Status.Addresses)
			// lastBin := strings.LastIndex( node.Status.Addresses[1].Address, "." )
			// subnetName := node.Status.Addresses[1].Address[0:lastBin]
			// fmt.Printf("Subnet is: %v\n", subnetName)

			// if _, ok := subnets[subnetName]; !ok {
			// 	fmt.Println("New subnet ", subnetName)
			// 	subnet := fluxgraph.MakeSubnet(sdnCount, subnetName)
			// 	sdnCount = sdnCount+1
			// 	subnets[subnetName] = subnet
			// 	fluxgraph.MakeEdge(cluster, subnet, "contains")
			// 	fluxgraph.MakeEdge(subnet, cluster, "in")
			// }
			// subnet := subnets[subnetName]

			totalcpu, _ := node.Status.Capacity.Cpu().AsInt64()
			totalmem, _ := node.Status.Capacity.Memory().AsInt64()
			workernode := fluxgraph.MakeNode(node_index, false, node.Name)
			fluxgraph.MakeEdge(rack, workernode, "contains")
			fluxgraph.MakeEdge(workernode, rack, "in")

			socket := fluxgraph.MakeSocket(0, "socket")
			fluxgraph.MakeEdge(workernode, socket, "contains")
			fluxgraph.MakeEdge(socket, workernode, "in")

			for index := 0; index < int(totalcpu); index++ {
				// MakeCore(index int, name string)
				core := fluxgraph.MakeCore(index, "core")
				fluxgraph.MakeEdge(socket, core, "contains")
				fluxgraph.MakeEdge(core, socket, "in")
				if vcores == 0 {
					fluxgraph.MakeNFDProperties(core, index, "cpu-", &node.Labels)
				} else {
					for vc := 0; vc < vcores; vc++ {
						vcore := fluxgraph.MakeVCore(core, vc, "vcore")
						fluxgraph.MakeNFDProperties(vcore, index, "cpu-", &node.Labels)
					}
				}
			}

			//  MakeMemory(index int, name string, unit string, size int
			mem := fluxgraph.MakeMemory(0, "memory", "KB", int(totalmem))
			fluxgraph.MakeEdge(socket, mem, "contains")
			fluxgraph.MakeEdge(mem, socket, "in")
		}
	}

	err = fluxgraph.WriteJGF(filename)
	if err != nil {
		return err
	}
	return nil

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
