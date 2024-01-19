package service

import (
	"os"

	"github.com/flux-framework/flux-k8s/flux-plugin/fluence/defaults"
	pb "github.com/flux-framework/flux-k8s/flux-plugin/fluence/service-grpc"

	"k8s.io/klog/v2"

	"context"
)

type ExternalService struct {
	pb.UnimplementedExternalPluginServiceServer
}

// Init is a helper function for any startup stuff, for which now we have none :)
func (f *ExternalService) Init() {
	klog.Infof("[Fluence] Created external service.")
}

// GetGroup gets and returns the group info
// TODO no good way to look up group - we would need to ask Fluxion directly OR put the grpc
// service alongside the scheduler plugin, which seems like a bad design
func (s *ExternalService) GetGroup(ctx context.Context, in *pb.GroupRequest) (*pb.GroupResponse, error) {
	klog.Infof("[Fluence] Calling get group endpoint! %v\n", in)

	// Prepare an empty match response (that can still be serialized)
	emptyResponse := &pb.GroupResponse{}
	return emptyResponse, nil
}

// List group returns existing groups
func (s *ExternalService) ListGroups(ctx context.Context, in *pb.GroupRequest) (*pb.GroupResponse, error) {

	emptyResponse := &pb.GroupResponse{}

	// Prepare an empty match response (that can still be serialized)
	klog.Infof("[Fluence] Calling list groups endpoint! %v\n", in)

	return emptyResponse, nil
}

// GetResources gets the current Kubernetes Json Graph Format JGF
// This should be created on init of the scheduler
func (s *ExternalService) GetResources(ctx context.Context, in *pb.ResourceRequest) (*pb.ResourceResponse, error) {

	emptyResponse := &pb.ResourceResponse{}

	// Prepare an empty match response (that can still be serialized)
	klog.Infof("[Fluence] Calling get resources endpoint! %v\n", in)

	jgf, err := os.ReadFile(defaults.KubernetesJsonGraphFormat)
	if err != nil {
		klog.Error("Error reading JGF")
		return emptyResponse, err
	}
	emptyResponse.Graph = string(jgf)
	return emptyResponse, nil
}
