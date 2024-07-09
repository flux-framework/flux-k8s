#!/bin/bash

UPSTREAM_K8S_REPO=${1}
UPSTREAM_K8S=${2}

# Important - the apimachinery runtime package has a change in interface
# that changes (and will break) after this version
echo "git clone --depth 1 ${UPSTREAM_K8S_REPO} ${UPSTREAM_K8S}"
git clone --depth 1 ${UPSTREAM_K8S_REPO} ${UPSTREAM_K8S}

# We need to add our custom module as a staging path
# sed -i '256 a       k8s.io/podgroup-controller => ./staging/src/k8s.io/podgroup-controller' ${UPSTREAM_K8S}/go.mod
# sed for in-place editing (-i) of the file: 'LINE_NUMBER a-ppend TEXT_TO_ADD'

# sed -i '121 a 		    k8s.io/podgroup-controller v0.0.0' ${UPSTREAM_K8S}/go.mod