package cluster

import (
	"testing"
)

func TestJoinServer(t *testing.T) {
	joinAddr := "localhost:2345"
	config := ClusterConfig{Node: "node1", ManagerJoin: "localhost:1234", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: joinAddr}

	cluster := &BullyCluster{
		config: config,
		nodeID: config.Node,
		peers:  make(map[string]string),
		stopCh: make(chan struct{}),
	}

	s, err := GetManager(&config, cluster)
	if err != nil {
		t.Fatalf("failed to get manager: %s", err)
	}

	if s == nil {
		t.Fatalf("Manager is nil")
	}
}

func TestUserTooShort(t *testing.T) {
	joinAddr := "localhost:2345"
	config := ClusterConfig{Node: "node1", ManagerJoin: "localhost:1234", User: "my", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: joinAddr}

	cluster := &BullyCluster{
		config: config,
		nodeID: config.Node,
		peers:  make(map[string]string),
		stopCh: make(chan struct{}),
	}

	_, err := GetManager(&config, cluster)
	if err == nil {
		t.Fatalf("Error was expected")
	}
}

func TestPassTooShort(t *testing.T) {
	joinAddr := "localhost:2345"
	config := ClusterConfig{Node: "node1", ManagerJoin: "localhost:1234", User: "my", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: joinAddr}

	cluster := &BullyCluster{
		config: config,
		nodeID: config.Node,
		peers:  make(map[string]string),
		stopCh: make(chan struct{}),
	}

	_, err := GetManager(&config, cluster)
	if err == nil {
		t.Fatalf("Error was expected")
	}
}

func TestWrongType(t *testing.T) {
	joinAddr := "localhost:2345"
	config := ClusterConfig{Node: "node1", ManagerJoin: "localhost:1234", User: "my_user", Pass: "my_pass", ManagerType: "wrong_type", ManagerAddr: joinAddr}

	cluster := &BullyCluster{
		config: config,
		nodeID: config.Node,
		peers:  make(map[string]string),
		stopCh: make(chan struct{}),
	}

	_, err := GetManager(&config, cluster)
	if err == nil {
		t.Fatalf("Error was expected")
	}
}

func TestNoType(t *testing.T) {
	joinAddr := "localhost:2345"
	config := ClusterConfig{Node: "node1", ManagerJoin: "localhost:1234", User: "my_user", Pass: "my_pass", ManagerType: "", ManagerAddr: joinAddr}

	cluster := &BullyCluster{
		config: config,
		nodeID: config.Node,
		peers:  make(map[string]string),
		stopCh: make(chan struct{}),
	}

	_, err := GetManager(&config, cluster)
	if err == nil {
		t.Fatalf("Error was expected")
	}
}
