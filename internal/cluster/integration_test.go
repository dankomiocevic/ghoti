package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func waitForLeader(t *testing.T, c *BullyCluster, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.IsLeader() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Node %s did not become leader within timeout", c.nodeID)
}

func waitForFollower(t *testing.T, c *BullyCluster, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !c.IsLeader() && c.GetLeader() != "" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Node %s did not become follower within timeout", c.nodeID)
}

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

	err = nodeOne.Start()
	if err != nil {
		t.Fatalf("failed to start cluster node one: %s", err)
	}

	waitForLeader(t, nodeOne, 5*time.Second)

	nodeTwo, err := NewCluster(configTwo)
	if err != nil {
		t.Fatalf("failed to create new cluster node two: %s", err)
	}

	err = nodeTwo.Start()
	if err != nil {
		t.Fatalf("failed to start node two: %s", err)
	}

	waitForFollower(t, nodeTwo, 5*time.Second)

	nodeThree, err := NewCluster(configThree)
	if err != nil {
		t.Fatalf("failed to create new cluster node three: %s", err)
	}

	err = nodeThree.Start()
	if err != nil {
		t.Fatalf("failed to start node three: %s", err)
	}

	waitForFollower(t, nodeThree, 5*time.Second)

	// Verify leader is node1
	if !nodeOne.IsLeader() {
		t.Fatalf("Node one should be leader")
	}
	if nodeTwo.GetLeader() != "node1" {
		t.Fatalf("Node two should see node1 as leader, got %s", nodeTwo.GetLeader())
	}
	if nodeThree.GetLeader() != "node1" {
		t.Fatalf("Node three should see node1 as leader, got %s", nodeThree.GetLeader())
	}

	// Shutdown the leader (node1)
	nodeOne.Shutdown()

	// Wait for the bully election to complete
	// node3 has the highest ID so it should become leader
	time.Sleep(5 * time.Second)

	// In bully algorithm, the highest ID node becomes leader
	// node3 > node2, so node3 should be leader
	if !nodeThree.IsLeader() {
		t.Fatalf("Node three should be the new leader after node one shutdown")
	}
	if nodeTwo.GetLeader() != "node3" {
		t.Fatalf("Node two should see node3 as leader, got %s", nodeTwo.GetLeader())
	}

	// Identify the leader and follower node configs
	leaderNodeConfig := configThree
	followerNodeConfig := configTwo

	// Request to remove nodeOne sent to follower (should fail)
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

	// Request to remove nodeOne sent to leader (should succeed)
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
	configOne = ClusterConfig{Node: "node1", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:2226", ManagerJoin: leaderNodeConfig.ManagerAddr, Bind: "localhost:1119"}
	nodeOne, err = NewCluster(configOne)
	if err != nil {
		t.Fatalf("failed to create new cluster node one: %s", err)
	}
	err = nodeOne.Start()
	if err != nil {
		t.Fatalf("failed to restart node one: %s", err)
	}

	waitForFollower(t, nodeOne, 5*time.Second)

	if nodeOne.GetLeader() != nodeTwo.GetLeader() {
		t.Fatalf("Leader does not match: nodeOne -> %s, nodeTwo -> %s", nodeOne.GetLeader(), nodeTwo.GetLeader())
	}

	// Clean up
	nodeOne.Shutdown()
	nodeTwo.Shutdown()
	nodeThree.Shutdown()
}
