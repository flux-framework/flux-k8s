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
We provide helper commands to do that.

```bash
# This clones the upstream scheduler plugins code, we will add fluence to it!
$ make prepare

# Add fluence assets
$ cd upstream/manifests/install/charts
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

#### All at once (Sidecar + Scheduler)

**recommended**

This will run the full builds for all containers in one step, which includes:

1. Building the fluence sidecar from source code in [src](src)
2. Cloning the upstream kubernetes-sigs/plugin-schedulers respository to ./upstream
3. Building the scheduler container

From the root here:

```bash
make
```

or customize the naming of your registry or local images:

```bash
make REGISTRY=vanessa SCHEDULER_IMAGE=fluence SIDECAR_IMAGE=fluence-sidecar
```

As an alternative, you can do each of the steps separately or manually (detailed below).

<details>

<summary> Manual Build Instructions </summary>

#### Build Sidecar

To build the plugin containers, we will basically be running `make` from the [src](src) directory. We have wrapped that for you
in the Makefile:

```bash
make build-sidecar
```

To build for a custom registry (e.g., "vanessa' on Docker Hub):

```bash
make build-sidecar REGISTRY=vanessa
```

And specify the sidecar image name too:

```bash
make build-sidecar REGISTRY=vanessa SIDECAR_IMAGE=another-sidecar
```

The equivalent manual command is:

```bash
cd src
make
```

Using either of the approaches above, this will create the scheduler plugin main container, which can be tagged and pushed to the preferred registry. As an example,
here we push to the result of the build above:

```bash
docker push docker.io/vanessa/fluence-sidecar:latest
```

#### Build Scheduler

Note that you can run this entire process like:

```bash
make prepare
make build
```

Or customize the name of the scheduler image:

```bash
make prepare
make build REGISTRY=vanessa
```

For a custom scheduler or controller image (we just need the scheduler):

```bash
make build REGISTRY=vanessa CONTROLLER_IMAGE=fluence-controller SCHEDULER_IMAGE=fluence
```

To walk through it manually, first, clone the upstream scheduler-plugins repository:

```bash
$ git clone https://github.com/kubernetes-sigs/scheduler-plugins ./upstream
```

We need to add our fluence package to the scheduler plugins to build. You can do that manully as follows:

```bash
# These are entirely new directory structures
cp -R sig-scheduler-plugins/pkg/fluence ./upstream/pkg/fluence
cp -R sig-scheduler-plugins/manifests/fluence ./upstream/manifests/fluence

# These are files with subtle changes to add fluence
cp sig-scheduler-plugins/cmd/scheduler/main.go ./upstream/cmd/scheduler/main.go
cp sig-scheduler-plugins/manifests/install/charts/as-a-second-scheduler/templates/deployment.yaml ./upstream/manifests/install/charts/as-a-second-scheduler/templates/deployment.yaml
cp sig-scheduler-plugins/manifests/install/charts/as-a-second-scheduler/templates/values.yaml ./upstream/manifests/install/charts/as-a-second-scheduler/templates/values.yaml
```

Then change directory to the scheduler plugins repository. 

```bash
cd ./upstream
```

And build! You'll most likely want to set a custom registry and image name again:

```bash
# This will build to localhost
$ make local-image

# this will build to docker.io/vanessa/fluence
$ make local-image REGISTRY=vanessa CONTROLLER_IMAGE=fluence
```

</details>

**Important** the make command above produces _two images_ and you want to use the first that is mentioned in the output (not the second, which is a controller).

Whatever build approach you use, you'll want to push to your registry for later discovery!

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
Kubernetes means that our container is going to provide specific endpoints to allow for custom scheduling. At this point you can follow the instructions
under [deploy](#deploy) to ensure you have cloned the upstream kubernetes-sigs/scheduler-plugins and installed fluence. This section will provide
more details to inspect attributes available to you. Let's say that you ran

```bash
$ make prepare
```

You could then inspect values with helm:

```bash
$ cd upstream/manifests/install/charts
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
The `helm install` shown under [deploy](#deploy) is how you can install to your cluster, and then proceed to testing below.
Here would be an example using custom images:

```bash
$ cd upstream/manifests/install/charts
$ helm install \
  --set scheduler.image=vanessa/fluence:latest \
  --set scheduler.sidecarimage=vanessa/fluence-sidecar \
    schedscheduler-plugins as-a-second-scheduler/
```

Next you can move down to testing the install.

### Testing Install

The installation process will run one scheduler and one controller pod for the Scheduler Plugin Framework in the default namespace.
You can double check that everything is running as follows:

```bash
$ kubectl get pods
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
$ kubectl logs fluence-6bbcbc6bbf-xjfx6 
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
$ kubectl logs fluence-6bbcbc6bbf-xjfx6 -c scheduler-plugins-scheduler
```

If you haven't done anything, you'll likely just see health checks.

### Deploy Pods

Let's now run a simple example! Change directory into this directory:

```bash
# This is from the root of flux-k8s
cd examples/simple_example
```

And then we want to deploy two pods, one assigned to the `default-scheduler` and the other
`fluence`. For FYI, we do this via setting `schedulerName` in the spec:

```yaml
spec:
  schedulerName: fluence
```

Here is how to create the pods:

```bash
kubectl apply -f default-scheduler-pod.yaml
kubectl apply -f fluence-scheduler-pod.yaml
```

Once it was created, aside from checking that it ran OK, I could verify by looking at the scheduler logs again:

```bash
kubectl logs fluence-6bbcbc6bbf-xjfx6
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
kubectl get events -o wide
```
```bash
kubectl get events -o wide |  awk {'print $4" " $5" " $6'} | column -t
```
```console
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
