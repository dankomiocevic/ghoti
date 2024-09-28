package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
)

type MockedCluster struct {
	mock.Mock
}

func (m *MockedCluster) Start() error {
	return nil
}

func (m *MockedCluster) Join(node string, addr string) error {
	args := m.Called(node, addr)
	return args.Error(0)
}

func runServer(t *testing.T, config *ClusterConfig, cluster Cluster) MembershipManager {
	// configure the join Server
	s, err := GetManager(config, cluster)
	if err != nil {
		t.Fatalf("failed to get manager: %s", err)
	}

	// Start the server
	s.Start()

	// wait for the Server to start
	time.Sleep(time.Duration(100) * time.Millisecond)

	return s
}

// More of an integration test
func TestJoin(t *testing.T) {
	joinAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", Join: "localhost:1234", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: joinAddr}

	cluster := new(MockedCluster)
	cluster.On("Join", "node2", "localhost:5555").Return(nil)

	s := runServer(t, config, cluster)
	defer s.Close()

	client := &http.Client{}

	b, err := json.Marshal(map[string]string{"addr": "localhost:5555", "id": "node2"})
	if err != nil {
		t.Fatalf("failed to encode key and value for POST: %s", err)
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/join", joinAddr), bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST request creation failed: %s", err)
	}
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth("my_user", "my_pass")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST request failed: %s", err)
	}
	defer resp.Body.Close()

	if resp.Status != "200 OK" {
		t.Fatalf("POST request returned wrong status %s", resp.Status)
	}
}

func TestNoAuth(t *testing.T) {
	joinAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", Join: "localhost:1234", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: joinAddr}

	cluster := new(MockedCluster)
	cluster.On("Join", "node2", "localhost:5555").Return(nil)

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	b, err := json.Marshal(map[string]string{"addr": "localhost:5555", "id": "node2"})
	if err != nil {
		t.Fatalf("failed to encode key and value for POST: %s", err)
	}
	req := httptest.NewRequest("POST", fmt.Sprintf("http://%s/join", joinAddr), bytes.NewReader(b))
	req.Header.Add("Content-Type", "application/json")

	w := httptest.NewRecorder()

	js.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.Status != "400 Bad Request" {
		t.Fatalf("POST request returned wrong status %s", resp.Status)
	}
}

func TestWrongAuth(t *testing.T) {
	joinAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", Join: "localhost:1234", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: joinAddr}

	cluster := new(MockedCluster)
	cluster.On("Join", "node2", "localhost:5555").Return(nil)

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	b, err := json.Marshal(map[string]string{"addr": "localhost:5555", "id": "node2"})
	if err != nil {
		t.Fatalf("failed to encode key and value for POST: %s", err)
	}
	req := httptest.NewRequest("POST", fmt.Sprintf("http://%s/join", joinAddr), bytes.NewReader(b))
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth("wrong_user", "my_pass")

	w := httptest.NewRecorder()

	js.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.Status != "400 Bad Request" {
		t.Fatalf("POST request returned wrong status %s", resp.Status)
	}
}

func TestWrongData(t *testing.T) {
	joinAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", Join: "localhost:1234", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: joinAddr}

	cluster := new(MockedCluster)
	cluster.On("Join", "node2", "localhost:5555").Return(nil)

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	var requestData [5][]byte
	b, _ := json.Marshal(map[string]string{"id": "node2"})
	requestData[0] = b
	b, _ = json.Marshal(map[string]string{"addr": "node2"})
	requestData[1] = b
	b, _ = json.Marshal(map[string]string{"pepe": "localhost:5555", "id": "node2"})
	requestData[2] = b
	b = []byte("something")
	requestData[3] = b
	b = []byte("{}")
	requestData[4] = b

	for _, element := range requestData {
		req := httptest.NewRequest("POST", fmt.Sprintf("http://%s/join", joinAddr), bytes.NewReader(element))
		req.Header.Add("Content-Type", "application/json")
		req.SetBasicAuth(config.User, config.Pass)

		w := httptest.NewRecorder()

		js.ServeHTTP(w, req)

		resp := w.Result()

		if resp.Status != "400 Bad Request" {
			t.Fatalf("POST request returned wrong status %s, for request: %s", resp.Status, element)
		}
		resp.Body.Close()
	}
}

func TestFailJoin(t *testing.T) {
	joinAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", Join: "localhost:1234", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: joinAddr}

	cluster := new(MockedCluster)
	cluster.On("Join", "node2", "localhost:5555").Return(fmt.Errorf("Something wrong"))

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	b, err := json.Marshal(map[string]string{"addr": "localhost:5555", "id": "node2"})
	if err != nil {
		t.Fatalf("failed to encode key and value for POST: %s", err)
	}
	req := httptest.NewRequest("POST", fmt.Sprintf("http://%s/join", joinAddr), bytes.NewReader(b))
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth(config.User, config.Pass)

	w := httptest.NewRecorder()

	js.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.Status != "500 Internal Server Error" {
		t.Fatalf("POST request returned wrong status %s", resp.Status)
	}
}
