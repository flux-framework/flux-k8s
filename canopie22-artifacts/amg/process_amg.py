#!/usr/bin/env python3

import os
import argparse
import re
import pandas as pd
import seaborn as sns
import matplotlib.pyplot as plt


def gather_outputs(outdir):
    results = {
        "scheduler": [],
        "ranks": [],
        "startup_time": [],
        "runtime": [],
        "end_to_end_time": [],
        "FOM": [],
        "real": [],
        "max_ppn": [],
        "zones": [],
        "num_pods": [],
        "slot_per_pod": [],
    }

    for file in os.listdir(outdir):
        if file.startswith("amg") and file.endswith(".out"):
            with open(f"{outdir}/{file}", "r") as f:
                lines = f.readlines()
            ppn_max = 1
            zones = set()
            for line in lines:
                if "for scheduler" in line:
                    sched = line.strip().split(" ")[-1]
                elif line.startswith("Ranks:"):
                    sline = line.strip().replace(",", "").split(" ")
                    try:
                        nranks = int(sline[1])
                    except ValueError:
                        nranks = sline[1]
                    npods = int(sline[3])
                elif line.startswith("Slots per pod:"):
                    sline = line.strip().replace(",", "").split(" ")
                    spp = int(sline[3])
                elif line.startswith("Figure of Merit (FOM_1):"):
                    fom = float(line.strip().split(" ")[-1])
                elif line.startswith("real"):
                    tmp = line.strip().split(" ")[-1]
                    try:
                        real_t = float(tmp)
                    except:
                        try:
                            real_t = float(re.search("m(.+?)s", tmp).group(1))
                        except AttributeError:
                            print("Unix runtime not found in substring")
                            raise
                elif line.startswith("MPIJob startup time for job"):
                    startup = float(line.strip().split(" ")[-2])
                elif line.startswith("MPIJob runtime for job"):
                    runtime = float(line.strip().split(" ")[-2])
                elif line.startswith("MPIJob end-to-end"):
                    e_to_e = float(line.strip().split(" ")[-2])
                elif "ran" and "pods in zone" in line:
                    pod_tmp = int(line.strip().split(" ")[2])
                    ppn_max = max([pod_tmp, ppn_max])
                    zones.add((line.strip().split(" ")[-1]))

            results["FOM"].append(fom)
            results["real"].append(real_t)
            results["scheduler"].append(sched)
            results["ranks"].append(nranks)
            results["num_pods"].append(npods)
            results["startup_time"].append(startup)
            results["runtime"].append(runtime)
            results["end_to_end_time"].append(e_to_e)
            results["max_ppn"].append(ppn_max)
            results["slot_per_pod"].append(spp)
            results["zones"].append(len(zones))

    return pd.DataFrame(data=results)


