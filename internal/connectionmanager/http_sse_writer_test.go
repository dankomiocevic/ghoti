package connectionmanager

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dankomiocevic/ghoti/internal/auth"
)

// streamWriter is a ResponseWriter that supports the deadline and flush
// controls the SSE stream relies on, and can be told to start failing them
// at a given call. It stands in for a real connection, or a middleware
// wrapping one, that goes bad mid-stream.
type streamWriter struct {
	*httptest.ResponseRecorder
	writeDeadlineCalls atomic.Int32
	flushCalls         atomic.Int32
	// failWriteDeadlineAt and failFlushAt are 1-based call numbers from
	// which the method fails; zero means it never fails.
	failWriteDeadlineAt int32
	failFlushAt         int32
}

var errWriterBroken = errors.New("connection broken")

func (w *streamWriter) SetReadDeadline(time.Time) error { return nil }

func (w *streamWriter) SetWriteDeadline(time.Time) error {
	if n := w.writeDeadlineCalls.Add(1); w.failWriteDeadlineAt > 0 && n >= w.failWriteDeadlineAt {
		return errWriterBroken
	}
	return nil
}

func (w *streamWriter) FlushError() error {
	if n := w.flushCalls.Add(1); w.failFlushAt > 0 && n >= w.failFlushAt {
		return errWriterBroken
	}
	w.Flush()
	return nil
}

// readDeadlineOnlyWriter supports read deadlines but not write deadlines.
type readDeadlineOnlyWriter struct{ *httptest.ResponseRecorder }

func (readDeadlineOnlyWriter) SetReadDeadline(time.Time) error { return nil }

// streamManager returns a manager with slot 3 streaming and a request for it.
func streamManager(heartbeat time.Duration) (*HTTPManager, *http.Request) {
	h := buildTestManager(echoCallback)
	h.SetStreamChecker(func(slot int) bool { return slot == 3 })
	h.timeouts.heartbeat = heartbeat
	return h, httptest.NewRequest(http.MethodGet, "/003", nil)
}

// runStream calls openBroadcastStream in the background and returns a channel
// that is closed when it returns.
func runStream(h *HTTPManager, w http.ResponseWriter, r *http.Request) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.openBroadcastStream(w, r, auth.User{})
	}()
	return done
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for " + what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestOpenBroadcastStreamRejectsWriterWithoutDeadlineControl(t *testing.T) {
	// The stream cannot be kept safe without lifting the server deadlines,
	// so a writer that offers no control over them is refused outright.
	h, r := streamManager(time.Second)

	cases := map[string]http.ResponseWriter{
		"no deadlines":       httptest.NewRecorder(),
		"read deadline only": readDeadlineOnlyWriter{httptest.NewRecorder()},
	}
	for name, w := range cases {
		t.Run(name, func(t *testing.T) {
			h.openBroadcastStream(w, r, auth.User{})
			rr := w.(interface{ Result() *http.Response }).Result()
			defer rr.Body.Close()
			if rr.StatusCode != http.StatusInternalServerError {
				t.Fatalf("expected 500, got %d", rr.StatusCode)
			}
			if subscriberCount(h) != 0 {
				t.Fatal("a refused stream must not be registered as a subscriber")
			}
		})
	}
}

func TestOpenBroadcastStreamDropsSubscriberWhenHeadersCannotBeFlushed(t *testing.T) {
	h, r := streamManager(time.Second)
	w := &streamWriter{ResponseRecorder: httptest.NewRecorder(), failFlushAt: 1}

	select {
	case <-runStream(h, w, r):
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not end after the header flush failed")
	}
	if subscriberCount(h) != 0 {
		t.Fatal("subscriber left registered after its headers could not be sent")
	}
}

func TestOpenBroadcastStreamEndsWhenHeartbeatCannotBeWritten(t *testing.T) {
	// The first write deadline call is the one lifting the server deadline
	// at stream start; the heartbeat's is the second.
	h, r := streamManager(20 * time.Millisecond)
	w := &streamWriter{ResponseRecorder: httptest.NewRecorder(), failWriteDeadlineAt: 2}

	select {
	case <-runStream(h, w, r):
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not end after the heartbeat failed")
	}
	if subscriberCount(h) != 0 {
		t.Fatal("subscriber left registered after its heartbeat failed")
	}
}

