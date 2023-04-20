# Run script for benchmark suite

This script takes a JSON application configuration and other inputs as arguments 
and submits the applications sequentially. It parallelizes the K8s watchers 
to look for state changes in the MPI Operator and pods for each application.

The script requires the official Kubernetes Python API:
```bash
pip3 install kubernetes
```
You can also find it here: https://github.com/kubernetes-client/python

Refer to app-experiment-config.json for an example for running AMG and LAMMPS. 
Note that the "pod_memory_gb" key overrides the default
behavior of each pod receiving a memory allocation proportional to the slots
per pod setting.

More detailed instructions are provided below.

## Benchmark YAML files

We provide a directory for each benchmark (e.g., `amg`, `lammps`, and `qmcpack`) which contains YAML templates for creating the development. These templates include macros for MPI-based parameters which are substituted when running the experiments using `run_experiments.py`.

- To run on AWS EKS with EFA, select: `<benchmark>_mpijob_efa.yaml`
- To run on OpenShift on IBM Cloud or other Cloud platforms, select: `oc_<benchmark>_mpijob.yaml.template.python`.

## Running the workflow

The JSON application configuration file `app-experiment-config.json` includes the configuration and other input parameters that define deployment for one or multiple benchmarks. Note that `"exp_config"` is a list of experimental configurations per application. Each entry corresponds to an experiment to be run with command-line specified repitions.

Depending on the size of your cluster and available nodes configuration, these parameters need to change.

```json
[
    {
        "metadata": {
            # Name of the benchmark
            "name": "lammps",
            # Path to the Benchmark YAML file
            "template_path": "../lammps/oc_lammps_mpijob.yaml.template.python",
            # Create a directory for allocating the logs mkdir triple_test
            "log_dir": "triple_test",
            # This is the temporary YAML file with all the replaced arguments
            "tmp_file": "lammps_tmp.yaml",
        },
        "spec": {
            # The launcher collects the output from all the different pods
            # used during the deployment. We recommend to have at least 2 CPUs
            "launcher_cpus": 4,
            "launcher_memory_gb": 10,

            # "exp_config" is a list of dictionaries. Supported keys are: ranks, pods,
            # cpus_per_pod, slots_per_pod, and pod_memory_gb. pod_memory_gb overrides
            # the default behavior of each pod getting memory proportional to the
            # CPU fraction it requests. pod_memory_gb is optional.
            "exp_config": [
                {
                  "ranks": 32, # number of mpi processes
                  "pods": 1, # number of pods
                  "cpus_per_pod": 32, # number of cpus per pod. They need to add to the total of ranks
                  "slots_per_pod": 32, #  slot: an allocation unit for a process. The number of slots need to add to the total of ranks
                  "pod_memory_gb": 60, # Check the available memory in your node
                }
              ],
            # This section is application specific
            # The following is LAMMPS-specific
            "problem_size": "-v x 64 -v y 16 -v z 8"
        },
    },
]
```

Example invocation for a node with 32 vcpus and 128 GB of memory:

- node_cpu: Number of cores in your node
- node_mem: Memory in your node
- num_runs: Number of runs for each benchmark listed in the config file

```raw
./run_experiments.py --app_config app-experiment-config.json --node_cpu 32 --node_mem 128 --num_runs 1 --cluster_log cluster.log
```
