#!/bin/sh

kubectl create -f ./pi-job-kubeflux-segfault.yaml
echo "wait 5 seconds"
sleep 5
kubectl create -f ./pi-job-kubeflux.yaml
