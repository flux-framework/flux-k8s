#!/bin/bash

echo Running default scheduler
./launch.sh default default

echo Running KubeFlux scheduler
sed s/%TAG%/latest/g  kubesched.template.yaml > kubesched.yaml
./launch flux flux kubesched.yaml

echo Running KubeFlux scheduler with subnet awareness
sed s/%TAG%/sa/g  kubesched.template.yaml > kubesched.yaml
./launch flux flux kubesched.yaml
