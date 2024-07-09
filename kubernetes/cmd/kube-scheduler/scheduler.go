/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"os"

	"k8s.io/component-base/cli"
	_ "k8s.io/component-base/logs/json/register" // for JSON log format registration
	_ "k8s.io/component-base/metrics/prometheus/clientgo"
	_ "k8s.io/component-base/metrics/prometheus/version" // for version metric registration
	"k8s.io/kubernetes/cmd/kube-scheduler/app"

	// We add fluence to core kubernetes to build into the scheduler command
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/fluence"
)

func main() {
	// Fluence is actually added as an in-tree plugin here
	command := app.NewSchedulerCommand(
		app.WithPlugin(fluence.Name, fluence.New),
	)
	code := cli.Run(command)
	os.Exit(code)
}
