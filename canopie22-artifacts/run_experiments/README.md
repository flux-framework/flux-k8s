# Run  script for benchmark suite

This script takes a JSON application configuration and other inputs as arguments 
and submits the applications sequentially. It parallelizes the K8s watchers 
to look for state changes in the MPI Operator and pods for each application.

The script requires the official Kubernetes Python API:
```bash
pip3 install kubernetes
```
You can also find it here: https://github.com/kubernetes-client/python

Refer to app-experiment-config.json for an example for running AMG, LAMMPS,
and QMCPACK. Note that the "pod_memory_gb" key overrides the default
behavior of each pod receiving a memory allocation proportional to the slots
per pod setting.

Example invocation:
```bash
./run_experiments.py --app_config app-experiment-config.json --node_cpu 94 --node_mem 346 --num_runs 20 --cluster_log cluster.log
```

