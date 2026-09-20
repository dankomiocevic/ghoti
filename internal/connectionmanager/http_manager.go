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
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/telemetry"
)

// httpRemoteAddr adapts the client address string net/http hands us
// (http.Request.RemoteAddr) into a net.Addr, so the synthetic connections
// below never hand back a nil net.Addr: code such as the server's logging
// calls RemoteAddr().String() unconditionally on every connection type.
type httpRemoteAddr string

func (a httpRemoteAddr) Network() string { return "tcp" }
func (a httpRemoteAddr) String() string  { return string(a) }

// chanConn implements net.Conn backed by a channel, used for HTTP request/response handling.
// Writes from the EventProcessor are captured in writeCh so the HTTP handler can read them.
type chanConn struct {
	writeCh    chan []byte
	closeCh    chan struct{}
	closeOnce  sync.Once
	remoteAddr net.Addr
}

func newChanConn(remoteAddr string) *chanConn {
	return &chanConn{
		writeCh:    make(chan []byte, 16),
		closeCh:    make(chan struct{}),
		remoteAddr: httpRemoteAddr(remoteAddr),
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
func (c *chanConn) RemoteAddr() net.Addr             { return c.remoteAddr }
func (c *chanConn) LocalAddr() net.Addr              { return nil }
func (c *chanConn) SetDeadline(time.Time) error      { return nil }
func (c *chanConn) SetReadDeadline(time.Time) error  { return nil }
func (c *chanConn) SetWriteDeadline(time.Time) error { return nil }

// sseConn implements net.Conn that writes SSE-formatted events to an http.ResponseWriter.
// Each line of data written to this conn is emitted as an SSE event.
//
// Writes honour the deadline set with SetWriteDeadline, and a write that
// fails closes the conn: a subscriber that stopped reading is dropped rather
// than left blocking its EventProcessor for as long as the socket lives.
type sseConn struct {
	// mu serialises the processor's event writes with the heartbeat, which
	// runs on the handler goroutine; the ResponseWriter is not safe for
	// concurrent use.
	mu         sync.Mutex
	writer     http.ResponseWriter
	control    *http.ResponseController
	closeCh    chan struct{}
	closeOnce  sync.Once
	remoteAddr net.Addr
	// lastWrite is when the last event or heartbeat reached the client, as
	// UnixNano, read by the heartbeat loop to tell a silent stream from a
	// busy one.
	lastWrite atomic.Int64
}

func newSSEConn(w http.ResponseWriter, remoteAddr string) *sseConn {
	c := &sseConn{
		writer:     w,
		control:    http.NewResponseController(w),
		closeCh:    make(chan struct{}),
		remoteAddr: httpRemoteAddr(remoteAddr),
	}
	c.lastWrite.Store(time.Now().UnixNano())
	return c
}

func (c *sseConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

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
	if err := c.flush(); err != nil {
		return 0, err
	}
	return len(b), nil
}

// heartbeat sends an SSE comment, which clients ignore, so that a peer that
// went away without closing the socket is found out by the failed write
// instead of lingering until the next broadcast. It is bounded by the same
// deadline as an event write.
//
// A stream that carried an event within the last interval is left alone: the
// event already proved the peer alive and kept the proxies busy, so the
// heartbeat would only be padding.
func (c *sseConn) heartbeat(interval, deadline time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.closeCh:
		return io.EOF
	default:
	}
	if time.Since(time.Unix(0, c.lastWrite.Load())) < interval {
		return nil
	}
	if err := c.control.SetWriteDeadline(time.Now().Add(deadline)); err != nil {
		return err
	}
	fmt.Fprint(c.writer, ": keepalive\n\n")
	return c.flush()
}

// flush pushes the buffered response out and closes the conn when that
// fails. The ResponseWriter buffers, so this is where a passed write deadline
// or a dead peer shows up. Callers hold mu.
func (c *sseConn) flush() error {
	if err := c.control.Flush(); err != nil {
		c.Close()
		return err
	}
	c.lastWrite.Store(time.Now().UnixNano())
	return nil
}

func (c *sseConn) Read([]byte) (int, error)        { return 0, io.EOF }
func (c *sseConn) Close() error                    { c.closeOnce.Do(func() { close(c.closeCh) }); return nil }
func (c *sseConn) RemoteAddr() net.Addr            { return c.remoteAddr }
func (c *sseConn) LocalAddr() net.Addr             { return nil }
func (c *sseConn) SetDeadline(time.Time) error     { return nil }
func (c *sseConn) SetReadDeadline(time.Time) error { return nil }

// SetWriteDeadline bounds the next writes on the underlying HTTP connection,
// the way it does on a plain TCP conn.
func (c *sseConn) SetWriteDeadline(t time.Time) error { return c.control.SetWriteDeadline(t) }

// httpTimeouts holds the limits applied to the public HTTP server. Every
// request is a single short command, so the values are tight; the SSE stream
// is the one long-lived handler and it lifts the read and write deadlines
// itself in openBroadcastStream.
type httpTimeouts struct {
	readHeader time.Duration // time allowed to send the request headers
	read       time.Duration // time allowed for the whole request, body included
	write      time.Duration // time allowed to write a whole response
	idle       time.Duration // how long a keep-alive connection may sit idle
	heartbeat  time.Duration // silence on an SSE stream before a comment is sent
}

// defaultHTTPTimeouts are the limits used by every production manager.
var defaultHTTPTimeouts = httpTimeouts{
	readHeader: 5 * time.Second,
	read:       10 * time.Second,
	write:      10 * time.Second,
	idle:       60 * time.Second,
	heartbeat:  15 * time.Second,
}

// sseWriteDeadline bounds a heartbeat write. It matches the deadline the
// EventProcessor puts on event writes, so a stalled subscriber is treated the
// same whichever write finds it first.
const sseWriteDeadline = 200 * time.Millisecond

// maxHTTPHeaderBytes caps the request headers. A slot request carries at most
// a Basic Auth header, so anything near the 1 MiB net/http default is abuse.
const maxHTTPHeaderBytes = 8 << 10

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
	lock           sync.RWMutex
	connections    map[string]Connection
	httpServer     *http.Server
	listener       net.Listener
	wg             sync.WaitGroup
	quit           chan interface{}
	callback       CallbackFn
	users          map[string]auth.User
	streamChecker  func(int) bool
	timeouts       httpTimeouts
	maxConnections int
}

