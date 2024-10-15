package cluster

import (
	"math"
	"testing"
	"time"

	"github.com/hashicorp/raft"
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

	// Exponential retry until set as leader
	baseDelay := 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if c.state() == raft.Leader {
			return
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	t.Fatalf("Single node cluster node not set as leader")
}

func TestClusterWrongConfig(t *testing.T) {
	config := ClusterConfig{Node: "node1", ManagerJoin: "", User: "", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:1234", Bind: ""}

	_, err := NewCluster(config)
	if err == nil {
		t.Fatalf("Cluster creation must fail")
	}
}

func TestClusterMissingBind(t *testing.T) {
	config := ClusterConfig{Node: "node1", ManagerJoin: "", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:1234", Bind: ""}

	c, err := NewCluster(config)
	if err != nil {
		t.Fatalf("Failed to create cluster: %s", err)
	}

	err = c.Start()

	if err == nil {
		t.Fatalf("Cluster should have failed to initialize")
	}
}
