package fluxion

import (
	"os"

	pb "github.com/flux-framework/flux-k8s/flux-plugin/fluence/fluxcli-grpc"
	"github.com/flux-framework/flux-k8s/flux-plugin/fluence/jobspec"
	"github.com/flux-framework/flux-k8s/flux-plugin/fluence/utils"
	"github.com/flux-framework/flux-sched/resource/reapi/bindings/go/src/fluxcli"

	"context"
	"errors"
	"fmt"
)

type Fluxion struct {
	cli *fluxcli.ReapiClient
	pb.UnimplementedFluxcliServiceServer
}

func (f *Fluxion) InitFluxion(policy *string, label *string) {
	f.cli = fluxcli.NewReapiClient()

	fmt.Println("Created flux resource client ", f.cli)
	fmt.Printf("%+v\n", f.cli)
	filename := "/home/data/jgf/kubecluster.json"
	err := utils.CreateJGF(filename, label)
	if err != nil {
		return
	}

	jgf, err := os.ReadFile(filename)
	if err != nil {
		fmt.Println("Error reading JGF")
		return
	}

	p := "{}"
	if *policy != "" {
		p = string("{\"matcher_policy\": \"" + *policy + "\"}")
		fmt.Println("Match policy: ", p)
	}

	f.cli.InitContext(string(jgf), p)
}

func (s *Fluxion) Cancel(ctx context.Context, in *pb.CancelRequest) (*pb.CancelResponse, error) {
	fmt.Printf("[GRPCServer] Received Cancel request %v\n", in)
	err := s.cli.Cancel(int64(in.JobID), true)
	if err != nil {
		return nil, errors.New("Error in Cancel")
	}

	// Why would we have an error code here if we check above?
	// This (I think) should be an error code for the specific job
	dr := &pb.CancelResponse{JobID: in.JobID}
	fmt.Printf("[GRPCServer] Sending Cancel response %v\n", dr)

	fmt.Printf("[CancelRPC] Errors so far: %s\n", s.cli.GetErrMsg())

	reserved, at, overhead, mode, fluxerr := s.cli.Info(int64(in.JobID))
	fmt.Println("\n\t----Job Info output---")
	fmt.Printf("jobid: %d\nreserved: %t\nat: %d\noverhead: %f\nmode: %s\nerror: %d\n", in.JobID, reserved, at, overhead, mode, fluxerr)

	fmt.Printf("[GRPCServer] Sending Cancel response %v\n", dr)
	return dr, nil
}

func (s *Fluxion) Match(ctx context.Context, in *pb.MatchRequest) (*pb.MatchResponse, error) {
	filename := "/home/data/jobspecs/jobspec.yaml"
	jobspec.CreateJobSpecYaml(in.Ps, in.Count, filename)

	spec, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.New("Error reading jobspec")
	}

	fmt.Printf("[GRPCServer] Received Match request %v\n", in)
	reserved, allocated, at, overhead, jobid, fluxerr := s.cli.MatchAllocate(false, string(spec))
	utils.PrintOutput(reserved, allocated, at, overhead, jobid, fluxerr)

	fmt.Printf("[MatchRPC] Errors so far: %s\n", s.cli.GetErrMsg())
	if fluxerr != nil {
		return nil, errors.New("Error in ReapiCliMatchAllocate")
	}

	if allocated == "" {
		return nil, nil
	}

	nodetasks := utils.ParseAllocResult(allocated)

	nodetaskslist := make([]*pb.NodeAlloc, len(nodetasks))
	for i, result := range nodetasks {
		nodetaskslist[i] = &pb.NodeAlloc{
			NodeID: result.Basename,
			Tasks:  int32(result.CoreCount) / in.Ps.Cpu,
		}
	}
	mr := &pb.MatchResponse{PodID: in.Ps.Id, Nodelist: nodetaskslist, JobID: int64(jobid)}
	fmt.Printf("[GRPCServer] Response %v \n", mr)
	return mr, nil
}
