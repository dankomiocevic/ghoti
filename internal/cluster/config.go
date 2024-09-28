package cluster

type ClusterConfig struct {
	Node        string
	Join        string
	User        string
	Pass        string
	Bind        string
	ManagerType string
	ManagerAddr string
}
