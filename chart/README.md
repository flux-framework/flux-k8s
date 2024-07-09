# Scheduler-plugins as a second scheduler in cluster

This was moved from the [https://github.com/kubernetes-sigs/scheduler-plugins](kubernetes-sigs/scheduler-plugins) repository to support Fluence here.

## Table of Contents

<!-- toc -->
- [Installation](#installation)
  - [Prerequisites](#prerequisites)
  - [Installing the chart](#installing-the-chart)
    - [Install chart using Helm v3.0+](#install-chart-using-helm-v30)
    - [Verify that scheduler and plugin-controller pod are running properly.](#verify-that-scheduler-and-plugin-controller-pod-are-running-properly)
  - [Configuration](#configuration)
<!-- /toc -->

## Installation

Quick start instructions for the setup and configuration of as-a-second-scheduler using Helm.

### Prerequisites

- [Helm](https://helm.sh/docs/intro/quickstart/#install-helm)

### Installing the chart

#### Install chart using Helm v3.0+

```bash
$ git clone https://github.com/flux-framework/fluence.git
```

Then follow the instructions in the README.