package connectionmanager

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/telemetry"
)

// chanConn implements net.Conn backed by a channel, used for HTTP request/response handling.
// Writes from the EventProcessor are captured in writeCh so the HTTP handler can read them.
type chanConn struct {
	writeCh   chan []byte
	closeCh   chan struct{}
	closeOnce sync.Once
}

func newChanConn() *chanConn {
	return &chanConn{
		writeCh: make(chan []byte, 16),
		closeCh: make(chan struct{}),
	}
}

func (c *chanConn) Write(b []byte) (int, error) {
	select {
	case <-c.closeCh:
		return 0, io.EOF
	default:
	}
	buf := make([]byte, len(b))
	copy(buf, b)
	select {
	case c.writeCh <- buf:
		return len(b), nil
	case <-c.closeCh:
		return 0, io.EOF
	case <-time.After(500 * time.Millisecond):
		return 0, fmt.Errorf("write timeout")
	}
}

func (c *chanConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *chanConn) Close() error                     { c.closeOnce.Do(func() { close(c.closeCh) }); return nil }
func (c *chanConn) RemoteAddr() net.Addr             { return nil }
func (c *chanConn) LocalAddr() net.Addr              { return nil }
func (c *chanConn) SetDeadline(time.Time) error      { return nil }
func (c *chanConn) SetReadDeadline(time.Time) error  { return nil }
func (c *chanConn) SetWriteDeadline(time.Time) error { return nil }

// sseConn implements net.Conn that writes SSE-formatted events to an http.ResponseWriter.
// Each line of data written to this conn is emitted as an SSE event.
type sseConn struct {
	writer    http.ResponseWriter
	flusher   http.Flusher
	closeCh   chan struct{}
	closeOnce sync.Once
}

func newSSEConn(w http.ResponseWriter, flusher http.Flusher) *sseConn {
	return &sseConn{
		writer:  w,
		flusher: flusher,
		closeCh: make(chan struct{}),
	}
}

func (c *sseConn) Write(b []byte) (int, error) {
	select {
	case <-c.closeCh:
		return 0, io.EOF
	default:
	}
	// Each non-empty line is emitted as an SSE event: "data: <line>\n\n"
	raw := strings.TrimRight(string(b), "\n")
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		fmt.Fprintf(c.writer, "data: %s\n\n", line)
	}
	c.flusher.Flush()
	return len(b), nil
}

func (c *sseConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *sseConn) Close() error                     { c.closeOnce.Do(func() { close(c.closeCh) }); return nil }
func (c *sseConn) RemoteAddr() net.Addr             { return nil }
func (c *sseConn) LocalAddr() net.Addr              { return nil }
func (c *sseConn) SetDeadline(time.Time) error      { return nil }
func (c *sseConn) SetReadDeadline(time.Time) error  { return nil }
func (c *sseConn) SetWriteDeadline(time.Time) error { return nil }

// HTTPManager implements ConnectionManager using HTTP for commands and SSE for broadcasts.
//
// HTTP endpoints:
//   - GET  /{slot} – read slot value (e.g. GET /000); if the slot is a broadcast slot
//     the connection is upgraded to an SSE stream that receives future events.
//   - POST /{slot} – write slot; request body is the value (e.g. POST /000 with body "hello")
//
// Authentication uses HTTP Basic Auth, matched against the users map provided via SetUsers.
// Anonymous access is allowed when no credentials are sent (slots without user restrictions).
type HTTPManager struct {
	lock          sync.RWMutex
	connections   map[string]Connection
	httpServer    *http.Server
	wg            sync.WaitGroup
	quit          chan interface{}
	callback      CallbackFn
	users         map[string]auth.User
	streamChecker func(int) bool
}

func NewHTTPManager() *HTTPManager {
	return &HTTPManager{
		quit:        make(chan interface{}),
		connections: make(map[string]Connection),
		users:       make(map[string]auth.User),
	}
}

// SetUsers provides the users map used for HTTP Basic Auth verification.
// Must be called before ServeConnections if any slots require authentication.
func (h *HTTPManager) SetUsers(users map[string]auth.User) {
	h.users = users
}

// SetStreamChecker provides a function that reports whether a slot index is a
// broadcast (streaming) slot. When set, GET requests on streaming slots open an
// SSE connection instead of returning an immediate value.
func (h *HTTPManager) SetStreamChecker(fn func(int) bool) {
	h.streamChecker = fn
}

