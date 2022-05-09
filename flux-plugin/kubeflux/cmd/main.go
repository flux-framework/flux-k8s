package main

import (
	"fmt"
	"flag"
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
	fmt.Println("This is the fluxion grpc server")
	policy := flag.String("policy", "", "Match policy")

	flag.Parse()
	fctx := fluxcli.NewReapiCli()
	fmt.Println("Created cli context ", fctx)
	fmt.Printf("%+v\n", fctx)
	filename := "/home/data/jgf/kubecluster.json"
	err := utils.CreateJGF(filename)
	if err != nil {
		return
	}
	
	jgf, err := ioutil.ReadFile(filename)
	if err != nil {
		fmt.Println("Error reading JGF")
		return
	}
	
	p := "{}"
	if *policy != "" {
		p = string("{\"matcher_policy\": \"" + *policy + "\"}")
		fmt.Println("Match policy: ", p)
	} 
	
	fluxcli.ReapiCliInit(fctx, string(jgf), p)


	// lis, err := net.Listen("unix", SockAddr)
	lis, err := net.Listen("tcp", port)
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

func (s *server) Cancel(ctx context.Context, in *pb.CancelRequest) (*pb.CancelResponse, error) {
	fmt.Printf("[GRPCServer] Received Cancel request %v\n", in)
	err := fluxcli.ReapiCliCancel(s.fctx, int64(in.JobID), true)
	if err < 0 {
		return nil, errors.New("Error in Cancel")
	}

	dr := &pb.CancelResponse{JobID: in.JobID, Error: int32(err)}
	fmt.Printf("[GRPCServer] Sending Cancel response %v\n", dr)

	fmt.Printf("[CancelRPC] Errors so far: %s\n", fluxcli.ReapiCliGetErrMsg(s.fctx))
	
	//func ReapiCliInfo(ctx *ReapiCtx, jobid int64) (reserved bool, at int64, overhead float64, mode string, err int)
	reserved, at, overhead, mode, fluxerr := fluxcli.ReapiCliInfo(s.fctx, int64(in.JobID))

	fmt.Println("\n\t----Job Info output---")
	fmt.Printf("jobid: %d\nreserved: %s\nat: %t\noverhead: %d\nmode: %f\nerror: %d\n", in.JobID, reserved, at, overhead, mode, fluxerr)

	fmt.Printf("[GRPCServer] Sending Cancel response %v\n", dr)
	return dr, nil
}

func (s *server) Match(ctx context.Context, in *pb.MatchRequest) (*pb.MatchResponse, error) {
	// currenttime := time.Now()
	// filename := fmt.Sprintf("/home/data/jobspecs/jobspec-%s-%s.yaml", currenttime.Format(time.RFC3339Nano), in.Ps.Id)
	filename := "/home/data/jobspecs/jobspec.yaml"
	jobspec.CreateJobSpecYaml(in.Ps, in.Count, filename)

	spec, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.New("Error reading jobspec")
	}

	fmt.Printf("[GRPCServer] Received Match request %v\n", in)
	reserved, allocated, at, overhead, jobid, fluxerr := fluxcli.ReapiCliMatchAllocate(s.fctx, false, string(spec))
	printOutput(reserved, allocated, at, overhead, jobid, fluxerr)

	fmt.Printf("[MatchRPC] Errors so far: %s\n", fluxcli.ReapiCliGetErrMsg(s.fctx))
	if fluxerr != 0 {
		return nil, errors.New("Error in ReapiCliMatchAllocate")
	}

	if allocated == "" {
		return nil, nil
	}

	// printOutput(reserved, allocated, at, overhead, jobid, fluxerr)

	nodetasks := utils.ParseAllocResult(allocated)
	
	nodetaskslist := make([]*pb.NodeAlloc, len(nodetasks))
	for i, result := range nodetasks {
		nodetaskslist[i] = &pb.NodeAlloc {
			NodeID: result.Basename,
			Tasks: int32(result.CoreCount)/in.Ps.Cpu,
		}
	}
	mr := &pb.MatchResponse{PodID: in.Ps.Id, Nodelist: nodetaskslist, JobID: int64(jobid)}
	fmt.Printf("[GRPCServer] Response %v \n", mr)
	return mr, nil
}

////// Utility functions
func printOutput(reserved bool, allocated string, at int64, overhead float64, jobid uint64, fluxerr int) {
	fmt.Println("\n\t----Match Allocate output---")
	fmt.Printf("jobid: %d\nreserved: %t\nallocated: %s\nat: %d\noverhead: %f\nerror: %d\n", jobid, reserved, allocated, at, overhead, fluxerr)
}