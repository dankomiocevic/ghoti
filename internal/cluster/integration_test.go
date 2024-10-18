package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/raft"
)

func TestClusterMultiNode(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test on Short mode")
	}

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
		if node_one.state() == raft.Leader {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if node_one.state() != raft.Leader {
		t.Fatalf("Node one not set as leader, state: %s", node_one.state())
	}

	err = node_two.Start()
	if err != nil {
		t.Fatalf("failed to start node two: %s", err)
	}

	// Exponential retry until node 2 is set as follower
	baseDelay = 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if node_two.state() == raft.Follower {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if node_two.state() != raft.Follower {
		t.Fatalf("Node two not set as follower, state: %s", node_two.state())
	}

	err = node_three.Start()
	if err != nil {
		t.Fatalf("failed to start node three: %s", err)
	}

	// Exponential retry until node 3 is set as follower
	baseDelay = 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if node_three.state() == raft.Follower {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if node_three.state() != raft.Follower {
		t.Fatalf("Node three not set as follower, state: %s", node_three.state())
	}

	// Shutting down leader node
	node_one.Shutdown().Error()

	// Exponential retry until another node becomes leader
	baseDelay = 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if node_two.state() == raft.Leader || node_three.state() == raft.Leader {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if node_two.state() != raft.Leader && node_three.state() != raft.Leader {
		t.Fatalf("Node two or three not set as leader, state_two: %s, state_three: %s", node_two.state(), node_three.state())
	}

	// Identify the leader and follower node
	var leaderNodeConfig, followerNodeConfig ClusterConfig
	if node_two.IsLeader() {
		leaderNodeConfig = config_two
		followerNodeConfig = config_three
	} else {
		leaderNodeConfig = config_three
		followerNodeConfig = config_two
	}

	// Adding node one again
	config_one = ClusterConfig{Node: "node1", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:2226", ManagerJoin: leaderNodeConfig.ManagerAddr, Bind: "localhost:1111"}
	node_one, err = NewCluster(config_one)
	if err != nil {
		t.Fatalf("failed to create new cluster node one: %s", err)
	}
	err = node_one.Start()
	if err != nil {
		t.Fatalf("failed to restart node one: %s", err)
	}

	// Exponential retry until node 1 is set as follower and gets leader info
	baseDelay = 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if node_one.state() == raft.Follower && len(node_one.GetLeader()) > 0 {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if node_one.state() != raft.Follower {
		t.Fatalf("Node one not set as follower, state: %s", node_one.state())
	}
	if node_two.GetLeader() != node_one.GetLeader() {
		t.Fatalf("Leader does not match: node_one -> %s, node_two -> %s", node_one.GetLeader(), node_two.GetLeader())
	}

	// Shutting down again node
	node_one.Shutdown().Error()

	// Exponential retry until another node becomes leader
	baseDelay = 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if node_two.state() == raft.Leader || node_three.state() == raft.Leader {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if node_two.state() != raft.Leader && node_three.state() != raft.Leader {
		t.Fatalf("Node two or three not set as leader, state_two: %s, state_three: %s", node_two.state(), node_three.state())
	}

	// Identify the leader and follower node
	if node_two.IsLeader() {
		leaderNodeConfig = config_two
		followerNodeConfig = config_three
	} else {
		leaderNodeConfig = config_three
		followerNodeConfig = config_two
	}

	// Request to remove node_one sent to follower
	b, err := json.Marshal(map[string]string{"id": config_one.Node})
	if err != nil {
		t.Fatalf("Failed to generate JSON: %s", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/remove", followerNodeConfig.ManagerAddr), bytes.NewReader(b))
	if err != nil {
		t.Fatalf("Failed to create request: %s", err)
	}
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth(followerNodeConfig.User, followerNodeConfig.Pass)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to do request: %s", err)
	}

	if resp.Status == "200 OK" {
		t.Fatalf("Request to remove sent to follower must fail")
	}

	resp.Body.Close()

	// Request to remove node_one sent to leader
	b, err = json.Marshal(map[string]string{"id": config_one.Node})
	if err != nil {
		t.Fatalf("Failed to generate JSON: %s", err)
	}

	req, err = http.NewRequest("POST", fmt.Sprintf("http://%s/remove", leaderNodeConfig.ManagerAddr), bytes.NewReader(b))
	if err != nil {
		t.Fatalf("Failed to create request: %s", err)
	}
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth(leaderNodeConfig.User, leaderNodeConfig.Pass)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to do request: %s", err)
	}

	if resp.Status != "200 OK" {
		t.Fatalf("Request to remove sent to leader must succeed")
	}

	resp.Body.Close()
}
