package labels

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Labels to be shared between different components

const (
	// https://github.com/kubernetes-sigs/scheduler-plugins/blob/master/apis/scheduling/v1alpha1/types.go#L109
	PodGroupLabel = "scheduling.x-k8s.io/pod-group"

	// TODO add more labels here, to be discovered used later
	//PodGroupNameLabel = "fluence.pod-group"
	PodGroupSizeLabel = "fluence.group-size"

	// Internal use
	PodGroupTimeCreated = "flunce.created-at"
)

// getTimeCreated returns the timestamp when we saw the object
func GetTimeCreated() string {

	// Set the time created for a label
	createdAt := metav1.NewMicroTime(time.Now())

	// If we get an error here, the reconciler will set the time
	var timestamp string
	timeCreated, err := createdAt.MarshalJSON()
	if err == nil {
		timestamp = string(timeCreated)
	}
	return timestamp
}