func NewHTTPManager() *HTTPManager {
	return &HTTPManager{
		quit:        make(chan interface{}),
		connections: make(map[string]Connection),
		users:       make(map[string]auth.User),
		timeouts:    defaultHTTPTimeouts,
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

func (h *HTTPManager) SetMaxConnections(limit int) {
	h.maxConnections = limit
}

func (h *HTTPManager) GetAddr() string {
	if h.listener != nil {
		return h.listener.Addr().String()
	}
	return ""
}

// StartListening binds the address right away, like the TCP manager does,
// so an address that cannot be bound is reported here instead of failing
// later inside the serving goroutine.
func (h *HTTPManager) StartListening(addr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", h.handleSlot)
	h.listener = limitListener(l, h.maxConnections)
	h.httpServer = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: h.timeouts.readHeader,
		ReadTimeout:       h.timeouts.read,
		WriteTimeout:      h.timeouts.write,
		IdleTimeout:       h.timeouts.idle,
		MaxHeaderBytes:    maxHTTPHeaderBytes,
	}
	return nil
}

func (h *HTTPManager) ServeConnections(callback CallbackFn) error {
	if h.listener == nil {
		return ErrNotListening
	}

	h.callback = callback
	err := h.httpServer.Serve(h.listener)
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
	h.lock.RLock()
	connections := make([]Connection, 0, len(h.connections))
	for _, conn := range h.connections {
		connections = append(connections, conn)
	}
	h.lock.RUnlock()

	// The callback channel has room for every response and is never closed:
	// the event processors may answer late, after this deadline is over, and
	// they must never block or send on a closed channel.
	callback := make(chan string, len(connections))
	dataBytes := []byte(data)

	eventID := uuid.NewString()
	event := Event{
		id:       eventID,
		data:     dataBytes,
		callback: callback,
		timeout:  time.Now().Add(200 * time.Millisecond),
	}

	sent := 0
	received := 0
	errors := 0

	for _, conn := range connections {
		sent++
		if !conn.Enqueue(event) {
			errors++
		}
	}

	// A single timer for the whole wait: time.After inside the loop would
	// allocate one throwaway timer per response, and keep every one of them
	// alive until the deadline passes.
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	okResponse := eventID + " OK"

outerLoop:
	for received+errors < sent {
		select {
		case response := <-callback:
			if response == okResponse {
				received++
			} else {
				errors++
			}
		case <-timer.C:
			// Deliveries we never got confirmation for are failures, not
			// silent successes.
			errors = sent - received
			break outerLoop
		}
	}

	return fmt.Sprintf("%d/%d/%d", received, sent, errors), nil
}

// Multicast sends the data only to the connections listed in targets and
// waits up to timeout for the confirmations.
func (h *HTTPManager) Multicast(data string, targets []net.Conn, timeout time.Duration) (MulticastResult, error) {
	h.lock.RLock()
	connections := make([]Connection, 0, len(h.connections))
	for _, conn := range h.connections {
		connections = append(connections, conn)
	}
	h.lock.RUnlock()

	return multicastToConnections(connections, targets, data, timeout), nil
}

// createConnection builds a Connection wrapping the provided net.Conn.
func (h *HTTPManager) createConnection(nc net.Conn) Connection {
	return NewConnection(uuid.New().String(), nc, 41, 200*time.Millisecond)
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

// isSlotPath reports whether the path is exactly three ASCII digits.
// Anything else, including the signs and spaces that strconv.Atoi would
// accept, is not a valid slot number.
func isSlotPath(path string) bool {
	if len(path) != 3 {
		return false
	}
	for i := 0; i < len(path); i++ {
		if path[i] < '0' || path[i] > '9' {
			return false
		}
	}
	return true
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
	if !isSlotPath(path) {
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

	fconn := newChanConn(r.RemoteAddr)
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
	// The stream is the one response that legitimately stays open. Lift the
	// server-wide deadlines for this request only: past ReadTimeout net/http
	// cancels the request context, and past WriteTimeout every write fails,
	// so without this an idle subscriber would be dropped after ten seconds.
	rc := http.NewResponseController(w)
	if err := rc.SetReadDeadline(time.Time{}); err != nil {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	sconn := newSSEConn(w, r.RemoteAddr)
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
	if err := rc.Flush(); err != nil {
		h.Delete(conn.ID)
		conn.Close()
		return
	}

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

	// http.Server.Shutdown does not cancel request contexts: it waits for
	// every handler to return. The stream has to notice the manager closing
	// on its own, or Close would wait on this handler forever.
	// A failed write closes sconn from the processor goroutine, and that
	// has to end the handler too.
	ticker := time.NewTicker(h.timeouts.heartbeat)
	defer ticker.Stop()
wait:
	for {
		select {
		case <-r.Context().Done():
			break wait
		case <-sconn.closeCh:
			break wait
		case <-h.quit:
			break wait
		case <-ticker.C:
			if err := sconn.heartbeat(h.timeouts.heartbeat, sseWriteDeadline); err != nil {
				break wait
			}
		}
	}

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
