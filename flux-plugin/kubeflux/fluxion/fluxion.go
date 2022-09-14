package fluxion

import (
	"fmt"
	"io/ioutil"
	"github.com/cmisale/flux-sched/resource/hlapi/bindings/go/src/fluxcli"
	"kubeflux/utils" 
	"kubeflux/jobspec"
	"errors"
)

type Fluxion struct {
	fctx 		*fluxcli.ReapiCtx
	Policy 		string
}

func (f *Fluxion) InitFluxion() {
	f.fctx = fluxcli.NewReapiCli()


	f.fctx = fluxcli.NewReapiCli()
	fmt.Println("Created cli context ", f.fctx)
	fmt.Printf("%+v\n", f.fctx)
	filename := "/kubecluster.json"
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
	if f.Policy != "" {
		p = string("{\"matcher_policy\": \"" + f.Policy + "\"}")
		fmt.Println("Match policy: ", p)
	} 
	
	fluxcli.ReapiCliInit(f.fctx, string(jgf), p)

}

func (f *Fluxion) Cancel(in *utils.CancelRequest) (*utils.CancelResponse, error) {
	fmt.Printf("[GRPCServer] Received Cancel request %v\n", in)
	err := fluxcli.ReapiCliCancel(f.fctx, int64(in.JobID), true)
	if err < 0 {
		return nil, errors.New("Error in Cancel")
	}

	dr := &utils.CancelResponse{JobID: in.JobID, Error: int32(err)}
	fmt.Printf("[GRPCServer] Sending Cancel response %v\n", dr)

	fmt.Printf("[CancelRPC] Errors so far: %s\n", fluxcli.ReapiCliGetErrMsg(f.fctx))
	
	//func ReapiCliInfo(ctx *ReapiCtx, jobid int64) (reserved bool, at int64, overhead float64, mode string, err int)
	reserved, at, overhead, mode, fluxerr := fluxcli.ReapiCliInfo(f.fctx, int64(in.JobID))

	fmt.Println("\n\t----Job Info output---")
	fmt.Printf("jobid: %d\nreserved: %t\nat: %d\noverhead: %f\nmode: %s\nerror: %d\n", in.JobID, reserved, at, overhead, mode, fluxerr)

	fmt.Printf("[GRPCServer] Sending Cancel response %v\n", dr)
	return dr, nil
}

func (f *Fluxion) Match(in *utils.MatchRequest) (*utils.MatchResponse, error) {
	// currenttime := time.Now()
	// filename := fmt.Sprintf("/home/data/jobspecs/jobspec-%s-%s.yaml", currenttime.Format(time.RFC3339Nano), in.Ps.Id)
	filename := "/jobspec.yaml"
	jobspec.CreateJobSpecYaml(&in.Ps, in.Count, filename)

	spec, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.New("Error reading jobspec")
	}

	fmt.Printf("[GRPCServer] Received Match request %v\n", in)
	reserved, allocated, at, overhead, jobid, fluxerr := fluxcli.ReapiCliMatchAllocate(f.fctx, false, string(spec))
	printOutput(reserved, allocated, at, overhead, jobid, fluxerr)

	fmt.Printf("[MatchRPC] Errors so far: %s\n", fluxcli.ReapiCliGetErrMsg(f.fctx))
	if fluxerr != 0 {
		return nil, errors.New("Error in ReapiCliMatchAllocate")
	}

	if allocated == "" {
		return nil, nil
	}

	// printOutput(reserved, allocated, at, overhead, jobid, fluxerr)

	nodetasks := utils.ParseAllocResult(allocated)
	
	nodetaskslist := make([]*utils.NodeAlloc, len(nodetasks))
	for i, result := range nodetasks {
		nodetaskslist[i] = &utils.NodeAlloc {
			NodeID: result.Basename,
			Tasks: int32(result.CoreCount)/in.Ps.Cpu,
		}
	}
	mr := &utils.MatchResponse{PodID: in.Ps.Id, Nodelist: nodetaskslist, JobID: int64(jobid)}
	fmt.Printf("[GRPCServer] Response %v \n", mr)
	return mr, nil
}

////// Utility functions
func printOutput(reserved bool, allocated string, at int64, overhead float64, jobid uint64, fluxerr int) {
	fmt.Println("\n\t----Match Allocate output---")
	fmt.Printf("jobid: %d\nreserved: %t\nallocated: %s\nat: %d\noverhead: %f\nerror: %d\n", jobid, reserved, allocated, at, overhead, fluxerr)
}