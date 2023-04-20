# Fluence

![docs/images/fluence.png](docs/images/fluence.png)

Fluence enables HPC-grade pod scheduling in Kubernetes via the [Kubernetes Scheduling Framework](https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/). Fluence uses the directed-graph based [Fluxion scheduler](https://github.com/flux-framework/flux-sched) to map pods or [podgroups](https://github.com/kubernetes-sigs/scheduler-plugins/tree/master/pkg/coscheduling) to nodes. Fluence supports all the Fluxion scheduling algorithms (e.g., `hi`, `low`, `hinode`, etc.). Note that Fluence does not currently support use in conjunction with the kube-scheduler. Pods must all be scheduled by Fluence.

🚧️ Under Construction! 🚧️

## Getting started
For instructions on how to start Fluence on a K8s cluster, see [examples](examples/). Documentation and instructions for reproducing our CANOPIE2022 paper (citation below) can be found in the [canopie22-artifacts branch](/../canopie22-artifacts/canopie22-artifacts).

For background on the Flux framework and the Fluxion scheduler, you can take a look at our award-winning R&D100 submission: https://ipo.llnl.gov/sites/default/files/2022-02/Flux_RD100_Final.pdf

### Building Fluence

To build the plugin, will need to download the scheduler-plugins fork and build the containers

```bash
git clone git@github.com:openshift-psap/scheduler-plugins.git -b fluence
cd scheduler-plugins
make local-image
```

This will create the scheduler plugin main container, which can be tagged and pushed in the preferred registry.

The installation of the plugin is done through Helm charts. Follow the instructions to customize the values.yaml file.
To install it with the default values, run:

```bash
git clone git@github.com:kubernetes-sigs/scheduler-plugins.git
cd scheduler-plugins/manifests/install/charts
helm install scheduler-plugins as-a-second-scheduler/
```

The installation process will create a namespace called scheduler-plugins and run the scheduler plugin pod there.

The sidecar container will show all the jobs being allocated using Fluence, the detailed output of each allocation, job IDs, errors if any.

```bash
kubectl logs <podname> -n scheduler-plugins -c sidecar
```

The main container's logs are also useful to check on possible deployment errors, initialization steps and calls to the gRPC server in the sidecar container:

```bash
kubectl logs <podname> -n scheduler-plugins -c scheduler-plugins-scheduler
```

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
