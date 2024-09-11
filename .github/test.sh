#!/bin/bash

# This will test fluence with two jobs.
# We choose jobs as they generate output and complete, and pods
# are expected to keep running (and then would error)

set -eEu -o pipefail

# ensure upstream exists
# This test script assumes fluence image and sidecar are already built
make prepare

# Keep track of root directory to return to
here=$(pwd)

# Never will use our loaded (just built) images
cd upstream/manifests/install/charts
helm install \
  --set scheduler.image=ghcr.io/flux-framework/fluence:latest \
  --set scheduler.sidecarimage=ghcr.io/flux-framework/fluence-sidecar:latest \
  --set controller.image=ghcr.io/flux-framework/fluence-controller:latest \
  --set controller.pullPolicy=Never \
  --set scheduler.pullPolicy=Never \
  --set scheduler.sidecarPullPolicy=Never \
    schedscheduler-plugins as-a-second-scheduler/

# These containers should already be loaded into minikube
echo "Sleeping 10 seconds waiting for scheduler deploy"
sleep 10
kubectl get pods

# This will get the fluence image (which has scheduler and sidecar), which should be first
fluence_pod=$(kubectl get pods -o json | jq -r .items[0].metadata.name)
echo "Found fluence pod ${fluence_pod}"

# Show logs for debugging, if needed
echo
echo "⭐️ kubectl logs ${fluence_pod} -c sidecar"
kubectl logs ${fluence_pod} -c sidecar
echo
echo "⭐️ kubectl logs ${fluence_pod} -c scheduler-plugins-scheduler"
kubectl logs ${fluence_pod} -c scheduler-plugins-scheduler

# We now want to apply the examples
cd ${here}/examples/test_example

# Apply both example jobs
kubectl apply -f default-job.yaml

# Get them based on associated job
default_job_pod=$(kubectl get pods --selector=job-name=default-job -o json | jq -r .items[0].metadata.name)

echo
echo "Default job pod is ${default_job_pod}"
sleep 20

# Shared function to check output
function check_output {
  check_name="$1"
  actual="$2"
  expected="$3"
  if [[ "${expected}" != "${actual}" ]]; then
    echo "Expected output is ${expected}"
    echo "Actual output is ${actual}"
    exit 1
  fi
}

# Pods should be completed
kubectl get pods
kubectl describe pods

# Get output (and show)
default_output=$(kubectl logs ${default_job_pod})
default_scheduled_by=$(kubectl get pod ${default_job_pod} -o json | jq -r .spec.schedulerName)
echo
echo "Default scheduler pod output: ${default_output}"
echo "                Scheduled by: ${default_scheduled_by}"

check_output 'check-default-scheduled-by' "${default_scheduled_by}" "default-scheduler"
check_output 'check-default-output' "${default_output}" "not potato"

# And the second should be the default scheduler, but reportingComponent is empty and we see the
# result in the source -> component
reported_by=$(kubectl events --for pod/${default_job_pod} -o json  | jq -c '[ .items[] | select( .reason | contains("Scheduled")) ]' | jq -r .[0].source.component)
check_output 'reported-by-default' "${reported_by}" "default-scheduler"

# Now delete default, schedule fluence
kubectl delete -f default-job.yaml
echo
kubectl apply -f fluence-job.yaml
sleep 10

fluence_job_pod=$(kubectl get pods --selector=job-name=fluence-job -o json | jq -r .items[0].metadata.name)
echo "Fluence job pod is ${fluence_job_pod}"
fluence_output=$(kubectl logs ${fluence_job_pod})
fluence_scheduled_by=$(kubectl get pod ${fluence_job_pod} -o json | jq -r .spec.schedulerName)
echo
echo "Fluence scheduler pod output: ${fluence_output}"
echo "                Scheduled by: ${fluence_scheduled_by}"

# Check output explicitly
check_output 'check-fluence-output' "${fluence_output}" "potato"
check_output 'check-fluence-scheduled-by' "${fluence_scheduled_by}" "fluence"

# But events tell us actually what happened, let's parse throught them and find our pods
# This tells us the Event -> reason "Scheduled" and who it was reported by.
reported_by=$(kubectl events --for pod/${fluence_job_pod} -o json  | jq -c '[ .items[] | select( .reason | contains("Scheduled")) ]' | jq -r .[0].reportingComponent)
check_output 'reported-by-fluence' "${reported_by}" "fluence"
