GITVERSION := $(shell git rev-parse --short HEAD)

DOCKER_IMAGE_NAME=pi
DOCKER_IMAGE_TAG=${GITVERSION}

DOCKER_IMAGE=${DOCKER_IMAGE_NAME}:${DOCKER_IMAGE_TAG}

DOCKER_SEGFAULT_IMAGE_NAME=pi-segfault
DOCKER_SEGFAULT_IMAGE=${DOCKER_SEGFAULT_IMAGE_NAME}:${DOCKER_IMAGE_TAG}

all:: build-image

clean:: clean-image

build-image:
	@echo "Building Docker image...";
	docker build -t ${DOCKER_IMAGE_NAME}:${DOCKER_IMAGE_TAG} .
	docker tag ${DOCKER_IMAGE_NAME}:${DOCKER_IMAGE_TAG} ${DOCKER_IMAGE_NAME}:latest
	docker tag ${DOCKER_IMAGE_NAME}:${DOCKER_IMAGE_TAG} localhost:5000/${DOCKER_IMAGE_NAME}:latest

build-segfault-image:
	@echo "Building Docker image (pi-segfault)...";
	docker build -t ${DOCKER_SEGFAULT_IMAGE_NAME}:${DOCKER_IMAGE_TAG} -f Dockerfile.segfault .
	docker tag ${DOCKER_SEGFAULT_IMAGE_NAME}:${DOCKER_IMAGE_TAG} ${DOCKER_SEGFAULT_IMAGE_NAME}:latest
	docker tag ${DOCKER_SEGFAULT_IMAGE_NAME}:${DOCKER_IMAGE_TAG} localhost:5000/${DOCKER_SEGFAULT_IMAGE_NAME}:latest

push-image-local:
	@echo "Pushing Docker image..."; 
	docker push localhost:5000/${DOCKER_IMAGE_NAME}:latest

push-segfault-image-local:
	@echo "Pushing Docker image (pi-segfault)..."; 
	docker push localhost:5000/${DOCKER_SEGFAULT_IMAGE_NAME}:latest

push-image:
	@echo "Pushing Docker image..."; 
	docker push ${DOCKER_IMAGE_NAME}:latest

clean-image:
	@echo "Cleaning Docker image...";
	docker rmi -f ${DOCKER_IMAGE_NAME}:${DOCKER_IMAGE_TAG} ${DOCKER_IMAGE_NAME}:latest localhost:5000/${DOCKER_IMAGE_NAME}:latest

setup-kubeflux:
	kubectl create -f ./rbac.yaml
	kubectl create -f ./kubesched.yaml

reset-kubeflux:
	kubectl delete -f ./kubesched.yaml
	kubectl create -f ./kubesched.yaml

check-kubeflux:
	kubectl get pod -l app=kubeflux -n kube-system

delete-kind:
	kind delete cluster --name xyl-kind

