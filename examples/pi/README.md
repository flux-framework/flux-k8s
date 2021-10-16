# A Minimum Working Example for Kubeflux

- [source](https://stackoverflow.com/questions/42564058/how-to-use-local-docker-images-with-minikube) 
- [Pi](https://github.com/ArangoGutierrez/Pi)
- [Multi-nodes solution](https://minikube.sigs.k8s.io/docs/handbook/registry/)
- [reference](https://hasura.io/blog/sharing-a-local-registry-for-minikube-37c7240d0615/)
## Setup
    - local cluster: minikube
    - Kubeflux: `kubesched.yaml`

## Using local docker image in minikube (multi-node can not use this method)

```bash
# Start minikube
minikube start --kuberbetes-version=1.19.8

# Set docker env
eval $(minikube docker-env)             # unix shells
minikube docker-env | Invoke-Expression # PowerShell

# Build image
docker build -t pi:latest .

# Run in minikube
kubectl run hello-foo --image=pi:latest --restart=Never --image-pull-policy=Never

# Check that it's running
kubectl get pods

eval $(minikube docker-env -u)
```

## Pushing to an in-cluster using Registry addon

- [sources](https://minikube.sigs.k8s.io/docs/handbook/pushing/)
- [refer](https://github.com/kubernetes/minikube/issues/6012)

## Steps:

```bash
   # we can find plugin version by reading from logs command's output and find corresponding k8s version from https://github.com/cmisale/scheduler-plugins 
   $ minikube start --kubernetes-version=v1.19.8
   $ kubectl create -f rbac.yaml 
   $ kubectl create -f ./kubesched.yaml 
   # find correct pod name with command minikube kubectl -- get pods -A
   $ kubectl logs -n kube-system  kubeflux-plugin-57c56dd87f-9c5km
   $ kubectl create -f pi.yaml

   $ POD=$(kubectl get pod -l app=kubeflux -n kube-system -o jsonpath="{.items[0].metadata.name}")
   $ kubectl exec -it <podname> -n kube-system -- /bin/bash

   $ kubectl exec -it $(kubectl get pod -l app=kubeflux -n kube-system -o jsonpath="{.items[0].metadata.name}") -n kube-system -- /bin/bash

   $ kubectl cp kube-system/$(kubectl get pod -l app=kubeflux -n kube-system -o jsonpath="{.items[0].metadata.name}"):/home/data/jgf/kubecluster.json  kubecluster_cp.json
```

### How to copy file from pods to local

 - [refer](https://stackoverflow.com/questions/52407277/how-to-copy-files-from-kubernetes-pods-to-local-system)

 ```bash
    kubectl cp kube-system/kubeflux-plugin-7b5fb86f5f-8wqq2:/home/data/jgf/kubecluster.json  kubecluster_before.json
 ```

 ### On CPU limit

 - [refer](https://learnk8s.io/setting-cpu-memory-limits-requests)


 ### Resource query

 - [refer](https://flux-framework.readthedocs.io/projects/flux-rfc/en/latest/spec_4.html)

 ### Kind local registry

 - [refer](https://kind.sigs.k8s.io/docs/user/local-registry/)


 ### Kind cluster with metrics server

 - [refer](https://www.scmgalaxy.com/tutorials/kubernetes-metrics-server-error-readiness-probe-failed-http-probe-failed-with-statuscode/)