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

.PHONY: all build build-sidecar prepare push push-sidecar push-controller

all: build-sidecar prepare build

build-sidecar: 
	make -C ./src LOCAL_REGISTRY=${REGISTRY} LOCAL_IMAGE=${SIDECAR_IMAGE}

prepare: 
	if [ -d "$(CLONE_UPSTREAM)" ]; then echo "Upstream is cloned"; else git clone $(UPSTREAM) ./$(CLONE_UPSTREAM); fi
	# These are entirely new directory structures
	cp -R sig-scheduler-plugins/pkg/fluence $(CLONE_UPSTREAM)/pkg/fluence
	cp -R sig-scheduler-plugins/manifests/fluence $(CLONE_UPSTREAM)/manifests/fluence
	# These are files with subtle changes to add fluence
	cp sig-scheduler-plugins/cmd/scheduler/main.go ./upstream/cmd/scheduler/main.go
	cp sig-scheduler-plugins/manifests/install/charts/as-a-second-scheduler/templates/deployment.yaml $(CLONE_UPSTREAM)/manifests/install/charts/as-a-second-scheduler/templates/deployment.yaml
	cp sig-scheduler-plugins/manifests/install/charts/as-a-second-scheduler/values.yaml $(CLONE_UPSTREAM)/manifests/install/charts/as-a-second-scheduler/values.yaml

build:
	REGISTRY=${REGISTRY} IMAGE=${SCHEDULER_IMAGE} CONTROLLER_IMAGE=${CONTROLLER_IMAGE} $(BASH) $(CLONE_UPSTREAM)/hack/build-images.sh

push-sidecar:
	$(DOCKER) push $(REGISTRY)/$(SIDECAR_IMAGE):$(TAG) --all-tags

push-controller:
	$(DOCKER) push $(REGISTRY)/$(CONTROLLER_IMAGE):$(TAG) --all-tags

push: push-sidecar push-controller
