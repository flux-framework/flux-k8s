# Overview

Project to manage Flux tasks needed to standardize kubernetes HPC scheduling interfaces

## Installing the chart

More detail will be added here about installing the chart. You will
be using the [install-as-a-second-scheduler](https://github.com/kubernetes-sigs/scheduler-plugins/tree/master/manifests/install/charts/as-a-second-scheduler)
charts. Fluence-specific values are detailed below.

### Fluence specific values

In `values.yaml` it is possible to customize the container image, already defaulted to the latest release, and the allocation policy
used by the scheduler.
Most common options are:

- `lonode`: choose the nodes with lower ID first. Can be compared to packing
- `low`: choose cores with lowest IDs from multiple nodes. Can be compared to spread process-to-resource placement

## Maturity Level

<!-- Check one of the values: Sample, Alpha, Beta, GA -->

- [x] Sample (for demonstrating and inspiring purpose)
- [ ] Alpha (used in companies for pilot projects)
- [ ] Beta (used in companies and developed actively)
- [ ] Stable (used in companies for production workloads)

<!-- TODO: write some useful KubeFlux documentation -->
