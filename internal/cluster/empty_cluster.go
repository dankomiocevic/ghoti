package cluster

import (
	"github.com/hashicorp/raft"
)

type EmptyCluster struct {
}

type EmptyFuture struct {
}

func (e *EmptyFuture) Error() error {
	return nil
}

func NewEmptyCluster() Cluster {
	return &EmptyCluster{}
}

func (c *EmptyCluster) Start() error {
	return nil
}

func (c *EmptyCluster) Join(a, b string) error {
	return nil
}

func (c *EmptyCluster) Bootstrap() raft.Future {
	return &EmptyFuture{}
}

func (c *EmptyCluster) State() raft.RaftState {
	return raft.Leader
}

func (c *EmptyCluster) Shutdown() raft.Future {
	return &EmptyFuture{}
}
