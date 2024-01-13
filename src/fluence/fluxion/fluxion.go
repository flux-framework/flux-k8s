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

// InitFluxion creates a new client to interaction with the fluxion API (via go bindings)
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

// Cancel wraps the Cancel function of the fluxion go bindings
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

// generateJobSpec generates a jobspec for a match request and returns the string
func (s *Fluxion) generateJobspec(in *pb.MatchRequest) ([]byte, error) {

	spec := []byte{}

	// Create a temporary file to write and read the jobspec
	// The first parameter here as the empty string creates in /tmp
	file, err := os.CreateTemp("", "jobspec.*.yaml")
	if err != nil {
		return spec, err
	}
	defer os.Remove(file.Name())
	jobspec.CreateJobSpecYaml(in.Ps, in.Count, file.Name())

	spec, err = os.ReadFile(file.Name())
	if err != nil {
		return spec, errors.New("Error reading jobspec")
	}
	return spec, err
}

// Match wraps the MatchAllocate function of the fluxion go bindings
// If a match is not possible, we return the error and an empty response
func (s *Fluxion) Match(ctx context.Context, in *pb.MatchRequest) (*pb.MatchResponse, error) {

	emptyResponse := &pb.MatchResponse{}

	// Prepare an empty match response (that can still be serialized)
	fmt.Printf("[GRPCServer] Received Match request %v\n", in)

	// Generate the jobspec, written to temporary file and read as string
	spec, err := s.generateJobspec(in)
	if err != nil {
		return emptyResponse, err
	}

	// Ask flux to match allocate!
	reserved, allocated, at, overhead, jobid, fluxerr := s.cli.MatchAllocate(false, string(spec))
	utils.PrintOutput(reserved, allocated, at, overhead, jobid, fluxerr)
	fmt.Printf("[MatchRPC] Errors so far: %s\n", s.cli.GetErrMsg())
	fmt.Printf("[GRPCServer] Flux err is %w\n", fluxerr)
	if fluxerr != nil {
		return emptyResponse, errors.New("Error in ReapiCliMatchAllocate")
	}

	// This usually means we cannot allocate
	// We need to return an error here otherwise we try to pass an empty string
	// to other RPC endpoints and get back an error.
	if allocated == "" {
		fmt.Println("[GRPCServer] Allocated is empty")
		return emptyResponse, errors.New("allocation was not possible")
	}

	// Pass the spec name in so we can include it in the allocation result
	// This will allow us to inspect the ordering later.
	nodetasks := utils.ParseAllocResult(allocated, in.Ps.Container)
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
