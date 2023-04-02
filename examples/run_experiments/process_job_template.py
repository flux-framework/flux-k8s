#!/usr/bin/env python3

from math import floor
import copy


def generate_mpirun_cmd(app, job, idx):
    """
    Generate the mpirun command for a particular application.
    Add custom parsing for apps here
    """
    # Configure launcher spec
    num_ranks = app["spec"]["exp_config"][idx]["ranks"]
    mpirun_cmd = job["spec"]["mpiReplicaSpecs"]["Launcher"]["template"]["spec"][
        "containers"
    ][0]["command"][2].strip()

    # Default: just replace %PROCS%
    mpirun_cmd = mpirun_cmd.replace("%PROCS%", str(num_ranks))
    # LAMMMPS
    if app["metadata"]["name"] == "amg":
        mpirun_cmd = mpirun_cmd.replace(
            "-n %SIZE% %SIZE% %SIZE% -P %SIZE% %SIZE% %SIZE%",
            f"-n {app['spec']['per_rank_size']} -P {app['spec']['rank_topology'][idx]}",
        )
    # AMG
    elif app["metadata"]["name"] == "lammps":
        mpirun_cmd = mpirun_cmd.replace(
            "%PROBLEM_SIZE%",
            f"{app['spec']['problem_size']}",
        )

    return mpirun_cmd


def process_template(app, jobid, scheduler, node_topology, idx, podgroup=True):
    """
    Parse the MPI job template, replacing input parameters in a deepcopy.
    Then create the job in Kubernetes.
    """
    num_pods = app["spec"]["exp_config"][idx]["pods"]
    cpus_per_pod = app["spec"]["exp_config"][idx]["cpus_per_pod"]
    slots_per_pod = app["spec"]["exp_config"][idx]["slots_per_pod"]
    launcher_memory = str(app["spec"]["launcher_memory_gb"]) + "Gi"
    launcher_cpu = app["spec"]["launcher_cpus"]
    app["spec"]["exp_config"][idx]["pods_per_node"] = floor(
        node_topology["cpu"] / slots_per_pod
    )
    if not "pod_memory_gb" in app["spec"]["exp_config"][idx]:
        # Each pod gets memory proportional to the CPU fraction it requests
        app["spec"]["exp_config"][idx]["pod_memory_gb"] = (
            floor((node_topology["mem"] / (node_topology["cpu"] / slots_per_pod)) * 10)
            / 10.0
        )
    memory = str(app["spec"]["exp_config"][idx]["pod_memory_gb"]) + "Gi"
    if (
        app["spec"]["exp_config"][idx]["pods_per_node"] < 1
        or app["spec"]["exp_config"][idx]["pod_memory_gb"] < 1
    ):
        raise RuntimeError(
            f"Can't fit {app['metadata']['name']} configuration "
            f"on node allocatable resources"
        )

    mpi_job = copy.deepcopy(app["spec"]["jobspec"])
    # Need to add unique job ID to name so we can select the
    # relvant Operator logs.
    job_name = f"{app['metadata']['name']}-{jobid}"
    processed_job = []
    for job in mpi_job:
        if job["apiVersion"] == "kubeflow.org/v2beta1":
            # Catch errors
            try:
                job["metadata"]["name"] = job_name
                job["spec"]["slotsPerWorker"] = slots_per_pod
                job["spec"]["mpiReplicaSpecs"]["Launcher"]["template"]["spec"][
                    "containers"
                ][0]["resources"]["limits"]["cpu"] = launcher_cpu
                job["spec"]["mpiReplicaSpecs"]["Launcher"]["template"]["spec"][
                    "containers"
                ][0]["resources"]["requests"]["cpu"] = launcher_cpu
                job["spec"]["mpiReplicaSpecs"]["Launcher"]["template"]["spec"][
                    "containers"
                ][0]["resources"]["limits"]["memory"] = launcher_memory
                job["spec"]["mpiReplicaSpecs"]["Launcher"]["template"]["spec"][
                    "containers"
                ][0]["resources"]["requests"]["memory"] = launcher_memory

                # Configure worker spec
                job["spec"]["mpiReplicaSpecs"]["Worker"]["template"]["spec"][
                    "schedulerName"
                ] = scheduler
                job["spec"]["mpiReplicaSpecs"]["Worker"]["replicas"] = num_pods
                job["spec"]["mpiReplicaSpecs"]["Worker"]["template"]["spec"][
                    "containers"
                ][0]["resources"]["limits"]["cpu"] = cpus_per_pod
                job["spec"]["mpiReplicaSpecs"]["Worker"]["template"]["spec"][
                    "containers"
                ][0]["resources"]["requests"]["cpu"] = cpus_per_pod
                job["spec"]["mpiReplicaSpecs"]["Worker"]["template"]["spec"][
                    "containers"
                ][0]["resources"]["limits"]["memory"] = memory
                job["spec"]["mpiReplicaSpecs"]["Worker"]["template"]["spec"][
                    "containers"
                ][0]["resources"]["requests"]["memory"] = memory

                # Generate the mpirun command
                job["spec"]["mpiReplicaSpecs"]["Launcher"]["template"]["spec"][
                    "containers"
                ][0]["command"][2] = generate_mpirun_cmd(app, job, idx)

                if not podgroup:
                    job["spec"]["mpiReplicaSpecs"]["Worker"]["template"]["metadata"][
                        "labels"
                    ].pop("pod-group.scheduling.sigs.k8s.io", None)

                # Processced successfully, append this sub template
                processed_job.append(job)
            except Exception as e:
                print(e)
                raise RuntimeError(str(e))

        elif podgroup and job["apiVersion"] == "scheduling.sigs.k8s.io/v1alpha1":
            try:
                job["spec"]["minMember"] = num_pods
                # Processced successfully, append this sub template
                processed_job.append(job)
            except KeyError as e:
                print(e)
                raise RuntimeError(str(e))

    return processed_job


def main(app, jobid, scheduler, node_topology, idx, podgroup):
    return process_template(app, jobid, scheduler, node_topology, idx, podgroup)


if __name__ == "__main__":
    import argparse
    import yaml

    parser = argparse.ArgumentParser(description="Run Kubernetes HPC scaling studies")
    parser.add_argument(
        "-app",
        "--app",
        type=str,
        required=True,
        help="file containing the applications configurations in JSON format",
    )
    parser.add_argument(
        "-jobid",
        "--jobid",
        required=True,
        help="number of CPUs per worker node",
    )
    parser.add_argument(
        "-scheduler",
        "--scheduler",
        type=str,
        required=True,
        help="GiB memory per worker node",
    )
    parser.add_argument(
        "-node_topology",
        "--node_topology",
        type=str,
        required=True,
        help="number of runs per test configuration",
    )
    parser.add_argument(
        "-idx",
        "--idx",
        required=True,
        type=int,
        help="log file containing cluster node details",
    )
    parser.add_argument(
        "-podgroup",
        "--podgroup",
        required=False,
        action="store_true",
        help="log file containing cluster node details",
    )
    args = parser.parse_args()
    app = yaml.safe_load(args.app)
    node_topology = yaml.safe_load(args.node_topology)

    processed_job = main(
        app, args.jobid, args.scheduler, node_topology, args.idx, args.podgroup
    )