def plot_outputs(df, plotname, slot_per_pod):
    schedulers = set(df.scheduler)
    fluence = (schedulers - set(["default-scheduler"])).pop()
    # Needed because some data uses the old KubeFlux name
    palette = {"default-scheduler": "#4878d0", fluence: "#ee854a"}
    plt.figure(figsize=(32, 32))
    sns.set_style("dark")
    if slot_per_pod:
        hue = df[["scheduler", "slot_per_pod"]].apply(
            lambda row: f"{row.scheduler}, {str(row.slot_per_pod)}", axis=1
        )
        hue.name = "scheduler, slot per pod"
        ax = sns.boxplot(x="ranks", y="FOM", hue=hue, data=df, whis=[5, 95])
    else:
        ax = sns.boxplot(
            x="ranks",
            y="FOM",
            hue="scheduler",
            data=df,
            whis=[5, 95],
            palette=palette,
        )
    # medians = df.groupby(["ranks", "scheduler"])["FOM"].median().values
    # maxes = df.groupby(["ranks", "scheduler"])["max_ppn"].max().values
    # zones = df.groupby(["ranks", "scheduler"])["zones"].max().values
    # vertical_offset = df["FOM"].median() * 0.03
    # idx = 0
    # for xtick in range(len(bp.get_xticklabels())):
    #     bp.text(
    #         xtick + 0.2,
    #         medians[idx + 1] + vertical_offset,
    #         f"max pods: {str(maxes[idx + 1])} AZs: {str(zones[idx + 1])}",
    #         horizontalalignment="center",
    #         size="medium",
    #         color="black",
    #         weight="bold",
    #     )
    #     bp.text(
    #         xtick - 0.2,
    #         medians[idx] + vertical_offset,
    #         f"max pods: {str(maxes[idx])} AZs: {str(zones[idx])}",
    #         horizontalalignment="center",
    #         size="medium",
    #         color="black",
    #         weight="bold",
    #     )
    #     idx += 2

    plt.title("AMG FOM")
    plt.legend([], [], frameon=False)
    ax.set_xlabel("MPI Ranks", fontsize=20)
    ax.set_ylabel("FOM", fontsize=20)
    ax.set_xticklabels(ax.get_xmajorticklabels(), fontsize=18)
    ax.set_yticklabels(ax.get_yticks(), fontsize=18)
    plt.savefig(f"fom_{plotname}.pdf")
    plt.clf()

    plt.figure(figsize=(32, 32))
    if slot_per_pod:
        ax = sns.boxplot(x="ranks", y="startup_time", hue=hue, data=df, whis=[5, 95])
    else:
        ax = sns.boxplot(
            x="ranks",
            y="startup_time",
            hue="scheduler",
            data=df,
            whis=[5, 95],
            palette=palette,
        )

    plt.title("AMG MPIJob startup time")
    plt.legend([], [], frameon=False)
    ax.set_xlabel("MPI Ranks", fontsize=20)
    ax.set_ylabel("Startup time (s)", fontsize=20)
    ax.set_xticklabels(ax.get_xmajorticklabels(), fontsize=18)
    ax.set_yticklabels(ax.get_yticks(), fontsize=18)
    plt.savefig(f"startup_{plotname}.pdf")
    plt.clf()

    plt.figure(figsize=(32, 32))
    if slot_per_pod:
        ax = sns.boxplot(x="ranks", y="runtime", hue=hue, data=df, whis=[5, 95])
    else:
        ax = sns.boxplot(
            x="ranks",
            y="runtime",
            hue="scheduler",
            data=df,
            whis=[5, 95],
            palette=palette,
        )

    plt.title("AMG MPIJob runtime")
    plt.legend([], [], frameon=False)
    ax.set_xlabel("MPI Ranks", fontsize=20)
    ax.set_ylabel("Runtime (s)", fontsize=20)
    ax.set_xticklabels(ax.get_xmajorticklabels(), fontsize=18)
    ax.set_yticklabels(ax.get_yticks(), fontsize=18)
    plt.savefig(f"runtime_{plotname}.pdf")
    plt.clf()

    plt.figure(figsize=(32, 32))
    if slot_per_pod:
        ax = sns.boxplot(x="ranks", y="end_to_end_time", hue=hue, data=df, whis=[5, 95])
    else:
        ax = sns.boxplot(
            x="ranks",
            y="end_to_end_time",
            hue="scheduler",
            data=df,
            whis=[5, 95],
            palette=palette,
        )

    plt.title("AMG MPIJob end-to-end time")
    plt.legend([], [], frameon=False)
    ax.set_xlabel("MPI Ranks", fontsize=20)
    ax.set_ylabel("End-to-end time (s)", fontsize=20)
    ax.set_xticklabels(ax.get_xmajorticklabels(), fontsize=18)
    ax.set_yticklabels(ax.get_yticks(), fontsize=18)
    plt.savefig(f"end_to_end_time_{plotname}.pdf")
    plt.clf()

    plt.figure(figsize=(32, 32))
    if slot_per_pod:
        ax = sns.boxplot(x="ranks", y="real", hue=hue, data=df, whis=[5, 95])
    else:
        ax = sns.boxplot(
            x="ranks",
            y="real",
            hue="scheduler",
            data=df,
            whis=[5, 95],
            palette=palette,
        )

    plt.title("AMG Unix real time")
    plt.legend([], [], frameon=False)
    ax.set_xlabel("MPI Ranks", fontsize=20)
    ax.set_ylabel("Unix real time (s)", fontsize=20)
    ax.set_xticklabels(ax.get_xmajorticklabels(), fontsize=18)
    ax.set_yticklabels(ax.get_yticks(), fontsize=18)
    plt.savefig(f"real_{plotname}.pdf")
    plt.clf()


def main():
    parser = argparse.ArgumentParser(description="Process AMG outputs")
    parser.add_argument(
        "-output_dir",
        "--output_dir",
        required=True,
        help="directory with the experimental outputs",
    )
    parser.add_argument(
        "-plotname",
        "--plotname",
        required=True,
        default="",
        help="base name for plot file",
    )
    parser.add_argument(
        "-slot_per_pod",
        "--slot_per_pod",
        required=False,
        action="store_true",
        help="generate slot per pod plot",
    )
    args = parser.parse_args()

    output_entries = args.output_dir.split(",")
    df = pd.concat(gather_outputs(x) for x in output_entries)
    plot_outputs(df, args.plotname, args.slot_per_pod)


if __name__ == "__main__":
    main()
