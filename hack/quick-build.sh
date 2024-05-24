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