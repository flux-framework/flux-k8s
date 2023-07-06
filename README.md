# Fluence

![docs/images/fluence.png](docs/images/fluence.png)

Fluence enables HPC-grade pod scheduling in Kubernetes via the [Kubernetes Scheduling Framework](https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/). Fluence uses the directed-graph based [Fluxion scheduler](https://github.com/flux-framework/flux-sched) to map pods or [podgroups](https://github.com/kubernetes-sigs/scheduler-plugins/tree/master/pkg/coscheduling) to nodes. Fluence supports all the Fluxion scheduling algorithms (e.g., `hi`, `low`, `hinode`, etc.). Note that Fluence does not currently support use in conjunction with the kube-scheduler. Pods must all be scheduled by Fluence.

🚧️ Under Construction! 🚧️

## Getting started

For instructions on how to start Fluence on a K8s cluster, see [examples](examples/). Documentation and instructions for reproducing our CANOPIE2022 paper (citation below) can be found in the [canopie22-artifacts branch](/../canopie22-artifacts/canopie22-artifacts).

For background on the Flux framework and the Fluxion scheduler, you can take a look at our award-winning R&D100 submission: https://ipo.llnl.gov/sites/default/files/2022-02/Flux_RD100_Final.pdf

### Setup

To build and test Fluence, you will need:

 - [Go](https://go.dev/doc/install), we have tested with version 1.19
 - [helm](https://helm.sh/docs/intro/install/) to install charts for scheduler plugins.
 - A Kubernetes cluster for testing, e.g., you can deploy one with [kind](https://kind.sigs.k8s.io/docs/user/quick-start/)

### Building Fluence

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

### Prepare Cluster

> Prepare a cluster and install the Kubernetes scheduling plugins framework

These steps will require a Kubernetes cluster to install to, and having pushed the plugin container to a registry. If you aren't using a cloud provider, you can
create a local one with `kind`:

```bash
$ kind create cluster
```

For some background, the [Scheduling Framework](https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/) provided by
Kubernetes means that our container is going to provide specific endpoints to allow for custom scheduling.
To make it possible to use our own plugin, we need to install [kubernetes-sigs/scheduler-plugins](https://github.com/kubernetes-sigs/scheduler-plugins),
which will be done through Helm charts. We will be customizing the values.yaml file to point to our image.
First, clone the repository:

```bash
git clone git@github.com:kubernetes-sigs/scheduler-plugins.git
cd scheduler-plugins/manifests/install/charts
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
  name: scheduler-plugins-scheduler
  image: registry.k8s.io/scheduler-plugins/kube-scheduler:v0.25.7
  replicaCount: 1
  leaderElect: false

controller:
  name: scheduler-plugins-controller
  image: registry.k8s.io/scheduler-plugins/controller:v0.25.7
  replicaCount: 1

# LoadVariationRiskBalancing and TargetLoadPacking are not enabled by default
# as they need extra RBAC privileges on metrics.k8s.io.

plugins:
  enabled: ["Coscheduling","CapacityScheduling","NodeResourceTopologyMatch","NodeResourcesAllocatable"]
  disabled: ["PrioritySort"] # only in-tree plugins need to be defined here

# Customize the enabled plugins' config.
# Refer to the "pluginConfig" section of manifests/<plugin>/scheduler-config.yaml.
# For example, for Coscheduling plugin, you want to customize the permit waiting timeout to 10 seconds:
pluginConfig:
- name: Coscheduling
  args:
    permitWaitingTimeSeconds: 10 # default is 60
# Or, customize the other plugins
# - name: NodeResourceTopologyMatch
#   args:
#     scoringStrategy:
#       type: MostAllocated # default is LeastAllocated
```

</details>

Note that this plugin is going to allow us to create a Deployment with our plugin to be used as a scheduler!
So we are good installing the defaults.

```bash
helm install scheduler-plugins as-a-second-scheduler/
```

The installation process will run one scheduler and one controller pod for the Scheduler Plugin Framework in the default namespace.
You can double check that everything is running as follows:

```bash
$ kubectl get  pods 
```
```console
NAME                                           READY   STATUS    RESTARTS   AGE
scheduler-plugins-controller-ccbc6dcf5-qdkmw   1/1     Running   0          64s
scheduler-plugins-scheduler-594968bf65-4dknk   1/1     Running   0          64s
```

### Deploy Fluence

We will need to use a Kubernetes Deployment to deploy fluence as a sidecar container! We have several examples in [examples](examples)
that you can follow, and will direct you to the [examples/demo_setup](./examples/demo_setup) example for this tutorial. From the root of the repository again:

```bash
$ cd ./examples/demo_setup
```

And create the config map and plugin:

```bash
$ kubectl create -f 01_fluence-rbac.yaml
$ kubectl create -f 02_fluence-configmap.yaml
$ kubectl create -f 03_scheduler-plugin.yaml
```

Note that the plugin uses the `scheduler-plugins-scheduler` service account. You can check when the replica is ready:

```bash
$ kubectl get  deployments
```
```console
NAME                           READY   UP-TO-DATE   AVAILABLE   AGE
fluence                        1/1     1            1           60s
scheduler-plugins-controller   1/1     1            1           31m
scheduler-plugins-scheduler    1/1     1            1           31m
```

And then you will also see fluence running as a pod (the replica mentioned above):

```bash
$ kubectl get pods
```
```console
NAME                                           READY   STATUS    RESTARTS   AGE
fluence-778dbb5996-wq92t                       2/2     Running   0          2m31s
scheduler-plugins-controller-ccbc6dcf5-qdkmw   1/1     Running   0          32m
scheduler-plugins-scheduler-594968bf65-4dknk   1/1     Running   0          32m
```

And you can look at logs:

```bash
$ kubectl logs fluence-778dbb5996-wq92t 
```
```console
Defaulted container "fluence-sidecar" out of: fluence-sidecar, fluence
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

At this point try scheduling a job and asking to use the "fluence" scheduler:

```bash
$ kubectl apply -f demo-job.yaml
```

*STOPPED HERE* it's not clear how the change in namespaces (the second plugin does not deploy to a scheduler-plugins namespace)
is interfering with the setup here. I am trying to create (almost) the same setup as I see with lammps, but there is some connection
missing because I don't see that fluence is known as a sidecar and the container does not run. Below are the original
docs that described usage:

The sidecar container will show all the jobs being allocated using Fluence, the detailed output of each allocation, job IDs, errors if any.

```bash
kubectl logs <podname> -n scheduler-plugins -c sidecar
```
The main container's logs are also useful to check on possible deployment errors, initialization steps and calls to the gRPC server in the sidecar container:

```bash
kubectl logs <podname> -n scheduler-plugins -c scheduler-plugins-scheduler
```

I think I'm missing some step and I need to read online about how these plugins work.

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
