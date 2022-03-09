package main

// #include "reapi_cli.h"
import "C"

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"time"

	pb "github.com/flux-framework/flux-k8s/pkg/fluxcli-grpc"
	"github.com/flux-framework/flux-k8s/pkg/jobspec"
	"github.com/flux-framework/flux-k8s/pkg/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

const (
	port = ":4242"
)

type (
	ReapiCtx C.struct_reapi_cli_ctx_t
)

const SockAddr = "/tmp/echo.sock"

var responsechan chan string

/*! Create and initialize reapi_cli context
reapi_cli_ctx_t *reapi_cli_new ();
*/

func NewReapiCli() *ReapiCtx {
	return (*ReapiCtx)(C.reapi_cli_new())
}

// func ReapiCliGetNode(ctx *ReapiCtx) string {
// 	return C.GoString(C.reapi_cli_get_node((*C.struct_reapi_cli_ctx)(ctx)))
// }

/*! Destroy reapi cli context
*
* \param ctx           reapi_cli_ctx_t context object
void reapi_cli_destroy (reapi_cli_ctx_t *ctx);
*/

func ReapiCliDestroy(ctx *ReapiCtx) {
	C.reapi_cli_destroy((*C.struct_reapi_cli_ctx)(ctx))
}

/* int reapi_cli_initialize (reapi_cli_ctx_t *ctx, const char *jgf); */

func ReapiCliInit(ctx *ReapiCtx, jgf string, options string) (err int) {
	err = (int)(C.reapi_cli_initialize((*C.struct_reapi_cli_ctx)(ctx),
		C.CString(jgf), (C.CString(options))))
	return err
}

/*! Match a jobspec to the "best" resources and either allocate
*  orelse reserve them. The best resources are determined by
*  the selected match policy.
*
*  \param ctx       reapi_cli_ctx_t context object
*  \param orelse_reserve
*                   Boolean: if false, only allocate; otherwise, first try
*                   to allocate and if that fails, reserve.
*  \param jobspec   jobspec string.
*  \param jobid     jobid of the uint64_t type.
*  \param reserved  Boolean into which to return true if this job has been
*                   reserved instead of allocated.
*  \param R         String into which to return the resource set either
*                   allocated or reserved.
*  \param at        If allocated, 0 is returned; if reserved, actual time
*                   at which the job is reserved.
*  \param ov        Double into which to return performance overhead
*                   in terms of elapse time needed to complete
*                   the match operation.
*  \return          0 on success; -1 on error.
int reapi_module_match_allocate (reapi_module_ctx_t *ctx, bool orelse_reserve,
  const char *jobspec, const uint64_t jobid,
  bool *reserved,
  char **R, int64_t *at, double *ov);
*/

func ReapiCliMatchAllocate(ctx *ReapiCtx, orelse_reserve bool,
	jobspec string) (reserved bool, allocated string, at int64, overhead float64, jobid uint64, err int) {
	var r = C.CString("")
	err = (int)(C.reapi_cli_match_allocate((*C.struct_reapi_cli_ctx)(ctx),
		(C.bool)(orelse_reserve),
		C.CString(jobspec),
		(*C.ulong)(&jobid),
		(*C.bool)(&reserved),
		&r,
		(*C.long)(&at),
		(*C.double)(&overhead)))
	allocated = C.GoString(r)
	return reserved, allocated, at, overhead, jobid, err

}

/*! Update the resource state with R.
 *
 *  \param ctx       reapi_cli_ctx_t context object
 *  \param jobid     jobid of the uint64_t type.
 *  \param R         R string
 *  \param at        return the scheduled time
 *  \param ov        return the performance overhead
 *                   in terms of elapse time needed to complete
 *                   the match operation.
 *  \param R_out     return the updated R string.
 *  \return          0 on success; -1 on error.
 *
 int reapi_cli_update_allocate (reapi_cli_ctx_t *ctx,
	const uint64_t jobid, const char *R, int64_t *at,
	double *ov, const char **R_out);
*/

func ReapiCliUpdateAllocate(ctx *ReapiCtx, jobid int, r string) (at int64, overhead float64, r_out string, err int) {
	var tmp_rout = C.CString("")
	err = (int)(C.reapi_cli_update_allocate((*C.struct_reapi_cli_ctx)(ctx),
		(C.ulong)(jobid),
		C.CString(r),
		(*C.long)(&at),
		(*C.double)(&overhead),
		&tmp_rout))
	r_out = C.GoString(tmp_rout)
	return at, overhead, r_out, err
}

/*! Cancel the allocation or reservation corresponding to jobid.
 *
 *  \param ctx       reapi_cli_ctx_t context object
 *  \param jobid     jobid of the uint64_t type.
 *  \param noent_ok  don't return an error on nonexistent jobid
 *  \return          0 on success; -1 on error.

 int reapi_cli_cancel (reapi_cli_ctx_t *ctx,
	const uint64_t jobid, bool noent_ok);*/

func ReapiCliCancel(ctx *ReapiCtx, jobid int64, noent_ok bool) (err int) {
	err = (int)(C.reapi_cli_cancel((*C.struct_reapi_cli_ctx)(ctx),
		(C.ulong)(jobid),
		(C.bool)(noent_ok)))
	return err
}

