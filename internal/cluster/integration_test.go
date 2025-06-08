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

	configOne := ClusterConfig{Node: "node1", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:2222", ManagerJoin: "", Bind: "localhost:1111"}
	configTwo := ClusterConfig{Node: "node2", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:2223", ManagerJoin: "localhost:2222", Bind: "localhost:1112"}
	configThree := ClusterConfig{Node: "node3", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:2225", ManagerJoin: "localhost:2222", Bind: "localhost:1113"}

	nodeOne, err := NewCluster(configOne)
	if err != nil {
		t.Fatalf("failed to create new cluster node one: %s", err)
	}

	nodeTwo, err := NewCluster(configTwo)
	if err != nil {
		t.Fatalf("failed to create new cluster node two: %s", err)
	}

	nodeThree, err := NewCluster(configThree)
	if err != nil {
		t.Fatalf("failed to create new cluster node three: %s", err)
	}

	err = nodeOne.Start()
	if err != nil {
		t.Fatalf("failed to start cluster node one: %s", err)
	}

	// Exponential retry until set as leader
	baseDelay := 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if nodeOne.state() == raft.Leader {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if nodeOne.state() != raft.Leader {
		t.Fatalf("Node one not set as leader, state: %s", nodeOne.state())
	}

	err = nodeTwo.Start()
	if err != nil {
		t.Fatalf("failed to start node two: %s", err)
	}

	// Exponential retry until node 2 is set as follower
	baseDelay = 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if nodeTwo.state() == raft.Follower {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if nodeTwo.state() != raft.Follower {
		t.Fatalf("Node two not set as follower, state: %s", nodeTwo.state())
	}

	err = nodeThree.Start()
	if err != nil {
		t.Fatalf("failed to start node three: %s", err)
	}

	// Exponential retry until node 3 is set as follower
	baseDelay = 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if nodeThree.state() == raft.Follower {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if nodeThree.state() != raft.Follower {
		t.Fatalf("Node three not set as follower, state: %s", nodeThree.state())
	}

	// Shutting down leader node
	nodeOne.Shutdown().Error()

	// Exponential retry until another node becomes leader
	baseDelay = 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if nodeTwo.state() == raft.Leader || nodeThree.state() == raft.Leader {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if nodeTwo.state() != raft.Leader && nodeThree.state() != raft.Leader {
		t.Fatalf("Node two or three not set as leader, state_two: %s, state_three: %s", nodeTwo.state(), nodeThree.state())
	}

	// Identify the leader and follower node
	var leaderNodeConfig, followerNodeConfig ClusterConfig
	if nodeTwo.IsLeader() {
		leaderNodeConfig = configTwo
		followerNodeConfig = configThree
	} else {
		leaderNodeConfig = configThree
		followerNodeConfig = configTwo
	}

	// Adding node one again
	configOne = ClusterConfig{Node: "node1", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:2226", ManagerJoin: leaderNodeConfig.ManagerAddr, Bind: "localhost:1111"}
	nodeOne, err = NewCluster(configOne)
	if err != nil {
		t.Fatalf("failed to create new cluster node one: %s", err)
	}
	err = nodeOne.Start()
	if err != nil {
		t.Fatalf("failed to restart node one: %s", err)
	}

	// Exponential retry until node 1 is set as follower and gets leader info
	baseDelay = 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if nodeOne.state() == raft.Follower && len(nodeOne.GetLeader()) > 0 {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if nodeOne.state() != raft.Follower {
		t.Fatalf("Node one not set as follower, state: %s", nodeOne.state())
	}
	if nodeTwo.GetLeader() != nodeOne.GetLeader() {
		t.Fatalf("Leader does not match: nodeOne -> %s, nodeTwo -> %s", nodeOne.GetLeader(), nodeTwo.GetLeader())
	}

	// Shutting down again node
	nodeOne.Shutdown().Error()

	// Exponential retry until another node becomes leader
	baseDelay = 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if nodeTwo.state() == raft.Leader || nodeThree.state() == raft.Leader {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if nodeTwo.state() != raft.Leader && nodeThree.state() != raft.Leader {
		t.Fatalf("Node two or three not set as leader, state_two: %s, state_three: %s", nodeTwo.state(), nodeThree.state())
	}

	// Identify the leader and follower node
	if nodeTwo.IsLeader() {
		leaderNodeConfig = configTwo
		followerNodeConfig = configThree
	} else {
		leaderNodeConfig = configThree
		followerNodeConfig = configTwo
	}

	// Request to remove nodeOne sent to follower
	b, err := json.Marshal(map[string]string{"id": configOne.Node})
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

	// Request to remove nodeOne sent to leader
	b, err = json.Marshal(map[string]string{"id": configOne.Node})
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

	// Adding node one again
	configOne = ClusterConfig{Node: "node1", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:2229", ManagerJoin: leaderNodeConfig.ManagerAddr, Bind: "localhost:1119"}
	nodeOne, err = NewCluster(configOne)
	if err != nil {
		t.Fatalf("failed to create new cluster node one: %s", err)
	}
	err = nodeOne.Start()
	if err != nil {
		t.Fatalf("failed to restart node one: %s", err)
	}

	// Exponential retry until node 1 is set as follower and gets leader info
	baseDelay = 100 * time.Millisecond
	for i := 0; i < 7; i++ {
		if nodeOne.state() == raft.Follower && len(nodeOne.GetLeader()) > 0 {
			break
		}

		secRetry := math.Pow(2, float64(i))
		delay := time.Duration(secRetry) * baseDelay
		time.Sleep(delay)
	}
	if nodeOne.state() != raft.Follower {
		t.Fatalf("Node one not set as follower, state: %s", nodeOne.state())
	}
	if nodeTwo.GetLeader() != nodeOne.GetLeader() {
		t.Fatalf("Leader does not match: nodeOne -> %s, nodeTwo -> %s", nodeOne.GetLeader(), nodeTwo.GetLeader())
	}

	// Request to join again for nodeOne
	err = requestToJoin(leaderNodeConfig.ManagerAddr, configOne.Bind, configOne.Node, configOne.User, configOne.Pass)
	if err != nil {
		t.Fatalf("Request to join existing node must fail silently")
	}
}
