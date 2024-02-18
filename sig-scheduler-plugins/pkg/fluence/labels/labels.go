package labels

// Labels to be shared between different components

const (
	// https://github.com/kubernetes-sigs/scheduler-plugins/blob/master/apis/scheduling/v1alpha1/types.go#L109
	PodGroupLabel = "scheduling.x-k8s.io/pod-group"

	// TODO add more labels here, to be discovered used later
	//PodGroupNameLabel = "fluence.pod-group"
	PodGroupSizeLabel = "fluence.group-size"
)
