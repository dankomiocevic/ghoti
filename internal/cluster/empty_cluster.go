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

func (c *EmptyCluster) Remove(a string) error {
	return nil
}

func (c *EmptyCluster) Bootstrap() raft.Future {
	return &EmptyFuture{}
}

func (c *EmptyCluster) IsLeader() bool {
	return true
}

func (c *EmptyCluster) GetLeader() string {
	return ""
}

func (c *EmptyCluster) Shutdown() raft.Future {
	return &EmptyFuture{}
}

func (c *EmptyCluster) state() raft.RaftState {
	return raft.Leader
}
