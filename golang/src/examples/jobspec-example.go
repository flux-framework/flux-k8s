/*
Copyright 2017 The Kubernetes Authors.
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

// Note: the example only works with the code within the same release/branch.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	resource "k8s.io/apimachinery/pkg/api/resource"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"jobspec"
)

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}
	// clientset, err := kubernetes.NewForConfig(config)
	// if err != nil {
	// 	panic(err)
	// }

	coreclient, err := corev1client.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	podspec := &apiv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "demo-pod-creation",
		},
		Spec: apiv1.PodSpec{
			// SchedulerName: "my-scheduler",
			Containers: []apiv1.Container{
				{
					Name:    "busybox",
					Image:   "busybox",
					Command: []string{"sleep", "500"},
					Resources: apiv1.ResourceRequirements{
						Limits: apiv1.ResourceList{
							"cpu":    resource.MustParse("1"),
							"memory": resource.MustParse("7Mi"),
						},
						Requests: apiv1.ResourceList{
							"cpu":    resource.MustParse("1"),
							"memory": resource.MustParse("7Mi"),
						},
					},
				},
			},
			RestartPolicy: "Never",
		},
	}

	newpod, err := coreclient.Pods("default").Create(podspec)
	if err != nil {
		panic(err.Error())
	}

	fmt.Printf("Created pod %q.\n", newpod.GetObjectMeta().GetName())

	fmt.Println("Creating jobspec from pod " + newpod.GetObjectMeta().GetName())

	pr := jobspec.InspectPodInfo(newpod)

	fmt.Println("Dumping jobspec ")
	path := jobspec.CreateJobSpecYaml(pr)

	fmt.Println("Jobspec saved: " + path)

	prompt()
	fmt.Println("Now deleting the pod.. ")
	err = coreclient.Pods("default").Delete(newpod.Name, &metav1.DeleteOptions{})
	if err != nil {
		panic(err.Error())
	}

}

func prompt() {
	fmt.Printf("-> Press Return key to continue.")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		break
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
	fmt.Println()
}

func createDeployment(clientset kubernetes.Clientset) {

	deploymentsClient := clientset.AppsV1().Deployments(apiv1.NamespaceDefault)
	count := 2
	for index := 0; index < count; index++ {
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "demo-deployment-" + strconv.Itoa(index),
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(2),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "demo",
					},
				},
				Template: apiv1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": "demo",
						},
					},
					Spec: apiv1.PodSpec{
						SchedulerName: "my-scheduler",
						Containers: []apiv1.Container{
							{
								Name:    "web",
								Image:   "nginx:1.12",
								Command: []string{"echo Hello && sleep 3600"},
								Resources: apiv1.ResourceRequirements{
									Limits: apiv1.ResourceList{
										"cpu":    resource.MustParse("0.001"),
										"memory": resource.MustParse("1G"),
									},
									Requests: apiv1.ResourceList{
										"cpu":    resource.MustParse("0.001"),
										"memory": resource.MustParse("1G"),
									},
								},
							},
						},
					},
				},
			},
		}

		// Create Deployment
		fmt.Println("Creating deployment...")
		result, err := deploymentsClient.Create(deployment)
		if err != nil {
			panic(err)
		}
		fmt.Printf("Created deployment %q.\n", result.GetObjectMeta().GetName())
	}

	// Delete Deployment
	prompt()
	fmt.Println("Deleting deployments...")
	for index := 0; index < count; index++ {
		deletePolicy := metav1.DeletePropagationForeground
		if err := deploymentsClient.Delete("demo-deployment-"+strconv.Itoa(index), &metav1.DeleteOptions{
			PropagationPolicy: &deletePolicy,
		}); err != nil {
			panic(err)
		}
		fmt.Println("Deleted deployment.")
	}

}
func int32Ptr(i int32) *int32 { return &i }
