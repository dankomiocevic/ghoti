package cluster

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/hashicorp/raft"
)

type Cluster interface {
	Start() error
	Join(string, string, string) error
	Bootstrap() raft.Future
	State() raft.RaftState
}

type RaftCluster struct {
	config     ClusterConfig
	raftConfig *raft.Config
	transport  *raft.NetworkTransport
	raft       *raft.Raft
	manager    MembershipManager
}

func NewCluster(config ClusterConfig) (Cluster, error) {
	cluster := &RaftCluster{config: config}
	manager, err := GetManager(&config, cluster)
	if err != nil {
		return nil, err
	}

	cluster.manager = manager
	return cluster, nil
}

func (c *RaftCluster) Start() error {
	c.raftConfig = raft.DefaultConfig()
	c.raftConfig.LocalID = raft.ServerID(c.config.Node)

	addr, err := net.ResolveTCPAddr("tcp", c.config.Bind)
	if err != nil {
		return err
	}

	transport, err := raft.NewTCPTransport(c.config.Bind, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return err
	}
	c.transport = transport

	snapshots := raft.NewInmemSnapshotStore()
	logStore := raft.NewInmemStore()
	stableStore := raft.NewInmemStore()

	ra, err := raft.NewRaft(c.raftConfig, nil, logStore, stableStore, snapshots, transport)
	if err != nil {
		return fmt.Errorf("new raft: %s", err)
	}
	c.raft = ra

	c.manager.Start()
	defer c.manager.Close()

	return nil
}

func (c *RaftCluster) State() raft.RaftState {
	return c.raft.State()
}

func (c *RaftCluster) Bootstrap() raft.Future {
	configuration := raft.Configuration{
		Servers: []raft.Server{
			{
				ID:      c.raftConfig.LocalID,
				Address: c.transport.LocalAddr(),
			},
		},
	}

	return c.raft.BootstrapCluster(configuration)
}

func (c *RaftCluster) Join(nodeID string, addr string, join string) error {
	configFuture := c.raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		fmt.Printf("failed to get raft configuration: %v", err)
		return err
	}

	for _, srv := range configFuture.Configuration().Servers {
		if srv.ID == raft.ServerID(join) || srv.Address == raft.ServerAddress(addr) {
			if srv.Address == raft.ServerAddress(addr) && srv.ID == raft.ServerID(join) {
				fmt.Printf("node %s at %s already member of cluster, ignoring join request", nodeID, addr)
				return nil
			}

			future := c.raft.RemoveServer(srv.ID, 0, 0)
			if err := future.Error(); err != nil {
				return fmt.Errorf("error removing existing node %s at %s: %s", nodeID, addr, err)
			}
		}
	}

	f := c.raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(addr), 0, 0)
	if f.Error() != nil {
		return f.Error()
	}
	fmt.Printf("node %s at %s joined successfully", nodeID, addr)
	return nil
}