func (h *HTTPManager) GetAddr() string {
	if h.httpServer != nil {
		return h.httpServer.Addr
	}
	return ""
}

func (h *HTTPManager) StartListening(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.handleSlot)
	h.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return nil
}

func (h *HTTPManager) ServeConnections(callback CallbackFn) error {
	h.callback = callback
	err := h.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (h *HTTPManager) Close() {
	close(h.quit)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if h.httpServer != nil {
		h.httpServer.Shutdown(ctx) //nolint:errcheck
	}
	h.wg.Wait()
}

func (h *HTTPManager) Delete(id string) {
	h.lock.Lock()
	defer h.lock.Unlock()
	if _, ok := h.connections[id]; ok {
		delete(h.connections, id)
		telemetry.DecrConnectedClients()
	}
}

// Broadcast sends data to all registered SSE subscriber connections.
func (h *HTTPManager) Broadcast(data string) (string, error) {
	callback := make(chan string, 100)
	defer close(callback)
	dataBytes := []byte(data)

	eventID := uuid.NewString()
	event := Event{
		id:       eventID,
		data:     dataBytes,
		callback: callback,
		timeout:  time.Now().Add(200 * time.Millisecond),
	}

	h.lock.RLock()
	connections := make([]Connection, 0, len(h.connections))
	for _, conn := range h.connections {
		connections = append(connections, conn)
	}
	h.lock.RUnlock()

	sent := 0
	received := 0
	errors := 0

	for _, conn := range connections {
		select {
		case conn.Events <- event:
			sent++
		default:
			sent++
			errors++
		}
	}

	timeout := time.Now().Add(200 * time.Millisecond)
outerLoop:
	for received+errors < sent {
		select {
		case response := <-callback:
			if response == eventID+" OK" {
				received++
			} else {
				errors++
			}
		case <-time.After(time.Until(timeout)):
			break outerLoop
		}
	}

	return fmt.Sprintf("%d/%d/%d", received, sent, errors), nil
}

// createConnection builds a Connection wrapping the provided net.Conn.
func (h *HTTPManager) createConnection(nc net.Conn) Connection {
	return Connection{
		ID:          uuid.New().String(),
		Quit:        make(chan interface{}),
		Events:      make(chan Event, 128),
		NetworkConn: nc,
		LoggedUser:  auth.User{},
		Callback:    make(chan string),
		Buffer:      make([]byte, 41),
		Timeout:     200 * time.Millisecond,
	}
}

// addSSEConnection registers an SSE connection in the broadcast map.
func (h *HTTPManager) addSSEConnection(conn Connection) {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.connections[conn.ID] = conn
	telemetry.IncrConnectedClients()
}

// authenticate validates HTTP Basic Auth credentials against the users map.
// Returns (user, true) on success or when no credentials are provided (anonymous).
// Returns (_, false) when credentials are present but invalid.
func (h *HTTPManager) authenticate(r *http.Request) (auth.User, bool) {
	username, password, hasAuth := r.BasicAuth()
	if !hasAuth {
		return auth.User{}, true
	}
	user, ok := h.users[username]
	if !ok || user.Password != password {
		return auth.User{}, false
	}
	return user, true
}

// handleSlot handles GET /{slot} and POST /{slot}.
//
// For GET on a broadcast slot (as determined by the streamChecker), the connection
// is upgraded to an SSE stream and kept open until the client disconnects; broadcast
// events are delivered as SSE data lines.
//
// For GET on any other slot, the current value is returned immediately.
// For POST, the request body (up to 36 bytes) is written to the slot.
func (h *HTTPManager) handleSlot(w http.ResponseWriter, r *http.Request) {
	// Parse the 3-digit slot number from the URL path.
	path := strings.TrimPrefix(r.URL.Path, "/")
	if len(path) != 3 {
		http.Error(w, "slot must be a 3-digit number (e.g. GET /000)", http.StatusBadRequest)
		return
	}

	user, ok := h.authenticate(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="ghoti"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// For GET requests on a streaming (broadcast) slot, open an SSE stream.
	if r.Method == http.MethodGet {
		slotNum, err := strconv.Atoi(path)
		if err == nil && h.streamChecker != nil && h.streamChecker(slotNum) {
			h.openBroadcastStream(w, r, user)
			return
		}
	}

	// Regular request/response for GET (non-broadcast) and POST.
	if h.callback == nil {
		http.Error(w, "server not ready", http.StatusServiceUnavailable)
		return
	}

	fconn := newChanConn()
	conn := h.createConnection(fconn)
	conn.LoggedUser = user
	if user.Name != "" {
		conn.Username = user.Name
		conn.IsLogged = true
	}

	defer conn.Close()
	go conn.EventProcessor()

	var msgStr string
	switch r.Method {
	case http.MethodGet:
		msgStr = "r" + path
	case http.MethodPost:
		body, err := io.ReadAll(io.LimitReader(r.Body, 37))
		if err != nil {
			http.Error(w, "error reading request body", http.StatusBadRequest)
			return
		}
		value := strings.TrimRight(string(body), "\r\n")
		if len(value) > 36 {
			http.Error(w, "value too long (max 36 characters)", http.StatusBadRequest)
			return
		}
		msgStr = "w" + path + value
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	msgBytes := []byte(msgStr + "\n")
	if err := h.callback(len(msgStr), msgBytes, &conn); err != nil {
		slog.Debug("HTTP callback returned error",
			slog.String("path", r.URL.Path),
			slog.Any("error", err),
		)
	}

	select {
	case data := <-fconn.writeCh:
		h.writeHTTPResponse(w, string(data))
	case <-time.After(500 * time.Millisecond):
		http.Error(w, "timeout waiting for server response", http.StatusGatewayTimeout)
	}
}

// writeHTTPResponse translates a ghoti protocol response line into an HTTP response.
//
//	v000value  → 200 OK, body: "value"
//	e000006    → 403 Forbidden  (WRITE_PERMISSION / READ_PERMISSION)
//	e000005    → 404 Not Found  (MISSING_SLOT)
//	e000000    → 503            (NOT_LEADER)
//	e000...    → 400 Bad Request
func (h *HTTPManager) writeHTTPResponse(w http.ResponseWriter, response string) {
	response = strings.TrimRight(response, "\n")
	if len(response) == 0 {
		http.Error(w, "empty response from server", http.StatusInternalServerError)
		return
	}

	switch response[0] {
	case 'v':
		value := ""
		if len(response) >= 4 {
			value = response[4:]
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, value)
	case 'e':
		errCode := ""
		if len(response) >= 7 {
			errCode = response[4:7]
		}
		switch errCode {
		case "006", "008": // WRITE_PERMISSION, READ_PERMISSION
			http.Error(w, "forbidden", http.StatusForbidden)
		case "005": // MISSING_SLOT
			http.Error(w, "slot not configured", http.StatusNotFound)
		case "000": // NOT_LEADER
			http.Error(w, "not the cluster leader", http.StatusServiceUnavailable)
		default:
			http.Error(w, "error: "+errCode, http.StatusBadRequest)
		}
	default:
		slog.Warn("Unexpected response from server", slog.String("response", response))
		http.Error(w, "unexpected server response", http.StatusInternalServerError)
	}
}

// openBroadcastStream upgrades a GET request on a broadcast slot to a persistent SSE stream.
// The caller receives all future broadcast events as SSE data lines until it disconnects.
// No immediate value is returned; the connection stays open waiting for writes to the slot.
func (h *HTTPManager) openBroadcastStream(w http.ResponseWriter, r *http.Request, user auth.User) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	sconn := newSSEConn(w, flusher)
	conn := h.createConnection(sconn)
	conn.LoggedUser = user
	if user.Name != "" {
		conn.Username = user.Name
		conn.IsLogged = true
	}

	h.addSSEConnection(conn)

	// Flush the response headers before starting the EventProcessor goroutine
	// so that the initial Flush and the goroutine's Flush calls never race on
	// the same ResponseWriter.
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	slog.Debug("SSE subscriber connected",
		slog.String("id", conn.ID),
		slog.String("remote_addr", r.RemoteAddr),
	)

	processorDone := make(chan struct{})
	h.wg.Add(1)
	go func() {
		defer close(processorDone)
		conn.EventProcessor()
	}()

	<-r.Context().Done()

	slog.Debug("SSE subscriber disconnected",
		slog.String("id", conn.ID),
		slog.String("remote_addr", r.RemoteAddr),
	)

	h.Delete(conn.ID)
	conn.Close()
	// Wait for EventProcessor to finish all writes before returning, so the
	// HTTP framework's finishRequest does not race with a concurrent Flush.
	<-processorDone
	h.wg.Done()
}
