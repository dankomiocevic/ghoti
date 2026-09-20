package connectionmanager

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// startTestHTTPServer runs the manager through its own StartListening and
// ServeConnections path, so the http.Server the manager configures is the one
// under test. The httptest based tests skip that server and its timeouts.
func startTestHTTPServer(t *testing.T, h *HTTPManager) {
	t.Helper()
	if err := h.StartListening("127.0.0.1:0"); err != nil {
		t.Fatalf("StartListening: %v", err)
	}
	go h.ServeConnections(h.callback) //nolint:errcheck
}

// openSSE subscribes to the broadcast slot 3 and returns the open response.
func openSSE(t *testing.T, h *HTTPManager) *http.Response {
	t.Helper()
	resp, err := http.Get("http://" + h.GetAddr() + "/003")
	if err != nil {
		t.Fatalf("SSE connect: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from SSE endpoint, got %d", resp.StatusCode)
	}
	return resp
}

func TestHTTPManagerCloseReturnsWithSSESubscriber(t *testing.T) {
	// http.Server.Shutdown never cancels request contexts, so an SSE handler
	// that only waits for the client to go away keeps Close blocked forever.
	h := buildTestManager(echoCallback)
	h.SetStreamChecker(func(slot int) bool { return slot == 3 })
	startTestHTTPServer(t, h)

	resp := openSSE(t, h)
	defer resp.Body.Close()

	done := make(chan struct{})
	go func() {
		h.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not return with one SSE subscriber connected")
	}
}

func TestHTTPManagerDropsClientThatNeverFinishesHeaders(t *testing.T) {
	// A client that opens a connection and dribbles a partial request must be
	// cut off by the header timeout instead of holding a goroutine forever.
	h := buildTestManager(echoCallback)
	h.timeouts.readHeader = 200 * time.Millisecond
	startTestHTTPServer(t, h)
	defer h.Close()

	conn, err := net.Dial("tcp", h.GetAddr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("GET /000 HTTP/1.1\r\nHost: x\r\n")); err != nil {
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

// readSSEData returns the payload of the next "data:" line on the stream, or
// "" if the stream ends or nothing arrives within the timeout.
func readSSEData(t *testing.T, body io.Reader, timeout time.Duration) string {
	t.Helper()
	got := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(body)
		for scanner.Scan() {
			if line := scanner.Text(); strings.HasPrefix(line, "data: ") {
				got <- strings.TrimPrefix(line, "data: ")
				return
			}
		}
		got <- ""
	}()

	select {
	case s := <-got:
		return s
	case <-time.After(timeout):
		return ""
	}
}

func TestHTTPManagerSSEOutlivesServerTimeouts(t *testing.T) {
	// The server-wide read and write timeouts must not apply to a broadcast
	// stream: net/http cancels the request context when the read deadline
	// passes and fails every write after the write deadline, either of which
	// would silently kill an idle subscriber.
	h := buildTestManager(echoCallback)
	h.SetStreamChecker(func(slot int) bool { return slot == 3 })
	h.timeouts.read = 300 * time.Millisecond
	h.timeouts.write = 300 * time.Millisecond
	startTestHTTPServer(t, h)
	defer h.Close()

	resp := openSSE(t, h)
	defer resp.Body.Close()

	// Sit idle past both deadlines, then check the stream is still live.
	time.Sleep(700 * time.Millisecond)

	if _, err := h.Broadcast("a003late\n"); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	if got := readSSEData(t, resp.Body, 2*time.Second); got != "a003late" {
		t.Fatalf("expected event a003late after the timeouts passed, got %q", got)
	}
}

// subscriberCount returns how many SSE subscribers the manager is tracking.
func subscriberCount(h *HTTPManager) int {
	h.lock.RLock()
	defer h.lock.RUnlock()
	return len(h.connections)
}

func TestHTTPManagerDropsSSESubscriberThatStopsReading(t *testing.T) {
	// A subscriber that stops draining its socket eventually fills the kernel
	// buffers and blocks the write. That write has to fail on its deadline and
	// take the subscriber down, the same way a stalled TCP client is dropped;
	// otherwise its EventProcessor sits in Write forever.
	h := buildTestManager(echoCallback)
	h.SetStreamChecker(func(slot int) bool { return slot == 3 })
	startTestHTTPServer(t, h)
	defer h.Close()

	resp := openSSE(t, h)
	defer resp.Body.Close()
	// The client never reads resp.Body from here on.

	payload := "a003" + strings.Repeat("x", 1<<20) + "\n"
	deadline := time.Now().Add(10 * time.Second)
	for subscriberCount(h) > 0 {
		if time.Now().After(deadline) {
			t.Fatal("stalled subscriber was never dropped")
		}
		h.Broadcast(payload) //nolint:errcheck
	}
}

func TestHTTPManagerSSESendsHeartbeatComments(t *testing.T) {
	// An idle stream must carry periodic SSE comments: they are how a peer
	// that silently went away is noticed, and they keep proxies between us
	// and the client from idling the stream out.
	h := buildTestManager(echoCallback)
	h.SetStreamChecker(func(slot int) bool { return slot == 3 })
	h.timeouts.heartbeat = 100 * time.Millisecond
	startTestHTTPServer(t, h)
	defer h.Close()

	resp := openSSE(t, h)
	defer resp.Body.Close()

	got := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if line := scanner.Text(); strings.HasPrefix(line, ":") {
				got <- line
				return
			}
		}
		got <- ""
	}()

	select {
	case line := <-got:
		if line == "" {
			t.Fatal("stream ended before any heartbeat")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no heartbeat comment received on an idle stream")
	}
}
