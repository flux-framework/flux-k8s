package utils

import (
	"context"
	"fmt"

	klog "k8s.io/klog/v2"

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
	controlPlaneLabel  = "node-role.kubernetes.io/control-plane"
	defaultClusterName = "k8scluster"
)

// RegisterExisting uses the in cluster API to get existing pods
// This is actually the same as computeTotalRequests but I wanted to compare the two
// It is currently not being used. The main difference is that below, we are essentially
// rounding the cpu to the smaller unit (logically for the graph) but losing some
// granularity, if we think "milli" values have feet.
func RegisterExisting(clientset *kubernetes.Clientset, ctx context.Context) (map[string]PodSpec, error) {

	// We are using PodSpec as a holder for a *summary* of cpu/memory being used
	// by the node, it is a summation across pods we find on each one
	nodes := map[string]PodSpec{}

	// get pods in all the namespaces by omitting namespace
	// Or specify namespace to get pods in particular namespace
	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Infof("Error listing pods: %s\n", err)
		return nodes, err
	}
	klog.Infof("Found %d existing pods in the cluster\n", len(pods.Items))

	// Create a new PodSpec for each
	for _, pod := range pods.Items {

		// Add the node to our lookup if we don't have it yet
		_, ok := nodes[pod.Spec.NodeName]
		if !ok {
			nodes[pod.Spec.NodeName] = PodSpec{}
		}
		ps := nodes[pod.Spec.NodeName]

		for _, container := range pod.Spec.Containers {
			specRequests := container.Resources.Requests
			ps.Cpu += int32(specRequests.Cpu().Value())
			ps.Memory += specRequests.Memory().Value()
			ps.Storage += specRequests.StorageEphemeral().Value()

			specLimits := container.Resources.Limits
			gpuSpec := specLimits["nvidia.com/gpu"]
			ps.Gpu += gpuSpec.Value()
		}
		nodes[pod.Spec.NodeName] = ps
	}
	return nodes, nil
}

