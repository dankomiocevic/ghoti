package connectionmanager

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/errs"
)

// buildTestManager creates an HTTPManager pre-wired with a callback and users.
func buildTestManager(callback CallbackFn) *HTTPManager {
	h := NewHTTPManager()
	h.SetUsers(map[string]auth.User{
		"alice": {Name: "alice", Password: "secret"},
	})
	h.callback = callback
	return h
}

// echoCallback simulates a server that always responds with a value response.
func echoCallback(size int, data []byte, conn *Connection) error {
	// parse the command byte and slot
	msg := string(data[:size])
	slot := "000"
	if len(msg) >= 4 {
		slot = msg[1:4]
	}
	var value string
	if msg[0] == 'w' && len(msg) > 4 {
		value = msg[4:]
	}
	return conn.SendEvent(fmt.Sprintf("v%s%s\n", slot, value))
}

// errorCallback simulates a server that always returns a READ_PERMISSION error.
func errorCallback(size int, data []byte, conn *Connection) error {
	msg := string(data[:size])
	slot := "000"
	if len(msg) >= 4 {
		slot = msg[1:4]
	}
	res := errs.Error("READ_PERMISSION")
	return conn.SendEvent(res.Response(slot))
}

func TestHTTPManagerRead(t *testing.T) {
	h := buildTestManager(echoCallback)

	req := httptest.NewRequest(http.MethodGet, "/000", nil)
	rr := httptest.NewRecorder()

	h.handleSlot(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHTTPManagerWrite(t *testing.T) {
	h := buildTestManager(echoCallback)

	body := strings.NewReader("hello")
	req := httptest.NewRequest(http.MethodPost, "/000", body)
	rr := httptest.NewRecorder()

	h.handleSlot(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != "hello" {
		t.Fatalf("expected body 'hello', got %q", rr.Body.String())
	}
}

func TestHTTPManagerWriteOtherSlot(t *testing.T) {
	h := buildTestManager(echoCallback)

	body := strings.NewReader("world")
	req := httptest.NewRequest(http.MethodPost, "/042", body)
	rr := httptest.NewRecorder()

	h.handleSlot(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "world" {
		t.Fatalf("expected body 'world', got %q", rr.Body.String())
	}
}

func TestHTTPManagerInvalidSlot(t *testing.T) {
	h := buildTestManager(echoCallback)

	req := httptest.NewRequest(http.MethodGet, "/invalid", nil)
	rr := httptest.NewRecorder()

	h.handleSlot(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHTTPManagerMethodNotAllowed(t *testing.T) {
	h := buildTestManager(echoCallback)

	req := httptest.NewRequest(http.MethodDelete, "/000", nil)
	rr := httptest.NewRecorder()

	h.handleSlot(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestHTTPManagerReadPermissionError(t *testing.T) {
	h := buildTestManager(errorCallback)

	req := httptest.NewRequest(http.MethodGet, "/000", nil)
	rr := httptest.NewRecorder()

	h.handleSlot(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestHTTPManagerBasicAuthValid(t *testing.T) {
	h := buildTestManager(echoCallback)

	req := httptest.NewRequest(http.MethodGet, "/000", nil)
	req.SetBasicAuth("alice", "secret")
	rr := httptest.NewRecorder()

	h.handleSlot(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid auth, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHTTPManagerBasicAuthInvalid(t *testing.T) {
	h := buildTestManager(echoCallback)

	req := httptest.NewRequest(http.MethodGet, "/000", nil)
	req.SetBasicAuth("alice", "wrong")
	rr := httptest.NewRecorder()

	h.handleSlot(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with bad credentials, got %d", rr.Code)
	}
}

func TestHTTPManagerValueTooLong(t *testing.T) {
	h := buildTestManager(echoCallback)

	longValue := strings.Repeat("x", 37)
	body := strings.NewReader(longValue)
	req := httptest.NewRequest(http.MethodPost, "/000", body)
	rr := httptest.NewRecorder()

	h.handleSlot(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for value too long, got %d", rr.Code)
	}
}

func TestHTTPManagerSSEReceivesBroadcast(t *testing.T) {
	// Use a real HTTP test server for SSE because httptest.NewRecorder
	// does not implement http.Flusher.
	h := buildTestManager(echoCallback)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/subscribe" {
			h.handleSSE(w, r)
		}
	}))
	defer srv.Close()

	// Connect SSE subscriber. http.Get returns once the server flushes the
	// response headers, which happens after h.addSSEConnection – so by the
	// time we proceed, the connection is already registered for broadcasts.
	resp, err := http.Get(srv.URL + "/subscribe")
	if err != nil {
		t.Fatalf("SSE connect failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from SSE endpoint, got %d", resp.StatusCode)
	}

	// Broadcast a message and verify all registered connections receive it.
	stats, err := h.Broadcast("a000hello\n")
	if err != nil {
		t.Fatalf("Broadcast error: %v", err)
	}

	// stats format is "received/sent/errors"
	parts := strings.Split(stats, "/")
	if len(parts) != 3 {
		t.Fatalf("unexpected stats format: %s", stats)
	}

	// Read one SSE event from the response body.
	done := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				done <- strings.TrimPrefix(line, "data: ")
				return
			}
		}
		done <- ""
	}()

	select {
	case got := <-done:
		if got != "a000hello" {
			t.Fatalf("expected SSE event 'a000hello', got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE event")
	}
}

func TestHTTPManagerBroadcastWithNoSubscribers(t *testing.T) {
	h := buildTestManager(echoCallback)

	stats, err := h.Broadcast("a000test\n")
	if err != nil {
		t.Fatalf("Broadcast error: %v", err)
	}
	// With zero subscribers: "0/0/0"
	if stats != "0/0/0" {
		t.Fatalf("expected '0/0/0', got %q", stats)
	}
}

func TestChanConnWriteAndRead(t *testing.T) {
	c := newChanConn()
	defer c.Close()

	msg := []byte("hello")
	go func() {
		c.Write(msg) //nolint:errcheck
	}()

	select {
	case got := <-c.writeCh:
		if string(got) != "hello" {
			t.Fatalf("expected 'hello', got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out reading from chanConn")
	}
}

func TestChanConnCloseReturnsEOF(t *testing.T) {
	c := newChanConn()
	c.Close()

	n, err := c.Write([]byte("data"))
	if err != io.EOF {
		t.Fatalf("expected EOF after close, got err=%v n=%d", err, n)
	}
}

func TestSSEConnFormatsEvents(t *testing.T) {
	rr := httptest.NewRecorder()
	sc := newSSEConn(rr, rr)

	sc.Write([]byte("a000hello\n")) //nolint:errcheck

	body := rr.Body.String()
	if body != "data: a000hello\n\n" {
		t.Fatalf("unexpected SSE body: %q", body)
	}
}

func TestSSEConnFormatsMultipleEvents(t *testing.T) {
	rr := httptest.NewRecorder()
	sc := newSSEConn(rr, rr)

	// Two events batched together (as sendBatchedEvents would produce)
	sc.Write([]byte("a000hello\n\na001world\n")) //nolint:errcheck

	body := rr.Body.String()
	expected := "data: a000hello\n\ndata: a001world\n\n"
	if body != expected {
		t.Fatalf("expected %q, got %q", expected, body)
	}
}
