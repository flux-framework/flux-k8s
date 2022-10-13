COMMONENVVAR=GOOS=$(shell uname -s | tr A-Z a-z)
BUILDENVVAR=CGO_CFLAGS="-I/home/flux-sched/resource/hlapi/bindings/c -I/home/flux-install/include" CGO_LDFLAGS="-L/home/flux-install/lib -lreapi_cli  -L/home/flux-install/lib -lresource -L/home/flux-install/lib -lflux-idset -lstdc++ -lczmq -ljansson -lhwloc -lboost_system -L/home/flux-install/lib -lflux-hostlist -lboost_graph -lyaml-cpp"

LOCAL_REGISTRY=localhost:5000
LOCAL_IMAGE=fluence-sidecar:latest
RELEASE_VERSION?=v$(shell date +%Y%m%d)-$(shell git describe --tags --match "v*")

.PHONY: all
all: fluxcli

.PHONY: fluxcli
fluxcli: 
	docker build --no-cache -f build/scheduler/Dockerfile --build-arg ARCH="amd64" --build-arg RELEASE_VERSION="$(RELEASE_VERSION)" -t $(LOCAL_REGISTRY)/$(LOCAL_IMAGE) .


.PHONY: server
server: 
	$(COMMONENVVAR) $(BUILDENVVAR) go build -ldflags '-w' -o bin/server cmd/main.go

.PHONY: proto
proto:
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative fluence/fluxcli-grpc/fluxcli.proto
