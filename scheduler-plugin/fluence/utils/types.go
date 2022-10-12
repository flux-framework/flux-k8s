package utils

type PodSpec struct {
	Id			string
	Container	string
	Cpu			int32
	Memory		int64
	Gpu			int64
	Storage		int64
	Labels		[]string
}

type MatchRequest struct {
	Ps			PodSpec
	Request		string
	Count		int32
}

type NodeAlloc struct {
	NodeID		string
	Tasks		int32
}

type MatchResponse struct {
	PodID		string
	Nodelist	[]*NodeAlloc
	JobID		int64
}

type CancelRequest struct {
	JobID		int64
}

type CancelResponse struct {
	JobID		int64
	Error		int32
}

// The Nodes/Cluster Update Status
type NodeStatus struct {
    CpuAvail		int32
    GpuAvail 		int32
    StorageAvail 	int64
    MemoryAvail 	int64
    AllowedPods 	int64
    NodeIP 			string
    Replication 	int32
}