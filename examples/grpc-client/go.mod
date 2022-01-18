module example.com/fluxclient

go 1.16

require (
	google.golang.org/grpc v1.43.0
	google.golang.org/protobuf v1.27.1
	k8s.io/api v0.23.1
)

replace (
	fluxcli => /go/src/fluxcli
	k8s.io/api => k8s.io/api v0.22.3
)
