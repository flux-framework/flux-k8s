GITVERSION := $(shell git rev-parse --short HEAD)

DOCKER_REGISTRY=quay.io/${USER}
DOCKER_LABEL=kubeflux-gromacs
DOCKER_TAG=${GITVERSION}

DOCKER_IMAGE=${DOCKER_REGISTRY}/${DOCKER_LABEL}:${DOCKER_TAG}

all:: build-image
all:: build-manifest

clean:: clean-image
clean:: clean-manifest

build-manifest:: gromacs-mpijob-benchmark.yaml
clean-manifest:: clean-gromacs-mpijob-benchmark.yaml

build-image:
	@echo "Building Docker image..."
	docker build -t ${DOCKER_IMAGE} . 
	docker tag ${DOCKER_IMAGE} ${DOCKER_REGISTRY}/${DOCKER_LABEL}:latest

gromacs-%.yaml: gromacs-%.template.yaml
	@echo "Building Kubernetes manifest..."
	sed \
	-e s/%DOCKER_IMAGE%/$(subst /,\\/,${DOCKER_IMAGE})/g \
	-e s/%JOBLABEL%/${USER}/g \
	$< > $@

publish:
	docker push ${DOCKER_IMAGE}
	docker push ${DOCKER_REGISTRY}/${DOCKER_LABEL}:latest

clean-image:
	docker rmi -f ${DOCKER_IMAGE} ${DOCKER_REGISTRY}/${DOCKER_LABEL}:latest

clean-%.yaml:
	rm -f $*.yaml
