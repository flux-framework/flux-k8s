#!/bin/bash

source conf.env

SCHEDULER_MANIFEST=$(mktemp)

echo Running default scheduler
./launch.sh default default ${KFGROMACS_RESULTS_PATH}

echo Running KubeFlux scheduler
sed s/%TAG%/latest/g  kubesched.template.yaml > ${SCHEDULER_MANIFEST}
./launch flux flux ${KFGROMACS_RESULTS_PATH} ${SCHEDULER_MANIFEST}

echo Running KubeFlux scheduler with subnet awareness
sed s/%TAG%/sa/g  kubesched.template.yaml > ${SCHEDULER_MANIFEST}
./launch flux flux-sa ${KFGROMACS_RESULTS_PATH} ${SCHEDULER_MANIFEST}

rm ${SCHEDULER_MANIFEST}
