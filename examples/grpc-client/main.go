package main

import (
	"fmt"
	"context"
	"time"
	"google.golang.org/grpc"
	"net"
	pb "example.com/fluxclient/fluxcli-grpc"
	"k8s.io/api/core/v1"
)



func main() {
	sock := "/tmp/echo.sock"

	conn, err := grpc.Dial(
		sock,
		grpc.WithInsecure(),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}))
	if err != nil {
		fmt.Println("[FluxClient] Error connecting to server: %v", err)
		return
	}
	defer conn.Close()


	grpcclient := pb.NewFluxcliServiceClient(conn)
	_, cancel := context.WithTimeout(context.Background(), 200*time.Second)
	defer cancel()

	jobspec := &pb.MatchRequest{
		Ps: &pb.PodSpec{
			Id:        "podID",
			Container: "fake-busybox",
			Cpu:  1,
			// Memory:    1,
			// Gpu:       0,
			// Storage:   0,
		},
		Request: "allocate"}

	r, err2 := grpcclient.Match(context.Background(), jobspec)
	if err2 != nil {
		fmt.Printf("[FluxClient] did not receive any match response: %v\n", err2)
		return
	}

	fmt.Printf("[FluxClient] response nodeID %s\n", r.GetNodeID())
	fmt.Printf("[FluxClient] response podID %s\n", r.GetPodID())
}


type PodRequest struct {
	ID string
	// Pod        *v1.Pod
	Containers []v1.Container
	CPU        []int64
	Memory     []int64
	Gpu        []int64
	Storage    []int64
	Labels     map[string]string
}

func InspectPodInfo(pod *v1.Pod) *PodRequest {
	pr := new(PodRequest)
	pr.ID = pod.Name
	// pr.Pod = pod
	pr.Containers = pod.Spec.Containers
	numcontainers := len(pr.Containers)
	pr.CPU = make([]int64, numcontainers)
	pr.Memory = make([]int64, numcontainers)
	pr.Gpu = make([]int64, numcontainers)
	pr.Storage = make([]int64, numcontainers)
	fmt.Println("[JobSpec] pod labels ", pod.Labels)
	pr.Labels = pod.Labels

	for i, cont := range pr.Containers {
		// fmt.Printf("[FML] \n%v\n", cont)
		specRequests := cont.Resources.Requests
		specLimits := cont.Resources.Limits

		if specRequests.Cpu().Value() == 0 {
			pr.CPU[i] = 1
		} else {
			pr.CPU[i] = specRequests.Cpu().Value()
		}
		if specRequests.Memory().Value() > 0 {
			pr.Memory[i] = specRequests.Memory().Value()
		}
		gpu := specLimits["nvidia.com/gpu"]
		pr.Gpu[i] = gpu.Value()
		pr.Storage[i] = specRequests.StorageEphemeral().Value()
		//for key := range pod.Spec.Containers[0].Resources.Requests {
		//	fmt.Printf("Requests key %v %p\n", key, key)
		//}

		fmt.Printf("[Jobspec] Pod spec: CPU %v/%v-milli, memory %v/%v-milli, GPU %v, storage %v\n", pr.CPU[i], specRequests.Cpu().MilliValue(),
			pr.Memory[i], specRequests.Memory().MilliValue(), pr.Gpu[i], pr.Storage[i])
	}

	fmt.Println("[Jobspec] Node selector: ", pod.Spec.NodeSelector)
	return pr
}