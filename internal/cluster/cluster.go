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
	Join(string, string) error
	State() raft.RaftState
}

type RaftCluster struct {
	config  ClusterConfig
	raft    *raft.Raft
	manager MembershipManager
}

func NewCluster(config ClusterConfig) Cluster {
	return &RaftCluster{config: config}
}

func (c *RaftCluster) Start() error {
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(c.config.Node)

	addr, err := net.ResolveTCPAddr("tcp", c.config.Bind)
	if err != nil {
		return err
	}

	transport, err := raft.NewTCPTransport(c.config.Bind, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return err
	}

	snapshots := raft.NewInmemSnapshotStore()
	logStore := raft.NewInmemStore()
	stableStore := raft.NewInmemStore()

	ra, err := raft.NewRaft(config, nil, logStore, stableStore, snapshots, transport)
	if err != nil {
		return fmt.Errorf("new raft: %s", err)
	}
	c.raft = ra

	if len(c.config.Join) < 1 {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      config.LocalID,
					Address: transport.LocalAddr(),
				},
			},
		}
		ra.BootstrapCluster(configuration)
	}

	return nil
}

func (c *RaftCluster) State() raft.RaftState {
	return c.raft.State()
}

func (c *RaftCluster) Join(nodeID string, addr string) error {
	configFuture := c.raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		fmt.Printf("failed to get raft configuration: %v", err)
		return err
	}

	for _, srv := range configFuture.Configuration().Servers {
		if srv.ID == raft.ServerID(c.config.Join) || srv.Address == raft.ServerAddress(addr) {
			if srv.Address == raft.ServerAddress(addr) && srv.ID == raft.ServerID(c.config.Join) {
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
