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

type TestFuture struct {
	mock.Mock
}

func (f *TestFuture) Error() error {
	args := f.Called()
	return args.Error(0)
}

func runServer(t *testing.T, config *ClusterConfig, cluster Cluster) MembershipManager {
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

	cluster := new(MockedCluster)
	cluster.On("Join", "node2", "localhost:5555").Return(nil)
	future := new(TestFuture)
	future.On("Error").Return(nil)
	cluster.On("Bootstrap").Return(future)

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
}

func TestJoinNoAuth(t *testing.T) {
	mgrAddr := "localhost:2345"
	config := &ClusterConfig{User: "my_user", Pass: "my_pass", ManagerAddr: mgrAddr}

	cluster := new(MockedCluster)
	cluster.On("Join", "node2", "localhost:5555", "localhost:1234").Return(nil)

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

	cluster := new(MockedCluster)
	cluster.On("Join", "node2", "localhost:5555").Return(nil)

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

	cluster := new(MockedCluster)
	cluster.On("Join", "node2", "localhost:5555").Return(nil)

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

func TestFailJoin(t *testing.T) {
	mgrAddr := "localhost:2345"
	config := &ClusterConfig{User: "my_user", Pass: "my_pass", ManagerAddr: mgrAddr}

	cluster := new(MockedCluster)
	cluster.On("Join", "node2", "localhost:5555").Return(fmt.Errorf("Something wrong"))

	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster, join: "localhost:1234"}

	b, err := json.Marshal(map[string]string{"addr": "localhost:5555", "id": "node2"})
	if err != nil {
		t.Fatalf("failed to encode key and value for POST: %s", err)
	}
	req := httptest.NewRequest("POST", fmt.Sprintf("http://%s/join", mgrAddr), bytes.NewReader(b))
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

func TestBootstrapFail(t *testing.T) {
	mgrAddr := "localhost:3345"
	config := &ClusterConfig{Node: "node1", ManagerJoin: "", User: "my_user", Pass: "my_pass", ManagerType: "join_server", ManagerAddr: mgrAddr}

	cluster := new(MockedCluster)
	future := new(TestFuture)
	retErr := fmt.Errorf("Generic error")
	future.On("Error").Return(retErr)
	cluster.On("Bootstrap").Return(future)

	// configure the join Server
	s, err := GetManager(config, cluster)
	if err != nil {
		t.Fatalf("failed to get manager: %s", err)
	}

	// Start the server
	err = s.Start()
	if err == nil {
		t.Fatalf("Test should fail to bootstrap")
	}
}

func TestRemoveWrongAuth(t *testing.T) {
	mgrAddr := "localhost:2345"
	config := &ClusterConfig{User: "my_user", Pass: "my_pass", ManagerAddr: mgrAddr}

	cluster := new(MockedCluster)
	cluster.On("IsLeader").Return(true)

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

	cluster := new(MockedCluster)
	cluster.On("IsLeader").Return(true)
	cluster.On("Remove", "node2").Return(nil)

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
