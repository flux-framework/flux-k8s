#!/bin/bash

# Before running this, you should:
# 1. create the kind cluster (needs more than one node, fluence does not scheduler to the control plane)
# 2. Install cert-manager
# 3. Customize the script to point to your registry if you intend to push

REGISTRY="${1:-ghcr.io/vsoch}"
HERE=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
ROOT=$(dirname ${HERE})

# Go to the script directory
cd ${ROOT}

# These build each of the images. The sidecar is separate from the other two in src/
make REGISTRY=${REGISTRY} SCHEDULER_IMAGE=fluence SIDECAR_IMAGE=fluence-sidecar CONTROLLER_IMAGE=fluence-controller

# This is what it might look like to push
# docker push ghcr.io/vsoch/fluence-sidecar && docker push ghcr.io/vsoch/fluence-controller && docker push ghcr.io/vsoch/fluence:latest

# We load into kind so we don't need to push/pull and use up internet data ;)
kind load docker-image ${REGISTRY}/fluence-sidecar:latest
kind load docker-image ${REGISTRY}/fluence:latest

# And then install using the charts. The pull policy ensures we use the loaded ones
helm uninstall fluence || true
helm install \
  --set scheduler.image=${REGISTRY}/fluence:latest \
  --set scheduler.sidecarPullPolicy=Never \
  --set scheduler.pullPolicy=Never \
#  --set controller.pullPolicy=Never \
#  --set controller.image=${REGISTRY}/fluence-controller:latest \
  --set scheduler.sidecarimage=${REGISTRY}/fluence-sidecar:latest \
        fluence chart/