func TestOpenBroadcastStreamEndsWhenEventWriteFails(t *testing.T) {
	// The processor goroutine finds the broken connection while writing an
	// event, with the request context still live; the handler has to notice
	// through the conn being closed.
	h, r := streamManager(time.Hour)
	w := &streamWriter{ResponseRecorder: httptest.NewRecorder(), failFlushAt: 2}
	done := runStream(h, w, r)

	waitFor(t, "the subscriber to register", func() bool { return subscriberCount(h) == 1 })
	h.Broadcast("a003boom\n") //nolint:errcheck

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not end after an event write failed")
	}
	if subscriberCount(h) != 0 {
		t.Fatal("subscriber left registered after its event write failed")
	}
}

func TestSSEConnHeartbeatOnClosedConn(t *testing.T) {
	sc := newSSEConn(httptest.NewRecorder(), "192.0.2.1:1")
	sc.Close()
	if err := sc.heartbeat(0, time.Second); err != io.EOF {
		t.Fatalf("expected io.EOF from a closed conn, got %v", err)
	}
}

func TestSSEConnSatisfiesNetConnContract(t *testing.T) {
	// The stream conn is write-only, but it is handed around as a net.Conn,
	// so the rest of the interface must behave as an idle conn would.
	sc := newSSEConn(httptest.NewRecorder(), "192.0.2.1:1")
	if n, err := sc.Read(make([]byte, 1)); n != 0 || err != io.EOF {
		t.Fatalf("Read = %d, %v; want 0, io.EOF", n, err)
	}
	if sc.LocalAddr() != nil {
		t.Fatal("LocalAddr should be nil for a synthetic conn")
	}
	if err := sc.SetDeadline(time.Now()); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}
	if err := sc.SetReadDeadline(time.Now()); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
}

func TestWriteHTTPResponseMapsProtocolErrors(t *testing.T) {
	// The translation from a protocol response line to an HTTP status is
	// part of the HTTP transport's contract, so every branch is pinned here.
	h := buildTestManager(echoCallback)
	cases := map[string]struct {
		response string
		status   int
	}{
		"value":              {"v000hello\n", http.StatusOK},
		"write permission":   {"e000006\n", http.StatusForbidden},
		"read permission":    {"e000008\n", http.StatusForbidden},
		"missing slot":       {"e000005\n", http.StatusNotFound},
		"not leader":         {"e000000\n", http.StatusServiceUnavailable},
		"other error":        {"e000001\n", http.StatusBadRequest},
		"empty":              {"\n", http.StatusInternalServerError},
		"unknown first byte": {"x000\n", http.StatusInternalServerError},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.writeHTTPResponse(w, tc.response)
			if w.Code != tc.status {
				t.Fatalf("response %q: expected %d, got %d", tc.response, tc.status, w.Code)
			}
		})
	}
}

func TestChanConnSatisfiesNetConnContract(t *testing.T) {
	c := newChanConn("192.0.2.1:1")
	if n, err := c.Read(make([]byte, 1)); n != 0 || err != io.EOF {
		t.Fatalf("Read = %d, %v; want 0, io.EOF", n, err)
	}
	if c.LocalAddr() != nil {
		t.Fatal("LocalAddr should be nil for a synthetic conn")
	}
	if err := c.SetDeadline(time.Now()); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}
	if err := c.SetReadDeadline(time.Now()); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
}

func TestChanConnWriteFailsWhenNobodyReads(t *testing.T) {
	// The channel behind a request conn is bounded; a processor writing into
	// a request nobody is waiting on must give up instead of blocking.
	c := newChanConn("192.0.2.1:1")
	for i := 0; i < cap(c.writeCh); i++ {
		if _, err := c.Write([]byte("x")); err != nil {
			t.Fatalf("write %d failed early: %v", i, err)
		}
	}
	if _, err := c.Write([]byte("x")); err == nil {
		t.Fatal("expected a write timeout once the channel is full")
	}
}

