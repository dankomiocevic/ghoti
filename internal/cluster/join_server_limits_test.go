package cluster

import (
	"bytes"
	"encoding/json"
	"io"
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

// unsizedBody hides the concrete reader type from httptest.NewRequest, so the
// request carries no Content-Length and the size has to be found by reading.
type unsizedBody struct{ io.Reader }

func postJoin(t *testing.T, js *joinServer, path string, body io.Reader) int {
	t.Helper()
	req := httptest.NewRequest("POST", "http://"+js.addr+path, body)
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth(js.user, js.pass)
	w := httptest.NewRecorder()
	js.ServeHTTP(w, req)
	return w.Code
}

func TestJoinServerRejectsBodyWithTrailingContent(t *testing.T) {
	// The decoder stops at the end of the first JSON value, so the size cap
	// alone does not notice what follows it. A valid object followed by
	// anything other than whitespace up to the cap must be refused, whether
	// or not the client declared a Content-Length.
	config := &ClusterConfig{User: "my_user", Pass: "my_pass", ManagerAddr: "localhost:2345"}
	valid := `{"id":"node2","addr":"host:1234"}`

	cases := map[string]string{
		"second object":        valid + `{"id":"node3","addr":"host:5678"}`,
		"oversized whitespace": valid + strings.Repeat(" ", 64<<10),
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: newTestCluster(*config)}
			if code := postJoin(t, js, "/join", unsizedBody{strings.NewReader(body)}); code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", code)
			}
		})
	}
}

func TestJoinServerRejectsDeclaredOversizedBodyBeforeReading(t *testing.T) {
	// A Content-Length over the cap is enough to refuse the request; the
	// body must not be read at all.
	config := &ClusterConfig{User: "my_user", Pass: "my_pass", ManagerAddr: "localhost:2345"}
	js := &joinServer{addr: config.ManagerAddr, user: config.User, pass: config.Pass, cluster: newTestCluster(*config)}

	body := &countingReader{Reader: strings.NewReader(strings.Repeat(" ", 64<<10))}
	req := httptest.NewRequest("POST", "http://"+js.addr+"/join", unsizedBody{body})
	req.ContentLength = 64 << 10
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth(js.user, js.pass)
	w := httptest.NewRecorder()
	js.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if body.n != 0 {
		t.Fatalf("read %d bytes of a body already known to be too large", body.n)
	}
}

type countingReader struct {
	io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.Reader.Read(p)
	c.n += n
	return n, err
}

func TestRequestToJoinRejectsOversizedOrTrailingResponse(t *testing.T) {
	// The same applies to what a peer answers on join: a valid peer list
	// followed by megabytes, or by a second document, is a broken peer.
	valid := `{"peers":{"node1":"host:1"},"leader":"node1"}`
	cases := map[string]string{
		"oversized whitespace": valid + strings.Repeat(" ", maxJoinResponseBytes),
		"second object":        valid + valid,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, body) //nolint:errcheck
			}))
			defer peer.Close()

			_, _, err := requestToJoin(strings.TrimPrefix(peer.URL, "http://"), "me:1", "me", "u", "p")
			if err == nil {
				t.Fatal("expected the join response to be rejected")
			}
		})
	}
}

func TestRequestToJoinAcceptsResponseWithTrailingNewline(t *testing.T) {
	// json.Encoder ends its output with a newline; that must still be fine.
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"peers":{"node1":"host:1"},"leader":"node1"}`+"\n") //nolint:errcheck
	}))
	defer peer.Close()

	peers, leader, err := requestToJoin(strings.TrimPrefix(peer.URL, "http://"), "me:1", "me", "u", "p")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if leader != "node1" || peers["node1"] != "host:1" {
		t.Fatalf("unexpected result: peers=%v leader=%q", peers, leader)
	}
}

func TestJoinServerCloseForcesStuckConnectionsClosed(t *testing.T) {
	// When graceful shutdown runs out of time, Close must not return with
	// handlers still running against cluster state: it has to close their
	// connections instead of leaving them behind.
	config := &ClusterConfig{Node: "node1", User: "u", Pass: "p", ManagerAddr: "127.0.0.1:0"}
	cluster := newTestCluster(*config)
	js := &joinServer{
		addr:     config.ManagerAddr,
		user:     config.User,
		pass:     config.Pass,
		nodeID:   config.Node,
		cluster:  cluster,
		timeouts: httpTimeouts{shutdown: 200 * time.Millisecond},
	}
	if err := js.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Holding the cluster lock parks the heartbeat handler on its RLock, so
	// the request below stays active for as long as we like.
	cluster.mu.Lock()
	defer cluster.mu.Unlock()

	reqErr := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + js.ln.Addr().String() + "/heartbeat")
		if err == nil {
			resp.Body.Close()
		}
		reqErr <- err
	}()
	time.Sleep(100 * time.Millisecond)

	closed := make(chan struct{})
	go func() {
		js.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not return")
	}

	select {
	case err := <-reqErr:
		if err == nil {
			t.Fatal("the stuck request completed; its connection should have been closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the stuck request is still open after Close returned")
	}
}

func TestRequestToJoinReportsTruncatedResponse(t *testing.T) {
	// A peer that promises more than it sends leaves the body short; that is
	// a read error, and it must come back as one rather than as bad JSON.
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		io.WriteString(w, `{"peers":{}`) //nolint:errcheck
	}))
	defer peer.Close()

	_, _, err := requestToJoin(strings.TrimPrefix(peer.URL, "http://"), "me:1", "me", "u", "p")
	if err == nil || !strings.Contains(err.Error(), "read join response") {
		t.Fatalf("expected a read error, got %v", err)
	}
}
