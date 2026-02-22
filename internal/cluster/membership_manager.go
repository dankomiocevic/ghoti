package cluster

import (
	"fmt"
)

type MembershipManager interface {
	Start() error
	Close()
}

func GetManager(config *ClusterConfig, cluster Cluster) (MembershipManager, error) {
	kind := config.ManagerType
	if kind == "join_server" {
		if len(config.User) < 4 || len(config.Pass) < 4 {
			return nil, fmt.Errorf("user or password is too short")
		}
		return &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster, join: config.ManagerJoin, nodeID: config.Node, clusterBind: config.Bind}, nil
	}

	return nil, fmt.Errorf("wrong cluster manager type: %s", kind)
}
