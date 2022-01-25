COMMONENVVAR=GOOS=$(shell uname -s | tr A-Z a-z)
BUILDENVVAR=CGO_CFLAGS="-I/root/flux-sched/resource/hlapi/bindings/c -I/root/flux-install/include" CGO_LDFLAGS="-L/root/flux-sched/resource/hlapi/bindings/c/.libs -lreapi_cli  -L/root/flux-sched/resource/.libs -lresource -lstdc++ -lczmq -ljansson -lhwloc -lboost_system -L/root/flux-install/lib -lflux-hostlist -lboost_graph -lyaml-cpp"

LOCAL_REGISTRY=localhost:5000
LOCAL_IMAGE=kubeflux-sidecar:latest
RELEASE_VERSION?=v$(shell date +%Y%m%d)-$(shell git describe --tags --match "v*")

.PHONY: all
all: fluxcli

.PHONY: fluxcli
fluxcli: 
	docker build -f build/scheduler/Dockerfile.flux --build-arg ARCH="amd64" --build-arg RELEASE_VERSION="$(RELEASE_VERSION)" -t $(LOCAL_REGISTRY)/$(LOCAL_IMAGE) .


.PHONY: server
server: 
	$(COMMONENVVAR) $(BUILDENVVAR) go build -ldflags '-w' -o bin/server cmd/main.go

.PHONY: proto
proto:
	protoc --go_out=. --go_opt=paths=source_relative     --go-grpc_out=. --go-grpc_opt=paths=source_relative     kubeflux/fluxcli-grpc/fluxcli.proto
