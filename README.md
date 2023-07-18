# Fluence

![docs/images/fluence.png](docs/images/fluence.png)

Fluence enables HPC-grade pod scheduling in Kubernetes via the [Kubernetes Scheduling Framework](https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/). Fluence uses the directed-graph based [Fluxion scheduler](https://github.com/flux-framework/flux-sched) to map pods or [podgroups](https://github.com/kubernetes-sigs/scheduler-plugins/tree/master/pkg/coscheduling) to nodes. Fluence supports all the Fluxion scheduling algorithms (e.g., `hi`, `low`, `hinode`, etc.). Note that Fluence does not currently support use in conjunction with the kube-scheduler. Pods must all be scheduled by Fluence.

🚧️ Under Construction! 🚧️

## Getting started

For instructions on how to start Fluence on a K8s cluster, see [examples](examples/). Documentation and instructions for reproducing our CANOPIE2022 paper (citation below) can be found in the [canopie22-artifacts branch](https://github.com/flux-framework/flux-k8s/tree/canopie22-artifacts).
For background on the Flux framework and the Fluxion scheduler, you can take a look at our award-winning R&D100 submission: https://ipo.llnl.gov/sites/default/files/2022-02/Flux_RD100_Final.pdf. For next steps:

 - To deploy our pre-built images, go to [Deploy](#deploy)
 - To build your own images, go to [Setup](#setup)

### Deploy

We provide a set of pre-build containers [alongside the repository](https://github.com/orgs/flux-framework/packages?repo_name=flux-k8s)
that you can easily use to deploy Fluence right away! You'll simply need to clone the proper helm charts, and then install to your cluster.
Here are the quick install steps:

```bash
$ git clone https://github.com/openshift-psap/scheduler-plugins.git -b fluence
$ cd scheduler-plugins/manifests/install/charts
$ helm install \
  --set scheduler.image=ghcr.io/flux-framework/fluence:latest \
  --set scheduler.sidecarimage=ghcr.io/flux-framework/fluence-sidecar \
    schedscheduler-plugins as-a-second-scheduler/
```

And that's it! See the [testing install](#testing-install) section for a basic example
to schedule pods using Fluence.

### Setup

To build and test Fluence, you will need:

 - [Go](https://go.dev/doc/install), we have tested with version 1.19
 - [helm](https://helm.sh/docs/intro/install/) to install charts for scheduler plugins.
 - A Kubernetes cluster for testing, e.g., you can deploy one with [kind](https://kind.sigs.k8s.io/docs/user/quick-start/)

### Building Fluence

There are two images we will be building:

 - the scheduler sidecar: built from the repository here
 - the scheduler: built from [this branch of scheduler-plugins](https://github.com/openshift-psap/scheduler-plugins/blob/fluence/build/scheduler/Dockerfile)

We will be adding more notes about how these containers work together.

#### Build Sidecar

To build the plugin containers, cd into [scheduler-plugins](./scheduler-plugins) and run `make`:

```bash
cd scheduler-plugin
make
```

To build for a custom registry (e.g., "vanessa' on Docker Hub):

```bash
make LOCAL_REGISTRY=vanessa
```

This will create the scheduler plugin main container, which can be tagged and pushed to the preferred registry. As an example,
here we push to the result of the build above:

```bash
docker push docker.io/vanessa/fluence-sidecar:latest
```

#### Build Scheduler

First, clone the repository:

```bash
$ git clone -b fluence https://github.com/openshift-psap/scheduler-plugins ./plugins
```

And build! You'll most likely want to set a custom registry and image name again:

```bash
# This will build to localhost
$ make local-image

# this will build to docker.io/vanessa/fluence
$ make local-image LOCAL_REGISTRY=vanessa LOCAL_IMAGE=fluence
```

**Important** the make command above produces _two images_ and you want to use the first that is mentioned in the output (not the second, which is a controller).

And push to your registry for later!

```bash
$ docker push docker.io/vanessa/fluence
```

### Prepare Cluster

> Prepare a cluster and install the Kubernetes scheduling plugins framework

These steps will require a Kubernetes cluster to install to, and having pushed the plugin container to a registry. If you aren't using a cloud provider, you can
create a local one with `kind`:

```bash
$ kind create cluster
```

### Install Fluence

For some background, the [Scheduling Framework](https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/) provided by
Kubernetes means that our container is going to provide specific endpoints to allow for custom scheduling.
To make it possible to use our own plugin, we need to install our fork of [kubernetes-sigs/scheduler-plugins](https://github.com/kubernetes-sigs/scheduler-plugins) at [openshift-psap/scheduler-plugins](https://github.com/openshift-psap/scheduler-plugins.git) which will be done through Helm charts. We will be customizing the values.yaml file to point to our image. First, clone the repository:

```bash
$ git clone https://github.com/openshift-psap/scheduler-plugins.git -b fluence
```

Install the charts, using the images you just built:

```bash
$ cd scheduler-plugins/manifests/install/charts
```

Here is how to see values allowed. Note the name of the directory - "as-a-second-scheduler" - this is what we are going to be doing - installing
Fluxion as a second scheduler!

```bash
$ helm show values as-a-second-scheduler/
```

<details>

<summary>Helm values for as-a-second-scheduler</summary>

```console
# Default values for scheduler-plugins-as-a-second-scheduler.
# This is a YAML-formatted file.
# Declare variables to be passed into your templates.

scheduler:
  name: fluence
  # image: k8s.gcr.io/scheduler-plugins/kube-scheduler:v0.23.10
  image: quay.io/cmisale1/fluence:upstream
  namespace: scheduler-plugins
  replicaCount: 1
  leaderElect: false
  sidecarimage: quay.io/cmisale1/fluence-sidecar:latest
  policy: lonode

controller:
  name: scheduler-plugins-controller
  image: k8s.gcr.io/scheduler-plugins/controller:v0.23.10
  namespace: scheduler-plugins
  replicaCount: 1

# LoadVariationRiskBalancing and TargetLoadPacking are not enabled by default
# as they need extra RBAC privileges on metrics.k8s.io.

plugins:
  enabled: ["Fluence"]
  disabled: ["CapacityScheduling","NodeResourceTopologyMatch","NodeResourcesAllocatable","PrioritySort","Coscheduling"] # only in-tree plugins need to be defined here
```

</details>

Note that this plugin is going to allow us to create a Deployment with our plugin to be used as a scheduler!
So we are good installing the defaults.

```bash
$ helm install \
  --set scheduler.image=vanessa/fluence:latest \
  --set scheduler.sidecarimage=vanessa/fluence-sidecar \
    schedscheduler-plugins as-a-second-scheduler/
```

### Testing Install

The installation process will run one scheduler and one controller pod for the Scheduler Plugin Framework in the default namespace.
You can double check that everything is running as follows:

```bash
$ kubectl get pods -n scheduler-plugins
```
```console
NAME                                            READY   STATUS    RESTARTS   AGE
fluence-6bbcbc6bbf-xjfx6                        2/2     Running   0          2m35s
scheduler-plugins-controller-787757d8b8-ss5qv   1/1     Running   0          2m35s
```

Wait until the pods are running! You've just deployed Fluence, congratulations!
Let's now check logs for containers to check that everything is OK.
First, let's look at logs for the sidecar container:

```bash
$ kubectl logs -n scheduler-plugins fluence-6bbcbc6bbf-xjfx6 
```
```console
Defaulted container "sidecar" out of: sidecar, scheduler-plugins-scheduler
This is the fluxion grpc server
Created cli context  &{}
&{}
Number nodes  1
node in flux group  kind-control-plane
Node  kind-control-plane  flux cpu  6
Node  kind-control-plane  total mem  16132255744
Can request at most  6  exclusive cpu
Match policy:  {"matcher_policy": "lonode"}
[GRPCServer] gRPC Listening on [::]:4242
```

And for the fluence container:

```bash
$ kubectl logs -n scheduler-plugins fluence-6bbcbc6bbf-xjfx6 -c scheduler-plugins-scheduler
```

If you haven't done anything, you'll likely just see health checks.

### Deploy Pods

Let's now run a simple example! Change directory into this directory:

```bash
cd ../../../../examples/simple_example
```

And then we want to deploy two pods, one assigned to the `default-scheduler` and the other
`fluence`. For FYI, we do this via setting `schedulerName` in the spec:

```yaml
spec:
  schedulerName: fluence
```

Here is how to create the pods:

```bash
$ kubectl apply -f default-scheduler-pod.yaml
$ kubectl apply -f fluence-scheduler-pod.yaml
```

Once it was created, aside from checking that it ran OK, I could verify by looking at the scheduler logs again:

```bash
$ kubectl logs -n scheduler-plugins fluence-6bbcbc6bbf-xjfx6
```
```bash
Defaulted container "sidecar" out of: sidecar, scheduler-plugins-scheduler
This is the fluxion grpc server
Created cli context  &{}
&{}
Number nodes  1
node in flux group  kind-control-plane
Node  kind-control-plane  flux cpu  6
Node  kind-control-plane  total mem  16132255744
Can request at most  6  exclusive cpu
Match policy:  {"matcher_policy": "lonode"}
[GRPCServer] gRPC Listening on [::]:4242
Labels  []   0
No labels, going with plain JobSpec
[JobSpec] JobSpec in YAML:
version: 9999
resources:
- type: slot
  count: 1
  label: default
  with:
  - type: core
    count: 1
attributes:
  system:
    duration: 3600
tasks:
- command: [""]
  slot: default
  count:
    per_slot: 1

[GRPCServer] Received Match request ps:{id:"fluence-scheduled-pod" cpu:1} request:"allocate" count:1

	----Match Allocate output---
jobid: 1
reserved: false
allocated: {"graph": {"nodes": [{"id": "3", "metadata": {"type": "core", "basename": "core", "name": "core0", "id": 0, "uniq_id": 3, "rank": -1, "exclusive": true, "unit": "", "size": 1, "paths": {"containment": "/k8scluster0/1/kind-control-plane2/core0"}}}, {"id": "2", "metadata": {"type": "node", "basename": "kind-control-plane", "name": "kind-control-plane2", "id": 2, "uniq_id": 2, "rank": -1, "exclusive": false, "unit": "", "size": 1, "paths": {"containment": "/k8scluster0/1/kind-control-plane2"}}}, {"id": "1", "metadata": {"type": "subnet", "basename": "", "name": "1", "id": 0, "uniq_id": 1, "rank": -1, "exclusive": false, "unit": "", "size": 1, "paths": {"containment": "/k8scluster0/1"}}}, {"id": "0", "metadata": {"type": "cluster", "basename": "k8scluster", "name": "k8scluster0", "id": 0, "uniq_id": 0, "rank": -1, "exclusive": false, "unit": "", "size": 1, "paths": {"containment": "/k8scluster0"}}}], "edges": [{"source": "2", "target": "3", "metadata": {"name": {"containment": "contains"}}}, {"source": "1", "target": "2", "metadata": {"name": {"containment": "contains"}}}, {"source": "0", "target": "1", "metadata": {"name": {"containment": "contains"}}}]}}

at: 0
overhead: 0.000549
error: 0
[MatchRPC] Errors so far: 
FINAL NODE RESULT:
 [{node kind-control-plane2 kind-control-plane 1}]
[GRPCServer] Response podID:"fluence-scheduled-pod" nodelist:{nodeID:"kind-control-plane" tasks:1} jobID:1 
```

I was trying to look for a way to see the assignment, and maybe we can see it here (this is the best I could come up with!)

```bash
$ kubectl get events -o wide
```
```console
$ kubectl get events -o wide |  awk {'print $4" " $5" " $6'} | column -t
REASON                     OBJECT                                        SUBOBJECT
pod/default-scheduler-pod  default-scheduler                             Successfully
pod/default-scheduler-pod  spec.containers{default-scheduler-container}  kubelet,
pod/default-scheduler-pod  spec.containers{default-scheduler-container}  kubelet,
pod/default-scheduler-pod  spec.containers{default-scheduler-container}  kubelet,
pod/default-scheduler-pod  spec.containers{default-scheduler-container}  kubelet,
pod/fluence-scheduled-pod  fluence,                                      fluence-fluence-6bbcbc6bbf-xjfx6
pod/fluence-scheduled-pod  spec.containers{fluence-scheduled-container}  kubelet,
pod/fluence-scheduled-pod  spec.containers{fluence-scheduled-container}  kubelet,
pod/fluence-scheduled-pod  spec.containers{fluence-scheduled-container}  kubelet,
...
```

There might be a better way to see that? Anyway, really cool! For the above, I found [this page](https://kubernetes.io/docs/tasks/extend-kubernetes/configure-multiple-schedulers/#enable-leader-election) very helpful.


## Papers

You can find details of Fluence architecture, implementation, experiments, and improvements to the Kubeflow MPI operator in our collaboration's papers:
```
@INPROCEEDINGS{10029991,
  author={Milroy, Daniel J. and Misale, Claudia and Georgakoudis, Giorgis and Elengikal, Tonia and Sarkar, Abhik and Drocco, Maurizio and Patki, Tapasya and Yeom, Jae-Seung and Gutierrez, Carlos Eduardo Arango and Ahn, Dong H. and Park, Yoonho},
  booktitle={2022 IEEE/ACM 4th International Workshop on Containers and New Orchestration Paradigms for Isolated Environments in HPC (CANOPIE-HPC)}, 
  title={One Step Closer to Converged Computing: Achieving Scalability with Cloud-Native HPC}, 
  year={2022},
  volume={},
  number={},
  pages={57-70},
  doi={10.1109/CANOPIE-HPC56864.2022.00011}
}
```
```
@INPROCEEDINGS{9652595,
  author={Misale, Claudia and Drocco, Maurizio and Milroy, Daniel J. and Gutierrez, Carlos Eduardo Arango and Herbein, Stephen and Ahn, Dong H. and Park, Yoonho},
  booktitle={2021 3rd International Workshop on Containers and New Orchestration Paradigms for Isolated Environments in HPC (CANOPIE-HPC)}, 
  title={It&#x0027;s a Scheduling Affair: GROMACS in the Cloud with the KubeFlux Scheduler}, 
  year={2021},
  volume={},
  number={},
  pages={10-16},
  doi={10.1109/CANOPIEHPC54579.2021.00006}
}
```
```
@inproceedings{10.1007/978-3-030-96498-6_18,
	address = {Cham},
	author = {Misale, Claudia and Milroy, Daniel J. and Gutierrez, Carlos Eduardo Arango and Drocco, Maurizio and Herbein, Stephen and Ahn, Dong H. and Kaiser, Zvonko and Park, Yoonho},
	booktitle = {Driving Scientific and Engineering Discoveries Through the Integration of Experiment, Big Data, and Modeling and Simulation},
	editor = {Nichols, Jeffrey and Maccabe, Arthur `Barney' and Nutaro, James and Pophale, Swaroop and Devineni, Pravallika and Ahearn, Theresa and Verastegui, Becky},
	isbn = {978-3-030-96498-6},
	pages = {310--326},
	publisher = {Springer International Publishing},
	title = {Towards Standard Kubernetes Scheduling Interfaces for Converged Computing},
	year = {2022}
}
```

Release

SPDX-License-Identifier: Apache-2.0

LLNL-CODE-764420
