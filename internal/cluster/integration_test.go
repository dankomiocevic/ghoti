package cluster

import (
	"math"
	"testing"
	"time"

	"github.com/hashicorp/raft"
)

func TestClusterMultiNode(t *testing.T) {
	config_one := ClusterConfig{Node: "node1", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:2222", ManagerJoin: "", Bind: "localhost:1111"}
	config_two := ClusterConfig{Node: "node2", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:2223", ManagerJoin: "localhost:2222", Bind: "localhost:1112"}
	config_three := ClusterConfig{Node: "node3", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:2225", ManagerJoin: "localhost:2222", Bind: "localhost:1113"}

	node_one, err := NewCluster(config_one)
	if err != nil {
		t.Fatalf("failed to create new cluster node one: %s", err)
	}

	node_two, err := NewCluster(config_two)
	if err != nil {
		t.Fatalf("failed to create new cluster node two: %s", err)
	}

	node_three, err := NewCluster(config_three)
	if err != nil {
		t.Fatalf("failed to create new cluster node three: %s", err)
	}

	err = node_one.Start()
	if err != nil {
		t.Fatalf("failed to start cluster node one: %s", err)
	}

	// Exponential retry until set as leader
	baseDelay := 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if node_one.State() == raft.Leader {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if node_one.State() != raft.Leader {
		t.Fatalf("Node one not set as leader, state: %s", node_one.State())
	}

	err = node_two.Start()
	if err != nil {
		t.Fatalf("failed to start node two: %s", err)
	}

	// Exponential retry until node 2 is set as follower
	baseDelay = 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if node_two.State() == raft.Follower {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if node_two.State() != raft.Follower {
		t.Fatalf("Node two not set as follower, state: %s", node_two.State())
	}

	err = node_three.Start()
	if err != nil {
		t.Fatalf("failed to start node three: %s", err)
	}

	// Exponential retry until node 3 is set as follower
	baseDelay = 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if node_three.State() == raft.Follower {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if node_three.State() != raft.Follower {
		t.Fatalf("Node three not set as follower, state: %s", node_three.State())
	}

	// Shutting down leader node
	node_one.Shutdown()
	// Exponential retry until another node becomes leader
	baseDelay = 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if node_two.State() == raft.Leader || node_three.State() == raft.Leader {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if node_two.State() != raft.Leader && node_three.State() != raft.Leader {
		t.Fatalf("Node two or three not set as leader, state_two: %s, state_three: %s", node_two.State(), node_three.State())
	}
}