// CreateInClusterJGF creates the Json Graph Format from the Kubernetes API
// We currently don't have support in fluxion to allocate jobs for existing pods,
// so instead we create the graph with fewer resources. When that support is
// added (see sig-scheduler-plugins/pkg/fluence/register.go) we can
// remove the adjustment here, which is more of a hack
func CreateInClusterJGF(filename string, skipLabel string) error {
	ctx := context.Background()
	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Println("Error getting InClusterConfig")
		return err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("Error getting ClientSet: %s", err)
		return err
	}
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Error listing nodes: %s", err)
		return err
	}

	// Create a Flux Json Graph Format (JGF) with all cluster nodes
	fluxgraph := jgf.NewFluxJGF()

	// Initialize the cluster. The top level of the graph is the cluster
	// This assumes fluxion is only serving one cluster.
	// previous comments indicate that we choose between the level
	// of a rack and a subnet. A rack doesn't make sense (the nodes could
	// be on multiple racks) so subnet is likely the right abstraction
	clusterNode, err := fluxgraph.InitCluster(defaultClusterName)
	if err != nil {
		return err
	}
	fmt.Println("Number nodes ", len(nodes.Items))

	// TODO for follow up / next PR:
	// Metrics / summary should be an attribute of the JGF outer flux graph
	// Resources should come in from entire group (and not repres. pod)
	var totalAllocCpu int64
	totalAllocCpu = 0

	// Keep a lookup of subnet nodes in case we see one twice
	// We don't want to create a new entity for it in the graph
	subnetLookup := map[string]jgf.Node{}

	for _, node := range nodes.Items {

		// We should not be scheduling to the control plane
		_, ok := node.Labels[controlPlaneLabel]
		if ok {
			fmt.Println("Skipping control plane node ", node.GetName())
			continue
		}

		// Anything labeled with "skipLabel" meaning it is present,
		// should be skipped
		if skipLabel != "" {
			_, ok := node.Labels[skipLabel]
			if ok {
				fmt.Printf("Skipping node %s\n", node.GetName())
				continue
			}
		}

		if node.Spec.Unschedulable {
			fmt.Printf("Skipping node %s, unschedulable\n", node.GetName())
			continue
		}

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

		// Have we seen this subnet node before?
		subnetName := node.Labels["topology.kubernetes.io/zone"]
		subnetNode, exists := subnetLookup[subnetName]
		if !exists {
			// Build the subnet according to topology.kubernetes.io/zone label
			subnetNode = fluxgraph.MakeSubnet(subnetName)

			// This is one example of bidirectional, I won't document in
			// all following occurrences but this is what the function does
			// [cluster] -> contains -> [subnet]
			// [subnet]  ->       in -> [cluster]
			fluxgraph.MakeBidirectionalEdge(clusterNode.Id, subnetNode.Id)
		}

		// These are requests for existing pods, for cpu and memory
		reqs := computeTotalRequests(pods)
		cpuReqs := reqs[corev1.ResourceCPU]
		memReqs := reqs[corev1.ResourceMemory]

		// Actual values that we have available (minus requests)
		totalCpu := node.Status.Allocatable.Cpu().MilliValue()
		totalMem := node.Status.Allocatable.Memory().Value()

		// Values accounting for requests
		availCpu := int64((totalCpu - cpuReqs.MilliValue()) / 1000)
		availMem := totalMem - memReqs.Value()

		// Show existing to compare to
		fmt.Printf("\n📦️ %s\n", node.GetName())
		fmt.Printf("      allocated cpu: %d\n", cpuReqs.Value())
		fmt.Printf("      allocated mem: %d\n", memReqs.Value())
		fmt.Printf("      available cpu: %d\n", availCpu)
		fmt.Printf("       running pods: %d\n", len(pods.Items))

		// keep track of overall total
		totalAllocCpu += availCpu
		fmt.Printf("      available mem: %d\n", availMem)
		gpuAllocatable, hasGpuAllocatable := node.Status.Allocatable["nvidia.com/gpu"]

		// TODO possibly look at pod resources vs. node.Status.Allocatable
		// Make the compute node, which is a child of the subnet
		// The parameters here are the node name, and the parent path
		computeNode := fluxgraph.MakeNode(node.Name, subnetNode.Metadata.Name)

		// [subnet] -> contains -> [compute node]
		fluxgraph.MakeBidirectionalEdge(subnetNode.Id, computeNode.Id)

		// Here we are adding GPU resources under nodes
		if hasGpuAllocatable {
			fmt.Println("GPU Resource quantity ", gpuAllocatable.Value())
			for index := 0; index < int(gpuAllocatable.Value()); index++ {

				// The subpath (from and not including root) is the subnet -> node
				subpath := fmt.Sprintf("%s/%s", subnetNode.Metadata.Name, computeNode.Metadata.Name)

				// TODO: can this size be greater than 1?
				gpuNode := fluxgraph.MakeGPU(jgf.NvidiaGPU, subpath, 1)

				// [compute] -> contains -> [gpu]
				fluxgraph.MakeBidirectionalEdge(computeNode.Id, gpuNode.Id)
			}

		}

		// Here is where we are adding cores
		for index := 0; index < int(availCpu); index++ {
			subpath := fmt.Sprintf("%s/%s", subnetNode.Metadata.Name, computeNode.Metadata.Name)
			coreNode := fluxgraph.MakeCore(jgf.CoreType, subpath)
			fluxgraph.MakeBidirectionalEdge(computeNode.Id, coreNode.Id)
		}

		// Here is where we are adding memory
		fractionMem := availMem >> 30
		for i := 0; i < int(fractionMem); i++ {
			subpath := fmt.Sprintf("%s/%s", subnetNode.Metadata.Name, computeNode.Metadata.Name)
			memoryNode := fluxgraph.MakeMemory(jgf.MemoryType, subpath, 1<<10)
			fluxgraph.MakeBidirectionalEdge(computeNode.Id, memoryNode.Id)
		}
	}
	fmt.Printf("\nCan request at most %d exclusive cpu", totalAllocCpu)
	err = fluxgraph.WriteJGF(filename)
	if err != nil {
		return err
	}
	return nil
}

// computeTotalRequests sums up the pod requests for the list. We do not consider limits.
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
	nodes := dat["graph"]
	str1 := nodes.(map[string]interface{})
	str2 := str1["nodes"].([]interface{})

	for _, item := range str2 {
		str1 = item.(map[string]interface{})
		metadata := str1["metadata"].(map[string]interface{})
		if metadata["type"].(string) == jgf.CoreType {
			corecount = corecount + 1
		}
		if metadata["type"].(string) == jgf.NodeType {
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
		fmt.Printf("error: %s\n", fluxerr)
	}
}
