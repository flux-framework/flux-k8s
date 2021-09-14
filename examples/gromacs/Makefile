GITVERSION := $(shell git rev-parse --short HEAD)

DOCKER_IMAGE_NAME=quay.io/${USER}/kubeflux-gromacs
DOCKER_IMAGE_TAG=${GITVERSION}

DOCKER_IMAGE=${DOCKER_IMAGE_NAME}:${DOCKER_IMAGE_TAG}

all:: build-image
all:: build-manifest

clean:: clean-image
clean:: clean-manifest

build-manifest:: gromacs-mpijob-benchmark.yaml
clean-manifest:: clean-gromacs-mpijob-benchmark.yaml

build-image:
	@echo "Building Docker image..."
	docker build -t ${DOCKER_IMAGE_NAME}:${DOCKER_IMAGE_TAG} .
	docker tag ${DOCKER_IMAGE_NAME}:${DOCKER_IMAGE_TAG} ${DOCKER_IMAGE_NAME}:latest

gromacs-%.yaml: gromacs-%.template.yaml
	@echo "Building Kubernetes manifest..."
	sed \
	-e s/%DOCKER_IMAGE%/$(subst /,\\/,${DOCKER_IMAGE})/g \
	-e s/%JOBLABEL%/${USER}/g \
	$< > $@

publish:
	docker push ${DOCKER_IMAGE_NAME}:${DOCKER_IMAGE_TAG}
	docker push ${DOCKER_IMAGE_NAME}:latest

clean-image:
	docker rmi -f ${DOCKER_IMAGE_NAME}:${DOCKER_IMAGE_TAG} ${DOCKER_IMAGE_NAME}:latest

clean-%.yaml:
	rm -f $*.yaml
