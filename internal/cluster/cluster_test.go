package cluster

import (
	"testing"
	"time"
)

func TestClusterSingleNode(t *testing.T) {
	config := ClusterConfig{Node: "node1", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:1234", ManagerJoin: "", Bind: "localhost:1235"}

	c, err := NewCluster(config)
	if err != nil {
		t.Fatalf("failed to create new cluster: %s", err)
	}

	err = c.Start()

	if err != nil {
		t.Fatalf("failed to start cluster: %s", err)
	}

	// Give time for the node to bootstrap as leader
	time.Sleep(200 * time.Millisecond)

	if !c.IsLeader() {
		t.Fatalf("Single node cluster node not set as leader")
	}

	err = c.Shutdown()
	if err != nil {
		t.Fatalf("failed to shutdown cluster: %s", err)
	}
}

func TestClusterWrongConfig(t *testing.T) {
	config := ClusterConfig{Node: "node1", ManagerJoin: "", User: "", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:1234", Bind: ""}

	_, err := NewCluster(config)
	if err == nil {
		t.Fatalf("Cluster creation must fail")
	}
}

func TestClusterMissingManagerAddr(t *testing.T) {
	config := ClusterConfig{Node: "node1", ManagerJoin: "", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "", Bind: ""}

	c, err := NewCluster(config)
	if err != nil {
		t.Fatalf("Failed to create cluster: %s", err)
	}

	err = c.Start()

	if err == nil {
		t.Fatalf("Cluster should have failed to initialize")
	}
}
