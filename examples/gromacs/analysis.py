#!/usr/bin/env python3

import argparse
import pandas as pd
import os
from sklearn import linear_model
import matplotlib.pyplot as plt


def main():
    parser = argparse.ArgumentParser(
            description='Analyze GROMACS benchmark results')
    parser.add_argument("in_fname",
                        help="the input CSV file")
    parser.add_argument("-p","--plot", action="store_true")
    parser.add_argument("-l","--latex", action="store_true")
    parser.add_argument("-v","--verbose", action="store_true")

    args = parser.parse_args()

    pdf = pd.read_csv(args.in_fname)

    dpdf = pdf.groupby(['n-nodes','mode','ranks']).describe()

    for metric in set(pdf.columns.tolist()) - set(['n-nodes','mode','ranks']):
        print("> {}".format(metric))
        print(dpdf[metric])


    # build sample
    feature_columns = ["n-subnets","max-node-occ","max-subnet-size","imbalance"]
    target_column = "ns/day"
    sample = dpdf[feature_columns + [target_column]].xs("50%", level=1, axis=1).reset_index()
    print(sample)

    # pretty printing
    pp_index=["ranks","mode"]
    pp_headers = {
        "n-subnets":"n. subnets",
        "max-node-occ":"max. node occ.",
        "max-subnet-size":"max. subnet size",
        "imbalance":"imbalance",
        "mode":"scheduler",
        "ranks":"MPI ranks"}
    pp_sample = sample.drop('n-nodes',axis=1).set_index(pp_index).sort_values(by=pp_index).astype(
        {"n-subnets":int, "max-node-occ":int, "max-subnet-size":int}).rename(columns = pp_headers)
    print(pp_sample)

    # dump pretty-printed sample into latex table
    if(args.latex):
        pp_sample.to_latex(buf="perf-model.tex", multirow=True,
        caption="Potential correlations to explain the relative performance of different schedulers.",
        label="tab:perf-model")

    # plot correlations
    if(args.plot):
        colors = {'default':'orange', 'flux':'blue', 'flux-ws':'gray'}
        if not os.path.exists('plots'):
            os.makedirs('plots')
        for f in feature_columns:
            for ranks in sample["ranks"]:
                grpd_sample = sample.loc[sample["ranks"] == ranks].groupby('mode')
                fig, ax = plt.subplots()
                for key, group in grpd_sample:
                    group.plot(ax=ax, kind='scatter', x=f, y=target_column, label=key, color=colors[key])
                plt.savefig('plots/{}-rnks{}.png'.format(f, ranks))
                plt.close()


if __name__ == '__main__':
    main()
