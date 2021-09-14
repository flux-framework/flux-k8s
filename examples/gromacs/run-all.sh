#!/bin/bash

SCHEDULER_MANIFEST=$(mktemp)

echo Running default scheduler
./launch.sh default default

echo Running KubeFlux scheduler
sed s/%TAG%/latest/g  kubesched.template.yaml > ${SCHEDULER_MANIFEST}
./launch flux flux ${SCHEDULER_MANIFEST}

echo Running KubeFlux scheduler with subnet awareness
sed s/%TAG%/sa/g  kubesched.template.yaml > ${SCHEDULER_MANIFEST}
./launch flux flux ${SCHEDULER_MANIFEST}

rm ${SCHEDULER_MANIFEST}
