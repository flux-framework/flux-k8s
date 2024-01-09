
FLUX_SCHED_ROOT ?= /home/flux-sched
INSTALL_PREFIX ?= /usr
LOCALBIN ?= $(shell pwd)/bin
COMMONENVVAR=GOOS=$(shell uname -s | tr A-Z a-z)

# This is what worked
# GOOS=linux CGO_CFLAGS="-I/home/flux-sched/resource/reapi/bindings/c" CGO_LDFLAGS="-L/usr/lib -L/home/flux-sched/resource -lresource -L/home/flux-sched/resource/libjobspec -ljobspec_conv -L/home/flux-sched/resource/reapi/bindings -lreapi_cli -lflux-idset -lstdc++ -lczmq -ljansson -lhwloc -lboost_system -lflux-hostlist -lboost_graph -lyaml-cpp" go build -ldflags '-w' -o bin/server cmd/main.go
BUILDENVVAR=CGO_CFLAGS="-I${FLUX_SCHED_ROOT}/resource/reapi/bindings/c" CGO_LDFLAGS="-L${INSTALL_PREFIX}/lib -L${FLUX_SCHED_ROOT}/resource -lresource -L${FLUX_SCHED_ROOT}/resource/libjobspec -ljobspec_conv -L/${FLUX_SCHED_ROOT}/resource/reapi/bindings -lreapi_cli -lflux-idset -lstdc++ -lczmq -ljansson -lhwloc -lboost_system -lflux-hostlist -lboost_graph -lyaml-cpp"

LOCAL_REGISTRY=localhost:5000
LOCAL_IMAGE=fluence-sidecar:latest
RELEASE_VERSION?=v$(shell date +%Y%m%d)-$(shell git describe --tags --match "v*")

.PHONY: all
all: fluxcli

.PHONY: $(LOCALBIN)
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

.PHONY: fluxcli
fluxcli: 
	docker build -f build/scheduler/Dockerfile --build-arg ARCH="amd64" --build-arg RELEASE_VERSION="$(RELEASE_VERSION)" -t $(LOCAL_REGISTRY)/$(LOCAL_IMAGE) .

.PHONY: server
server: 
	$(COMMONENVVAR) $(BUILDENVVAR) go build -ldflags '-w' -o bin/server cmd/main.go

.PHONY: protoc
protoc: $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
	GOBIN=$(LOCALBIN) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2
	
# You can use make protoc to download proto
.PHONY: proto
proto: protoc
	PATH=$(LOCALBIN):${PATH} protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative fluence/fluxcli-grpc/fluxcli.proto
