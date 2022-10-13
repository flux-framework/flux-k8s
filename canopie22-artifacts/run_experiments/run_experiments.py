#!/usr/bin/env python3

import argparse
import os
import yaml
import subprocess
from multiprocessing import Pool
import time
import uuid
from datetime import datetime
from math import ceil
from kubernetes import client, config, watch
from collections import defaultdict
from process_job_template import process_template


def get_cluster_config(client, cluster_log, node_cpu, node_mem):
    """
    Get cluster node configuration and build
    the zone topology map.
    """
    cluster_nodes = client.list_node()
    try:
        with open(cluster_log, "w") as f:
            f.write(cluster_nodes.to_str())
    except IOError as e:
        raise

    node_topology = {"nodes": {}}
    node_topology["cpu"] = node_cpu
    node_topology["mem"] = node_mem
    for node in cluster_nodes.items:
        node_topology["nodes"][node.metadata.name] = node.metadata.labels[
            "topology.kubernetes.io/zone"
        ]

    return node_topology


def read_template(app):
    template = app["metadata"]["template_path"]
    try:
        with open(template, "r") as f:
            mpi_job = list(yaml.safe_load_all(f))
    except IOError as e:
        print(e)
        raise

    app["spec"]["jobspec"] = mpi_job


def submit_job(app, processed_job):
    """
    Create the job in Kubernetes.
    """
    app["spec"]["submitted_job"] = False
    try:
        # clobber existing tmp file and create for appending
        tmp_file = app["metadata"]["tmp_file"]
        with open(tmp_file, "w") as f:
            for job in processed_job:
                f.write(yaml.dump(job))
                f.write("---\n")

        subprocess.run([f"kubectl apply -f {tmp_file}"], shell=True, check=True)
    except Exception as e:
        print(e)
        raise RuntimeError(str(e))

    app["spec"]["submitted_job"] = True


def delete_job(app):
    """
    Delete the Kubernetes job.
    """
    if app["spec"]["submitted_job"]:
        job_path = app["metadata"]["tmp_file"]
    else:
        job_path = app["metadata"]["template_path"]

    try:
        subprocess.run([f"kubectl delete -f {job_path}"], shell=True, check=True)
    except subprocess.CalledProcessError as e:
        print(e)
        raise


def get_app_logs(app, start_time, job_id, sched, node_topology, idx):
    """
    Get the application mapping and performance logs
    """
    config.load_kube_config()
    log_client = client.CoreV1Api()
    app_name = app["metadata"]["name"]
    log_file = f"{app['metadata']['log_dir']}/{app_name}_{job_id}_{sched}_"
    log_file += f"{str(datetime.utcnow().strftime('%Y-%m-%d_T%H%M_%S'))}.out"
    watcher = watch.Watch()
    for event in watcher.stream(
        func=log_client.list_namespaced_pod,
        namespace="default",
        label_selector="training.kubeflow.org/job-role=launcher",
        timeout_seconds=600,
    ):
        if (
            event["object"].status.phase == "Failed"
            and app_name in event["object"].metadata.name
        ):
            watcher.stop()
            with open(log_file, "a") as f:
                f.write(f"MPI Job Failed at: {datetime.utcnow()}\n")
            raise RuntimeError(f"MPI Job Failed at: {datetime.utcnow()}")

        if (
            event["object"].status.phase == "Succeeded"
            and app_name in event["object"].metadata.name
        ):
            watcher.stop()
            # Get the launcher logs
            launcher_name = event["object"].metadata.name
            log_output = log_client.read_namespaced_pod_log(
                name=launcher_name, namespace="default"
            )
            # Get the job topology information
            selector = f"training.kubeflow.org/job-role=worker,app={app_name}"
            workers = log_client.list_namespaced_pod(
                namespace="default", label_selector=selector
            )
            mapping = f"Mapping for job {job_id} for scheduler {sched}\n"
            mapping_dict = defaultdict(int)
            for worker in workers.items:
                mapping_dict[worker.spec.node_name] += 1
                mapping += f"{worker.metadata.name} --> {worker.spec.node_name}\n"

            for worker_name, count in mapping_dict.items():
                mapping += f"{worker_name} ran {str(count)} pods in zone "
                mapping += f"{node_topology['nodes'][worker_name]}\n"

            log_output += mapping

            # Write the run configuration
            rconfig = f"Run configuration for job {job_id}:\n"
            rconfig += f"Ranks: {app['spec']['exp_config'][idx]['ranks']}, pods: "
            rconfig += f"{app['spec']['exp_config'][idx]['pods']}, pods per node: "
            rconfig += f"{app['spec']['exp_config'][idx]['pods_per_node']}\n"
            rconfig += (
                f"Slots per pod: {app['spec']['exp_config'][idx]['slots_per_pod']}, "
            )
            rconfig += f"GiB memory per pod: {app['spec']['exp_config'][idx]['pod_memory_gb']}\n"
            rconfig += f"Launcher CPUs: {node_topology['cpu']}, "
            rconfig += f"launcher memory in GiB: {node_topology['mem']}\n"
            log_output += rconfig
            with open(log_file, "a") as f:
                f.write(log_output)

            get_operator_logs(log_client, app_name, start_time, job_id, log_file)


