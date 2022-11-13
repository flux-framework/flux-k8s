# Benchmark Suite for CANOPIE 2022: *One Step Closer to Converged Computing: Achieving Scalability with Cloud-Native HPC*

We provide this documentation for anyone interested in reproducing our work published in the proceedings of CANOPIE 2022 or learning more about cloud-native HPC.

These benchmarks work on vanilla Kubernetes (e.g., Kind clusters), EKS and OpenShift clusters.
Each benchmark can be scheduled with either Fluence or with the default kube-scheduler.

## Creating AWS EKS cluster

If you have AWS credentials installed, you can build an EKS cluster with the same configuration (i.e., same Hpc6a EC2 instance type, Kubernetes version, etc.) we used in our study. The `eks-efa-cluster-config.yaml` in the `kube_setup` directory contains options for specifying `placementGroups`, `launcher`, and `worker`managedNodeGroups. Note that if you intend to use EC2 EFA devices that the number of launcher nodes in the EKS configuration must be the same as the number of applications requiring EFA. After modifying the configuration to create the desired cluster name, Kubernetes version, and number of instances, execute:

```bash
eksctl create cluster -f kube_setup/eks-efa-cluster-config.yaml
```
You will need the [AWS CLI](https://aws.amazon.com/cli/) and [`eksctl`](https://eksctl.io/) tools installed.

## Configuring the running cluster
All workloads require the MPI Operator, which must be deployed first via:

```bash
kubectl create -f kube_setup/mpi-operator.yaml
```
Note that `kube_setup/mpi-operator.yaml` references the MPI Operator container built with our improvements that fix race conditions.

## Deploy EFA (Only for AWS clusters)
AWS EFA requires a daemonset to make the devices available to EKS pods.  To start the daemonset on all nodes (including the launcher):
```bash
kubectl create -f kube_setup/efa-daemonset.yaml
```

## Deploy Fluence
To deploy Fluence you need to create configurations, RBACs, and the pod group CRD from `kube_setup/fluence_setup` directory. The manifests are enumerated so Kubernetes will create the corresponding objects in sequence.

```bash
kubectl create -f kube_setup/fluence_setup/
```
## Taint worker nodes
Apply a taint to worker nodes which corresponds to the toleration set in each worker pod manifest. This forces the launcher pods to run only on the launcher node(s).
```bash
./taint_workers.sh
```

# Running experiments with AMG, LAMMPS, and QMCPACK
The `run_experiments.py` script in the `run_experiments` directory allows you to run combinations of the three CORAL-2 benchmarks or other containerized, MPI Operator-based applications with the Fluence and default kube-scheduler. The `run_experiments` directory contains a README with specific instructions for running and configuring the experiments.

## MPI Job YAMLs
Each CORAL-2 benchmark directory (e.g., `lammps`) contains YAMLs for creating pods and pod groups. The manifests include macros for MPI-based parameters which are substituted at creation time by `run_experiments.py`. We discuss manifest details in the following sections.

### Group Scheduling
Fluence can schedule pod-by-pod or with group scheduling.
To enable group scheduling, we need to create a `PodGroup` object to attach to the MPI job.
Every `oc_*.yaml` has it implemented already, but the following is the example for `gromacs`.

1. Create the `PodGroup` object in the same yaml or a different one:

```yaml
apiVersion: scheduling.sigs.k8s.io/v1alpha1
kind: PodGroup
metadata:
  name: gromacs
spec:
  scheduleTimeoutSeconds: 10
  minMember: 4
```

2. Add the pod group name label in the `worker` section of the MPI job:

```yaml
[...]
 Worker:
      replicas: 4
      template:
        metadata:
          app: gromacs
          labels:
            app: gromacs
            pod-group.scheduling.sigs.k8s.io: gromacs
[...]
```

To schedule pod-by-pod you can comment out `pod-group.scheduling.sigs.k8s.io: gromacs`.

## Changes made to run on OpenShift
MPIJob yaml files that work on OpenShift are the ones named `oc_benchmarkname_mpijob.yaml`.
They load the image from `quay.io/cmisale1`, which has been built with the Dockerfile in each folder.

### User IDs
OpenShift blocks containers from running as root or as a specific users because it does randomize the UIDs. Since we use use a specific user in the container (namely `mpiuser`), we have to relax the constraint within the [security contexts](https://docs.openshift.com/container-platform/4.9/authentication/managing-security-context-constraints.html), so that we are allowed to use `AnyUID`

``` bash
oc adm policy add-scc-to-user anyuid -z default -n default
```

This command is saved in `kube_setup/oc-scc-setup.sh`. This assumes you have `oc` installed.


### Run sshd as non root user
OpenShift also restricts the use of certain ports, such as `22`.

For the port, we need to change the `sshd_config` file to use another port, `2222` and credentials for the `mpiuser`.
Therefore we need to:
1. enable ssh to use another port because port 22 is reserved in openshift (Dockerfile)
2. enable the other user (mpiuser) to use that sshd on the other port and other credentials (Dockerfile)
3. enable the user `USER mpiuser` (Dockerfile)
3. load the users' `sshAuthMountPath` (MPIJob yaml)

```yaml
apiVersion: kubeflow.org/v2beta1
kind: MPIJob 
metadata:          
  name: gromacs
spec:                         
  sshAuthMountPath: /home/mpiuser/.ssh
  [...]
```

4. start `sshd` in the worker with the custom `sshd_config` file

```yaml
- image: quay.io/cmisale1/gromacs:ocp
    imagePullPolicy: Always
    name: worker
    command:
    - /usr/sbin/sshd
    args:
    - -De
    - -f
    - /home/mpiuser/.sshd_config 
```

## Data Analysis

Each experiment directory contains a Python script that you can use to analyze an experiment's outputs via various plots. Each script provides a basic help printout:

```bash
./lammps/process_lammps.py --help
```

## Building CORAL-2 docker images
Each CORAL-2 directory contains dockerfiles to build default, portable images for Kubernetes, Kubernetes with EFA and AMD CPUs, and OpenShift. The dockerfiles fix the OpenMPI versions, and supply necessary [Spack](https://github.com/spack/spack) build arguments. The dockerfiles have optional build arguments that you may pass to configure the build. We recommend using the Docker buildkit:

```bash
cd lammps/
DOCKER_BUILDKIT=1 docker build -t lammps-focal-openmpi-4.1.2 . -f Dockerfile --build-arg spack_cpu_arch=broadwell --build_arg build_jobs=16
``` 
