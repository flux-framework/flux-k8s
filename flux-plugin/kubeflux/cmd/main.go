package main

import (
	"fmt"
	pb "kubeflux/fluxcli-grpc"
	"net"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc"
	"time"
	"io/ioutil"
	"context"
	"fluxcli"
	"kubeflux/utils" 
	"kubeflux/jobspec"
	"errors"
)


const (
	port = ":4242"
)

const SockAddr = "/tmp/echo.sock"

var responsechan chan string

// server is used to implement grpc_rq.FluxcliServiceServer.
type server struct{
	fctx	*fluxcli.ReapiCtx 
	pb.UnimplementedFluxcliServiceServer
}


func main () {
	fmt.Println("this is the fluxion grpc server")
	
	fctx := fluxcli.NewReapiCli()
	fmt.Println("Created cli context ", fctx)
	fmt.Printf("%+v\n", fctx)
	filename := "/temporary/cluster.json" //"/home/data/jgf/kubecluster.json"
	// err := utils.CreateJGF(filename)
	// if err != nil {
	// 	return 
	// }
	
	jgf, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Println("Error reading JGF")
		return 
	}
	
	fluxcli.ReapiCliInit(fctx, string(jgf), "{}")

	// From ancient kubeflux, just placeholders to make things clear

	lis, err := net.Listen("unix", SockAddr)

	if err != nil {
		fmt.Printf("[GRPCServer] failed to listen: %v\n", err)
	}
	responsechan = make(chan string)
	s := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 5 * time.Minute,          
		}),
	)
	pb.RegisterFluxcliServiceServer(s, &server{fctx: fctx})
	fmt.Printf("[GRPCServer] gRPC Listening on %s\n", lis.Addr().String())
	if err := s.Serve(lis); err != nil {
		fmt.Printf("[GRPCServer] failed to serve: %v\n", err)
	}
	
	fmt.Printf("[GRPCServer] Exiting\n")
}

func (s *server) Match(ctx context.Context, in *pb.MatchRequest) (*pb.MatchResponse, error) {
	// fluxjbs := jobspec.InspectPodInfo(in.Ps)
	// currenttime := time.Now()
	// filename := fmt.Sprintf("/home/data/jobspecs/jobspec-%s-%s.yaml", currenttime.Format(time.RFC3339Nano), in.Ps.Id)
	filename := "/home/data/jobspecs/jobspec.yaml"
	jobspec.CreateJobSpecYaml(in.Ps, filename)


	// filename = "/home/test001.yaml"
	spec, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.New("Error reading jobspec")
	}

	fmt.Printf("[GRPCServer] Received request %v\n", in)
	fmt.Printf("[GRPCServer] string read %v\n", string(spec))
	reserved, allocated, at, overhead, jobid, fluxerr := fluxcli.ReapiCliMatchAllocate(s.fctx, false, string(spec))
	fmt.Printf("Errors so far: %s\n", fluxcli.ReapiCliGetErrMsg(s.fctx))
	printOutput(reserved, allocated, at, overhead, jobid, fluxerr)
	if fluxerr != 0 {
		return nil, errors.New("Error in ReapiCliMatchAllocate")
	}

	if allocated == "" {
		return nil, nil
	}

	printOutput(reserved, allocated, at, overhead, jobid, fluxerr)

	node := utils.ParseAllocResult(allocated)
	nodename := node.Name
	fmt.Println("nodename ", nodename)
	
	mr := &pb.MatchResponse{PodID: in.Ps.Id, NodeID: nodename}
	fmt.Printf("[GRPCServer] Response %v \n", mr)
	return mr, nil
}

////// Utility functions
func printOutput(reserved bool, allocated string, at int64, overhead float64, jobid uint64, fluxerr int) {
	fmt.Println("\n\t----Match Allocate output---")
	fmt.Printf("jobid: %d\nreserved: %t\nallocated: %s\nat: %d\noverhead: %f\nerror: %d\n", jobid, reserved, allocated, at, overhead, fluxerr)
}

// func InspectPodInfo(pod *v1.Pod) *PodRequest {
// 	pr := new(PodRequest)
// 	pr.ID = pod.Name
// 	// pr.Pod = pod
// 	pr.Containers = pod.Spec.Containers
// 	numcontainers := len(pr.Containers)
// 	pr.CPU = make([]int64, numcontainers)
// 	pr.Memory = make([]int64, numcontainers)
// 	pr.Gpu = make([]int64, numcontainers)
// 	pr.Storage = make([]int64, numcontainers)
// 	fmt.Println("[JobSpec] pod labels ", pod.Labels)
// 	pr.Labels = pod.Labels

// 	for i, cont := range pr.Containers {
// 		// fmt.Printf("[FML] \n%v\n", cont)
// 		specRequests := cont.Resources.Requests
// 		specLimits := cont.Resources.Limits

// 		if specRequests.Cpu().Value() == 0 {
// 			pr.CPU[i] = 1
// 		} else {
// 			pr.CPU[i] = specRequests.Cpu().Value()
// 		}
// 		if specRequests.Memory().Value() > 0 {
// 			pr.Memory[i] = specRequests.Memory().Value()
// 		}
// 		gpu := specLimits["nvidia.com/gpu"]
// 		pr.Gpu[i] = gpu.Value()
// 		pr.Storage[i] = specRequests.StorageEphemeral().Value()
// 		//for key := range pod.Spec.Containers[0].Resources.Requests {
// 		//	fmt.Printf("Requests key %v %p\n", key, key)
// 		//}

// 		fmt.Printf("[Jobspec] Pod spec: CPU %v/%v-milli, memory %v/%v-milli, GPU %v, storage %v\n", pr.CPU[i], specRequests.Cpu().MilliValue(),
// 			pr.Memory[i], specRequests.Memory().MilliValue(), pr.Gpu[i], pr.Storage[i])
// 	}

// 	fmt.Println("[Jobspec] Node selector: ", pod.Spec.NodeSelector)
// 	return pr
// }