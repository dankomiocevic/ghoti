package cluster

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJoinServerRejectsOversizedBodies(t *testing.T) {
	// The bodies these endpoints expect are a handful of short fields. A
	// body far bigger than that must be refused before it is decoded, even
	// when it is otherwise valid JSON the handler would accept.
	config := &ClusterConfig{User: "my_user", Pass: "my_pass", ManagerAddr: "localhost:2345"}
	cluster := newTestCluster(*config)
	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: cluster}

	pad := strings.Repeat("x", 64<<10)
	cases := map[string]map[string]string{
		"/join":        {"addr": pad, "id": "node2"},
		"/coordinator": {"id": pad},
	}

	for path, body := range cases {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "http://"+config.ManagerAddr+path, bytes.NewReader(b))
		req.Header.Add("Content-Type", "application/json")
		req.SetBasicAuth(config.User, config.Pass)
		w := httptest.NewRecorder()

		js.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s accepted a %d byte body with status %d", path, len(b), w.Code)
		}
	}
}

func TestJoinServerDropsClientThatNeverFinishesHeaders(t *testing.T) {
	// The join server is reachable by anything that can reach the node, so a
	// client that opens a connection and never finishes its request must be
	// cut off instead of holding a goroutine for as long as it likes.
	config := &ClusterConfig{Node: "node1", User: "my_user", Pass: "my_pass", ManagerAddr: "127.0.0.1:0"}
	cluster := newTestCluster(*config)
	js := &joinServer{
		addr:     config.ManagerAddr,
		user:     config.User,
		pass:     config.Pass,
		nodeID:   config.Node,
		cluster:  cluster,
		timeouts: httpTimeouts{readHeader: 200 * time.Millisecond},
	}
	if err := js.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer js.Close()

	conn, err := net.Dial("tcp", js.ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("GET /leader HTTP/1.1\r\nHost: x\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Read(make([]byte, 1))
	if err == nil {
		t.Fatal("expected the server to close the connection")
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("server kept the half-sent request open past the header timeout")
	}
}
