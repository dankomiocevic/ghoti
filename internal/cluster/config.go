package cluster

type ClusterConfig struct {
	Node        string
	Bind        string
	User        string
	Pass        string
	ManagerType string
	ManagerAddr string
	ManagerJoin string

	// LeaderEnabled exposes the GET /leader endpoint on the cluster
	// manager server. It reports whether this node is the cluster leader
	// so an external load balancer can route traffic to the leader only.
	LeaderEnabled bool
}
