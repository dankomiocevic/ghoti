package cluster

type ClusterConfig struct {
	Node        string
	Bind        string
	User        string
	Pass        string
	ManagerType string
	ManagerAddr string
	ManagerJoin string
}
