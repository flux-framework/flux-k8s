CLONE_UPSTREAM ?= ./upstream
UPSTREAM ?= https://github.com/kubernetes-sigs/scheduler-plugins
BASH ?= /bin/bash
DOCKER ?= docker
TAG ?= latest

# These are passed to build the sidecar
REGISTRY ?= ghcr.io/flux-framework
SIDECAR_IMAGE ?= fluence-sidecar:latest
CONTROLLER_IMAGE ?= fluence-controller
SCHEDULER_IMAGE ?= fluence

.PHONY: all build build-sidecar clone update push push-sidecar push-controller

all: prepare build-sidecar build

build-sidecar: 
	make -C ./src LOCAL_REGISTRY=${REGISTRY} LOCAL_IMAGE=${SIDECAR_IMAGE}

clone:
	if [ -d "$(CLONE_UPSTREAM)" ]; then echo "Upstream is cloned"; else git clone $(UPSTREAM) ./$(CLONE_UPSTREAM); fi

update: clone
	git -C $(CLONE_UPSTREAM) pull origin master

prepare: clone
	# These are entirely new directory structures
	rm -rf $(CLONE_UPSTREAM)/pkg/fluence
	rm -rf $(CLONE_UPSTREAM)/manifests/fluence
	cp -R sig-scheduler-plugins/pkg/fluence $(CLONE_UPSTREAM)/pkg/fluence
	# This is the one exception not from sig-scheduler-plugins because it is needed in both spots
	cp -R src/fluence/fluxcli-grpc $(CLONE_UPSTREAM)/pkg/fluence/fluxcli-grpc
	# These are files with subtle changes to add fluence
	cp sig-scheduler-plugins/cmd/scheduler/main.go ./upstream/cmd/scheduler/main.go
	cp sig-scheduler-plugins/manifests/install/charts/as-a-second-scheduler/templates/deployment.yaml $(CLONE_UPSTREAM)/manifests/install/charts/as-a-second-scheduler/templates/deployment.yaml
	cp sig-scheduler-plugins/manifests/install/charts/as-a-second-scheduler/values.yaml $(CLONE_UPSTREAM)/manifests/install/charts/as-a-second-scheduler/values.yaml

build: prepare
	REGISTRY=${REGISTRY} IMAGE=${SCHEDULER_IMAGE} CONTROLLER_IMAGE=${CONTROLLER_IMAGE} $(BASH) $(CLONE_UPSTREAM)/hack/build-images.sh

push-sidecar:
	$(DOCKER) push $(REGISTRY)/$(SIDECAR_IMAGE):$(TAG) --all-tags

push-controller:
	$(DOCKER) push $(REGISTRY)/$(CONTROLLER_IMAGE):$(TAG) --all-tags

push: push-sidecar push-controller
