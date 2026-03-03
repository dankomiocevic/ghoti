package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestCluster(config ClusterConfig) *BullyCluster {
	return &BullyCluster{
		config: config,
		nodeID: config.Node,
		peers:  make(map[string]string),
		isUp:   true,
		stopCh: make(chan struct{}),
	}
}

func runServer(t *testing.T, config *ClusterConfig, cluster *BullyCluster) MembershipManager {
	// configure the join Server
	s, err := GetManager(config, cluster)
	if err != nil {
		t.Fatalf("failed to get manager: %s", err)
	}

	// Start the server
	err = s.Start()
	if err != nil {
		t.Fatalf("failed to start server: %s", err)
	}

	// wait for the Server to start
	time.Sleep(time.Duration(100) * time.Millisecond)

	return s
}

// More of an integration test.
func TestJoin(t *testing.T) {
	mgrAddr := "localhost:5345"
	config := &ClusterConfig{Node: "node1", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: mgrAddr, ManagerJoin: "", Bind: "localhost:5555"}

	cluster := newTestCluster(*config)

	s := runServer(t, config, cluster)
	defer s.Close()

	client := &http.Client{}

	b, err := json.Marshal(map[string]string{"addr": "localhost:5555", "id": "node2"})
	if err != nil {
		t.Fatalf("failed to encode key and value for POST: %s", err)
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/join", mgrAddr), bytes.NewReader(b))
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

	// Verify the peer was added
	peers := cluster.GetPeers()
	if _, ok := peers["node2"]; !ok {
		t.Fatalf("node2 was not added to peers")
	}
}

func TestJoinNoAuth(t *testing.T) {
	mgrAddr := "localhost:2345"
	config := &ClusterConfig{User: "my_user", Pass: "my_pass", ManagerAddr: mgrAddr}

	cluster := newTestCluster(*config)

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	b, err := json.Marshal(map[string]string{"addr": "localhost:5555", "id": "node2"})
	if err != nil {
		t.Fatalf("failed to encode key and value for POST: %s", err)
	}
	req := httptest.NewRequest("POST", fmt.Sprintf("http://%s/join", mgrAddr), bytes.NewReader(b))
	req.Header.Add("Content-Type", "application/json")

	w := httptest.NewRecorder()

	js.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.Status != "400 Bad Request" {
		t.Fatalf("POST request returned wrong status %s", resp.Status)
	}
}

func TestJoinWrongAuth(t *testing.T) {
	mgrAddr := "localhost:2345"
	config := &ClusterConfig{User: "my_user", Pass: "my_pass", ManagerAddr: mgrAddr}

	cluster := newTestCluster(*config)

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	b, err := json.Marshal(map[string]string{"addr": "localhost:5555", "id": "node2"})
	if err != nil {
		t.Fatalf("failed to encode key and value for POST: %s", err)
	}
	req := httptest.NewRequest("POST", fmt.Sprintf("http://%s/join", mgrAddr), bytes.NewReader(b))
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

func TestWrongJoinData(t *testing.T) {
	mgrAddr := "localhost:2345"
	config := &ClusterConfig{User: "my_user", Pass: "my_pass", ManagerAddr: mgrAddr}

	cluster := newTestCluster(*config)

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	var requestData [6][]byte
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
	b, _ = json.Marshal(map[string]string{"pepe": "localhost:5555", "addr": "node2"})
	requestData[5] = b

	for _, element := range requestData {
		req := httptest.NewRequest("POST", fmt.Sprintf("http://%s/join", mgrAddr), bytes.NewReader(element))
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

func TestRemoveWrongAuth(t *testing.T) {
	mgrAddr := "localhost:2345"
	config := &ClusterConfig{User: "my_user", Pass: "my_pass", ManagerAddr: mgrAddr}

	cluster := newTestCluster(*config)
	cluster.leader = cluster.nodeID // Make it leader

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	b, err := json.Marshal(map[string]string{"id": "node2"})
	if err != nil {
		t.Fatalf("failed to encode key and value for POST: %s", err)
	}
	req := httptest.NewRequest("POST", fmt.Sprintf("http://%s/remove", mgrAddr), bytes.NewReader(b))
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

func TestWrongRemoveData(t *testing.T) {
	mgrAddr := "localhost:2345"
	config := &ClusterConfig{User: "my_user", Pass: "my_pass", ManagerAddr: mgrAddr}

	cluster := newTestCluster(*config)
	cluster.leader = cluster.nodeID // Make it leader

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	var requestData [4][]byte
	b, _ := json.Marshal(map[string]string{"pepe": "node2"})
	requestData[0] = b
	b, _ = json.Marshal(map[string]string{"addr": "node2"})
	requestData[1] = b
	b = []byte("something")
	requestData[2] = b
	b = []byte("{}")
	requestData[3] = b

	for _, element := range requestData {
		req := httptest.NewRequest("POST", fmt.Sprintf("http://%s/remove", mgrAddr), bytes.NewReader(element))
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

func TestHeartbeat(t *testing.T) {
	mgrAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", ManagerJoin: "", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: mgrAddr}

	cluster := newTestCluster(*config)

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	req := httptest.NewRequest("GET", fmt.Sprintf("http://%s/heartbeat", mgrAddr), nil)
	w := httptest.NewRecorder()

	js.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Heartbeat should return 200 OK, got %d", resp.StatusCode)
	}
}

func TestHeartbeatWhenDown(t *testing.T) {
	mgrAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", ManagerJoin: "", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: mgrAddr}

	cluster := newTestCluster(*config)
	cluster.isUp = false

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	req := httptest.NewRequest("GET", fmt.Sprintf("http://%s/heartbeat", mgrAddr), nil)
	w := httptest.NewRecorder()

	js.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("Heartbeat should return 503, got %d", resp.StatusCode)
	}
}

func TestElectionEndpoint(t *testing.T) {
	mgrAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", ManagerJoin: "", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: mgrAddr}

	cluster := newTestCluster(*config)

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	b, _ := json.Marshal(map[string]string{"id": "node0"})
	req := httptest.NewRequest("POST", fmt.Sprintf("http://%s/election", mgrAddr), bytes.NewReader(b))
	req.SetBasicAuth("my_user", "my_pass")

	w := httptest.NewRecorder()

	js.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Election endpoint should return 200, got %d", resp.StatusCode)
	}
}

func TestElectionEndpointWrongAuth(t *testing.T) {
	mgrAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", ManagerJoin: "", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: mgrAddr}

	cluster := newTestCluster(*config)

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	b, _ := json.Marshal(map[string]string{"id": "node0"})
	req := httptest.NewRequest("POST", fmt.Sprintf("http://%s/election", mgrAddr), bytes.NewReader(b))
	req.SetBasicAuth("wrong_user", "my_pass")

	w := httptest.NewRecorder()

	js.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Election endpoint should return 400 for wrong auth, got %d", resp.StatusCode)
	}
}

func TestCoordinatorEndpoint(t *testing.T) {
	mgrAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", ManagerJoin: "", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: mgrAddr}

	cluster := newTestCluster(*config)

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	b, _ := json.Marshal(map[string]string{"id": "node2"})
	req := httptest.NewRequest("POST", fmt.Sprintf("http://%s/coordinator", mgrAddr), bytes.NewReader(b))
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth("my_user", "my_pass")

	w := httptest.NewRecorder()

	js.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Coordinator endpoint should return 200, got %d", resp.StatusCode)
	}

	if cluster.GetLeader() != "node2" {
		t.Fatalf("Leader should be set to node2, got %s", cluster.GetLeader())
	}
}

func TestCoordinatorEndpointWrongAuth(t *testing.T) {
	mgrAddr := "localhost:2345"
	config := &ClusterConfig{Node: "node1", ManagerJoin: "", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: mgrAddr}

	cluster := newTestCluster(*config)

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	b, _ := json.Marshal(map[string]string{"id": "node2"})
	req := httptest.NewRequest("POST", fmt.Sprintf("http://%s/coordinator", mgrAddr), bytes.NewReader(b))
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth("wrong_user", "my_pass")

	w := httptest.NewRecorder()

	js.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Coordinator endpoint should return 400 for wrong auth, got %d", resp.StatusCode)
	}
}
