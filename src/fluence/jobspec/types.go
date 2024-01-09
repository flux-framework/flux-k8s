/*
Copyright © 2021 IBM Corporation

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

package jobspec

type Version struct {
	Version   int
	Resources []Resource `yaml:"resources,omitempty"`
}

type Resource struct {
	Type  string     `yaml:"type"`
	Count int64      `yaml:"count"`
	Label string     `yaml:"label,omitempty"`
	With  []Resource `yaml:"with,omitempty"`
}

type System struct {
	Duration int64 `yaml:"duration,omitempty"`
}

type Attribute struct {
	SystemAttr System `yaml:"system,omitempty"`
}

type Count struct {
	PerSlot int64 `yaml:"per_slot,omitempty"`
}

type Task struct {
	Command []string `yaml:"command,flow"`
	Slot    string   `yaml:"slot"`
	Counts  Count    `yaml:"count"`
}

type JobSpec struct {
	Version    Version   `yaml:"version,inline"`
	Attributes Attribute `yaml:"attributes,omitempty"`
	Tasks      []Task    `yaml:"tasks,omitempty"`
}