func TestChanConnWriteFailsWhenClosedWhileBlocked(t *testing.T) {
	c := newChanConn("192.0.2.1:1")
	for i := 0; i < cap(c.writeCh); i++ {
		c.Write([]byte("x")) //nolint:errcheck
	}
	errCh := make(chan error, 1)
	go func() {
		_, err := c.Write([]byte("x"))
		errCh <- err
	}()
	time.Sleep(20 * time.Millisecond)
	c.Close()
	select {
	case err := <-errCh:
		if err != io.EOF {
			t.Fatalf("expected io.EOF when closed mid-write, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked write did not return after Close")
	}
}

func TestSSEConnWriteOnClosedConn(t *testing.T) {
	sc := newSSEConn(httptest.NewRecorder(), "192.0.2.1:1")
	sc.Close()
	if _, err := sc.Write([]byte("a003x\n")); err != io.EOF {
		t.Fatalf("expected io.EOF writing to a closed stream, got %v", err)
	}
}

func TestIsSlotPathRejectsNonDigits(t *testing.T) {
	for _, p := range []string{"abc", "0a0", "+01", "-01", " 01"} {
		if isSlotPath(p) {
			t.Fatalf("%q must not be accepted as a slot", p)
		}
	}
	if !isSlotPath("007") {
		t.Fatal("007 must be accepted as a slot")
	}
}

func TestHandleSlotBeforeServing(t *testing.T) {
	// Until ServeConnections installs the callback there is nothing to hand
	// a command to, and the client is told so instead of being left waiting.
	h := NewHTTPManager()
	w := httptest.NewRecorder()
	h.handleSlot(w, httptest.NewRequest(http.MethodGet, "/000", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 before serving, got %d", w.Code)
	}
}

// failingBody is a request body whose first read fails.
type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errWriterBroken }

func TestHandleSlotUnreadableBody(t *testing.T) {
	h := buildTestManager(echoCallback)
	w := httptest.NewRecorder()
	h.handleSlot(w, httptest.NewRequest(http.MethodPost, "/000", failingBody{}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unreadable body, got %d", w.Code)
	}
}

func TestHandleSlotServerNeverAnswers(t *testing.T) {
	// A callback that fails without producing a response must not hang the
	// request: the handler gives up on its own deadline with a 504.
	h := buildTestManager(func(int, []byte, *Connection) error { return errWriterBroken })
	w := httptest.NewRecorder()
	start := time.Now()
	h.handleSlot(w, httptest.NewRequest(http.MethodGet, "/000", nil))
	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504 when the server never answers, got %d", w.Code)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("handler waited far longer than its deadline")
	}
}

func TestOpenBroadcastStreamRecordsAuthenticatedUser(t *testing.T) {
	// A subscriber that authenticated is registered under its user, the same
	// as an authenticated command connection, so slot permissions apply.
	h := buildTestManager(echoCallback)
	h.SetStreamChecker(func(slot int) bool { return slot == 3 })
	startTestHTTPServer(t, h)
	defer h.Close()

	req, _ := http.NewRequest(http.MethodGet, "http://"+h.GetAddr()+"/003", nil)
	req.SetBasicAuth("alice", "secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE connect: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	h.lock.RLock()
	defer h.lock.RUnlock()
	for _, conn := range h.connections {
		if !conn.IsLogged || conn.Username != "alice" {
			t.Fatalf("subscriber registered as %q (logged=%v), want alice", conn.Username, conn.IsLogged)
		}
		return
	}
	t.Fatal("no subscriber registered")
}

func TestBroadcastCountsClosedSubscriberAsError(t *testing.T) {
	// A subscriber that is closed but not yet removed from the map refuses
	// the event at enqueue time; that is a failed delivery, not a skipped one.
	h := buildTestManager(echoCallback)
	conn := h.createConnection(newSSEConn(httptest.NewRecorder(), "192.0.2.1:1"))
	h.addSSEConnection(conn)
	defer h.Delete(conn.ID)
	conn.Close()

	stats, err := h.Broadcast("a003x\n")
	if err != nil {
		t.Fatal(err)
	}
	if stats != "0/1/1" {
		t.Fatalf("expected 0/1/1 (received/sent/errors), got %s", stats)
	}
}