def get_operator_logs(client, app_name, start_time, job_id, log_file):
    """
    Get the MPI Operator logs to write the relevant times
    to the application log file.
    """
    since_s = ceil((datetime.utcnow() - start_time).total_seconds())
    mpi_operator = client.list_pod_for_all_namespaces(label_selector="app=mpi-operator")
    # TODO: could there ever be more than one MPI Operator returned?
    operator_output = client.read_namespaced_pod_log(
        name=mpi_operator.items[0].metadata.name,
        namespace="mpi-operator",
        since_seconds=since_s,
        timestamps=True,
    )

    seen = False
    tmp = start_time = startup_time = end_time = None
    for line in operator_output.splitlines():
        if job_id in line and app_name in line:
            """
            Need try, excepts in this block because K8s and
            OpenShift return different string formats, even with
            the same MPI Operator container and Python API versions.
            """
            try:
                tmp = datetime.fromisoformat(line.strip().split(" ")[0][:-4])
            except ValueError:
                tmp = datetime.fromisoformat(line.strip().split(" ")[0][:-12])

            if not seen:
                seen = True
                start_time = tmp
            else:
                if "reason: 'MPIJobRunning'" in line:
                    startup_time = tmp
                elif "reason: 'MPIJobSucceeded'" in line:
                    end_time = tmp
                    break

    startup_time_s = (startup_time - start_time).total_seconds()
    run_time_s = (end_time - startup_time).total_seconds()
    end_to_end_time_s = (end_time - start_time).total_seconds()
    log_output = (
        f"MPIJob startup time for job {app_name}-{job_id} is {startup_time_s} seconds\n"
    )
    log_output += (
        f"MPIJob runtime for job {app_name}-{job_id} is {run_time_s} seconds\n"
    )
    log_output += f"MPIJob end-to-end time for job {app_name}-{job_id} "
    log_output += f"{end_to_end_time_s} seconds\n"
    with open(log_file, "a") as f:
        f.write(log_output)


def process_error_handler(e):
    print(e)
    raise


def run(kube_client, apps, num_runs, num_run_configs, node_topology):
    """
    Run the experiments for the input apps
    """

    for app in apps:
        try:
            read_template(app)
        except IOError:
            raise

    # Test the two schedulers
    for sched in ["fluence", "default-scheduler"]:
        if sched == "fluence":
            podgroup = True
        else:
            podgroup = False

        for idx in range(num_run_configs):
            # Run the experiments
            for rep in range(1, num_runs + 1):
                # Generate UUID for all tasks in the experiment
                job_id = uuid.uuid4().hex[:12]
                print(
                    f"\nStarting jobid {job_id}, run {rep} of "
                    f"{num_runs} with the {sched} scheduler"
                )
                start_time = datetime.utcnow()
                try:
                    for app in apps:
                        print(
                            f"\nStarting {app['metadata']['name']} on "
                            f"{app['spec']['exp_config'][idx]['ranks']} MPI ranks across "
                            f"{app['spec']['exp_config'][idx]['pods']} pods with "
                            f"{app['spec']['exp_config'][idx]['slots_per_pod']} slots per pod\n"
                        )
                        job = process_template(
                            app, job_id, sched, node_topology, idx, podgroup=podgroup
                        )
                        submit_job(app, job)

                    # Start an asynchronous process pool to monitor
                    # the apps in parallel
                    with Pool(processes=len(apps)) as pool:
                        results = []
                        for app in apps:
                            results.append(
                                pool.apply_async(
                                    get_app_logs,
                                    args=(
                                        app,
                                        start_time,
                                        job_id,
                                        sched,
                                        node_topology,
                                        idx,
                                    ),
                                    error_callback=process_error_handler,
                                )
                            )
                        [result.wait() for result in results]

                except Exception as e:
                    print(e)

                try:
                    for app in apps:
                        print(f"\nDeleting {app['metadata']['name']}")
                        delete_job(app)
                except Exception as e:
                    print(e)

                # Need to wait for all pods to terminate or Operator will generate errors
                for app in apps:
                    time.sleep(5)
                    pods = app["spec"]["exp_config"][idx]["pods"]
                    while pods > 0:
                        pods = len(
                            kube_client.list_pod_for_all_namespaces(
                                label_selector=f"app={app['metadata']['name']}"
                            ).items
                        )
                        time.sleep(5)
                print(
                    f"\nCompleted jobid {job_id}\n"
                    f"================================="
                    f"================================\n"
                )


def main():
    parser = argparse.ArgumentParser(description="Run Kubernetes HPC scaling studies")
    parser.add_argument(
        "-app_config",
        "--app_config",
        required=True,
        help="file containing the applications configurations in JSON format",
    )
    parser.add_argument(
        "-node_cpu",
        "--node_cpu",
        type=int,
        required=True,
        help="number of CPUs per worker node",
    )
    parser.add_argument(
        "-node_mem",
        "--node_mem",
        type=int,
        required=True,
        help="GiB memory per worker node",
    )
    parser.add_argument(
        "-num_runs",
        "--num_runs",
        type=int,
        required=False,
        default=10,
        help="number of runs per test configuration",
    )
    parser.add_argument(
        "-cluster_log",
        "--cluster_log",
        required=False,
        default="cluster.log",
        help="log file containing cluster node details",
    )
    args = parser.parse_args()

    with open(args.app_config, "r") as f:
        apps = yaml.safe_load(f)

    config_lens = set()
    for app in apps:
        if not os.path.isdir(app["metadata"]["log_dir"]):
            print(
                "Output directory does not exist. Please create it before running tests."
            )
            return
        config_lens.add(len(app["spec"]["exp_config"]))

    # Make sure all the run con
    num_run_configs = next(iter(config_lens))
    if len(config_lens) > 1:
        print("Apps specify different number of runtime tests. This is not supported.")
        return

    config.load_kube_config()
    kube_client = client.CoreV1Api()

    # Write cluster node configuration and return mapping
    node_topology = get_cluster_config(
        kube_client, args.cluster_log, args.node_cpu, args.node_mem
    )

    try:
        run(kube_client, apps, args.num_runs, num_run_configs, node_topology)
    except Exception as e:
        print(e)
        raise


if __name__ == "__main__":
    main()
