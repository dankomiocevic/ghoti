package cluster

import (
	"testing"
	"time"
)

// joinServerOf returns the join server backing a cluster node so a test can
// inspect how many join requests that node has served.
func joinServerOf(t *testing.T, c *BullyCluster) *joinServer {
	t.Helper()
	js, ok := c.manager.(*joinServer)
	if !ok {
		t.Fatalf("node %s is not backed by a join server", c.nodeID)
	}
	return js
}

// waitForPeers blocks until the node knows about the expected number of peers.
func waitForPeers(t *testing.T, c *BullyCluster, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(c.GetPeers()) == want && c.GetLeader() != "" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("node %s did not converge on %d peers within timeout, has %d",
		c.nodeID, want, len(c.GetPeers()))
}

// TestJoinNotificationsStopAfterConvergence guards against a join notification
// storm: once every node knows every other node, no further join traffic must
// be generated. Peers that re-broadcast a join they already know about make
// each other repeat the notification forever, which keeps every node in the
// cluster busy serving join requests until it falls over.
func TestJoinNotificationsStopAfterConvergence(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test on Short mode")
	}

	configs := []ClusterConfig{
		{Node: "node1", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:2331", ManagerJoin: "", Bind: "localhost:1331"},
		{Node: "node2", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:2332", ManagerJoin: "localhost:2331", Bind: "localhost:1332"},
		{Node: "node3", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: "localhost:2333", ManagerJoin: "localhost:2331", Bind: "localhost:1333"},
	}

	nodes := make([]*BullyCluster, 0, len(configs))
	for _, config := range configs {
		node, err := NewCluster(config)
		if err != nil {
			t.Fatalf("failed to create %s: %s", config.Node, err)
		}
		if err := node.Start(); err != nil {
			t.Fatalf("failed to start %s: %s", config.Node, err)
		}
		defer node.Shutdown()
		nodes = append(nodes, node)
	}

	// Every node must end up knowing the other two.
	for _, node := range nodes {
		waitForPeers(t, node, len(nodes)-1, 10*time.Second)
	}

	counts := func() []uint64 {
		result := make([]uint64, len(nodes))
		for i, node := range nodes {
			result[i] = joinServerOf(t, node).JoinRequestCount()
		}
		return result
	}

	before := counts()
	time.Sleep(2 * time.Second)
	after := counts()

	for i, node := range nodes {
		if after[i] != before[i] {
			t.Fatalf("node %s kept serving join requests after convergence: %d -> %d over 2s (join notifications are looping between peers)",
				node.nodeID, before[i], after[i])
		}
	}

	// A converged three node cluster should have needed very little join
	// traffic in total: node1 serves the two direct joins, and each of the
	// other two learns about the remaining node through a single notification.
	total := uint64(0)
	for _, c := range after {
		total += c
	}
	if total > uint64(len(nodes)*len(nodes)) {
		t.Fatalf("three node cluster served %d join requests to converge, expected at most %d", total, len(nodes)*len(nodes))
	}
}