/*! Get the information on the allocation or reservation corresponding
 *  to jobid.
 *
 *  \param ctx       reapi_cli_ctx_t context object
 *  \param jobid     const jobid of the uint64_t type.
 *  \param reserved  Boolean into which to return true if this job has been
 *                   reserved instead of allocated.
 *  \param at        If allocated, 0 is returned; if reserved, actual time
 *                   at which the job is reserved.
 *  \param ov        Double into which to return performance overhead
 *                   in terms of elapse time needed to complete
 *                   the match operation.
 *  \return          0 on success; -1 on error.

 int reapi_cli_info (reapi_cli_ctx_t *ctx, const uint64_t jobid,
	bool *reserved, int64_t *at, double *ov);
*/

func ReapiCliInfo(ctx *ReapiCtx, jobid int64) (reserved bool, at int64, overhead float64, mode string, err int) {
	var tmp_mode = C.CString("")
	err = (int)(C.reapi_cli_info((*C.struct_reapi_cli_ctx)(ctx),
		(C.ulong)(jobid),
		(&tmp_mode),
		(*C.bool)(&reserved),
		(*C.long)(&at),
		(*C.double)(&overhead)))
	return reserved, at, overhead, C.GoString(tmp_mode), err
}

/*! Get the performance information about the resource infrastructure.
 *
 *  \param ctx       reapi_cli_ctx_t context object
 *  \param V         Number of resource vertices
 *  \param E         Number of edges
 *  \param J         Number of jobs
 *  \param load      Graph load time
 *  \param min       Min match time
 *  \param max       Max match time
 *  \param avg       Avg match time
 *  \return          0 on success; -1 on error.

 int reapi_cli_stat (reapi_cli_ctx_t *ctx, int64_t *V, int64_t *E,
	int64_t *J, double *load,
	double *min, double *max, double *avg);
*/
func ReapiCliStat(ctx *ReapiCtx) (v int64, e int64,
	jobs int64, load float64, min float64, max float64, avg float64, err int) {
	err = (int)(C.reapi_cli_stat((*C.struct_reapi_cli_ctx)(ctx),
		(*C.long)(&v),
		(*C.long)(&e),
		(*C.long)(&jobs),
		(*C.double)(&load),
		(*C.double)(&min),
		(*C.double)(&max),
		(*C.double)(&avg)))
	return v, e, jobs, load, min, max, avg, err
}

func ReapiCliGetErrMsg(ctx *ReapiCtx) string {
	errmsg := C.reapi_cli_get_err_msg((*C.struct_reapi_cli_ctx)(ctx))
	return C.GoString(errmsg)
}

func ReapiCliClearErrMsg(ctx *ReapiCtx) {
	C.reapi_cli_clear_err_msg((*C.struct_reapi_cli_ctx)(ctx))
}

/*! Set the opaque handle to the reapi cli context.
*
*  \param ctx       reapi_cli_ctx_t context object
*  \param h         Opaque handle. How it is used is an implementation
*                   detail. However, when it is used within a Flux's
*                   service cli, it is expected to be a pointer
*                   to a flux_t object.
*  \return          0 on success; -1 on error.
*
int reapi_cli_set_handle (reapi_cli_ctx_t *ctx, void *handle);
*/

func ReapiCliSetHandle(ctx *ReapiCtx) int {
	return -1
}

/*! Set the opaque handle to the reapi cli context.
 *
 *  \param ctx       reapi_cli_ctx_t context object
 *  \return          handle
 *
void *reapi_cli_get_handle (reapi_cli_ctx_t *ctx);
*/
func ReapiCliGetHandle(ctx *ReapiCtx) int {
	return -1
}

// server is used to implement grpc_rq.FluxcliServiceServer.
type server struct {
	fctx *ReapiCtx
	pb.UnimplementedFluxcliServiceServer
}

func (s *server) Cancel(ctx context.Context, in *pb.CancelRequest) (*pb.CancelResponse, error) {
	fmt.Printf("[GRPCServer] Received Cancel request %v\n", in)
	err := ReapiCliCancel(s.fctx, int64(in.JobID), false)
	if err < 0 {
		return nil, errors.New("Error in Cancel")
	}

	dr := &pb.CancelResponse{JobID: in.JobID, Error: int32(err)}
	fmt.Printf("[GRPCServer] Sending Cancel response %v\n", dr)

	return dr, nil
}

func (s *server) Match(ctx context.Context, in *pb.MatchRequest) (*pb.MatchResponse, error) {

	filename := "/home/data/jobspecs/jobspec.yaml"
	jobspec.CreateJobSpecYaml(in.Ps, in.Count, filename)

	spec, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.New("Error reading jobspec")
	}

	fmt.Printf("[GRPCServer] Received Match request %v\n", in)
	reserved, allocated, at, overhead, jobid, fluxerr := ReapiCliMatchAllocate(s.fctx, false, string(spec))
	fmt.Printf("Errors so far: %s\n", ReapiCliGetErrMsg(s.fctx))
	if fluxerr != 0 {
		return nil, errors.New("Error in ReapiCliMatchAllocate")
	}

	if allocated == "" {
		return nil, nil
	}

	utils.PrintOutput(reserved, allocated, at, overhead, jobid, fluxerr)

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

func main() {
	fmt.Println("This is the fluxion grpc server")
	policy := flag.String("policy", "", "Match policy")
	flag.Parse()
	fctx := NewReapiCli()
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

	if *policy != "" {
		p := string("{\"matcher_policy\": \"" + *policy + "\"}")
		fmt.Println("Match policy: ", p)
	}

	ReapiCliInit(fctx, string(jgf), "{}")

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
