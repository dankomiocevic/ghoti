package cluster

import (
	"testing"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/mock"
)

type MockedCluster struct {
	mock.Mock
}

func (m *MockedCluster) Start() error {
	return nil
}

func (m *MockedCluster) Join(node, addr string) error {
	args := m.Called(node, addr)
	return args.Error(0)
}

func (m *MockedCluster) Remove(node string) error {
	args := m.Called(node)
	return args.Error(0)
}

func (m *MockedCluster) state() raft.RaftState {
	args := m.Called()
	return args.Get(0).(raft.RaftState)
}

func (m *MockedCluster) Bootstrap() raft.Future {
	args := m.Called()
	return args.Get(0).(raft.Future)
}

func (m *MockedCluster) Shutdown() raft.Future {
	args := m.Called()
	return args.Get(0).(raft.Future)
}

func (m *MockedCluster) GetLeader() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockedCluster) IsLeader() bool {
	args := m.Called()
	return args.Bool(0)
}

func TestJoinServer(t *testing.T) {
	joinAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", ManagerJoin: "localhost:1234", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: joinAddr}

	cluster := new(MockedCluster)
	cluster.On("Join", "node2", "localhost:5555", joinAddr).Return(nil)

	s, err := GetManager(config, cluster)
	if err != nil {
		t.Fatalf("failed to get manager: %s", err)
	}

	if s == nil {
		t.Fatalf("Manager is nil")
	}
}

func TestUserTooShort(t *testing.T) {
	joinAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", ManagerJoin: "localhost:1234", User: "my", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: joinAddr}

	cluster := new(MockedCluster)
	cluster.On("Join", "node2", "localhost:5555", joinAddr).Return(nil)

	_, err := GetManager(config, cluster)
	if err == nil {
		t.Fatalf("Error was expected")
	}
}

func TestPassTooShort(t *testing.T) {
	joinAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", ManagerJoin: "localhost:1234", User: "my", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: joinAddr}

	cluster := new(MockedCluster)
	cluster.On("Join", "node2", "localhost:5555", joinAddr).Return(nil)

	_, err := GetManager(config, cluster)
	if err == nil {
		t.Fatalf("Error was expected")
	}
}

func TestWrongType(t *testing.T) {
	joinAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", ManagerJoin: "localhost:1234", User: "my_user", Pass: "my_pass", ManagerType: "wrong_type", ManagerAddr: joinAddr}

	cluster := new(MockedCluster)
	cluster.On("Join", "node2", "localhost:5555", joinAddr).Return(nil)

	_, err := GetManager(config, cluster)
	if err == nil {
		t.Fatalf("Error was expected")
	}
}

func TestNoType(t *testing.T) {
	joinAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", ManagerJoin: "localhost:1234", User: "my_user", Pass: "my_pass", ManagerType: "", ManagerAddr: joinAddr}

	cluster := new(MockedCluster)
	cluster.On("Join", "node2", "localhost:5555", joinAddr).Return(nil)

	_, err := GetManager(config, cluster)
	if err == nil {
		t.Fatalf("Error was expected")
	}
}
