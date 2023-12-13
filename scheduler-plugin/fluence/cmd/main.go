package main

import (
	"flag"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"net"
	"time"

	pb "github.com/flux-framework/flux-k8s/flux-plugin/fluence/fluxcli-grpc"
	"github.com/flux-framework/flux-k8s/flux-plugin/fluence/fluxion"
)

const (
	port = ":4242"
)

var responsechan chan string

func main() {
	fmt.Println("This is the fluxion grpc server")
	policy := flag.String("policy", "", "Match policy")
	label := flag.String("label", "", "Label name for fluence dedicated nodes")

	flag.Parse()
	flux := fluxion.Fluxion{}
	flux.InitFluxion(policy, label)

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
	pb.RegisterFluxcliServiceServer(s, &flux /*&server{flux: flux}*/)
	fmt.Printf("[GRPCServer] gRPC Listening on %s\n", lis.Addr().String())
	if err := s.Serve(lis); err != nil {
		fmt.Printf("[GRPCServer] failed to serve: %v\n", err)
	}

	fmt.Printf("[GRPCServer] Exiting\n")
}
