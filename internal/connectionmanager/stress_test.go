package connectionmanager

import (
	"net"
	"sync"
	"testing"
	"time"
)

// slowConn is a net.Conn whose writes take longer than the broadcast deadline,
// so the event processor answers the callback after the broadcaster gave up.
type slowConn struct {
	*TestConn
	delay time.Duration
}

func (c *slowConn) Write(b []byte) (int, error) {
	time.Sleep(c.delay)
	return len(b), nil
}

// chunkedConn writes at most chunk bytes per call and reports the partial
// count, the way net.Conn is allowed to.
type chunkedConn struct {
	*TestConn
	chunk int

	mu      sync.Mutex
	written []byte
}

func (c *chunkedConn) Write(b []byte) (int, error) {
	n := c.chunk
	if n > len(b) {
		n = len(b)
	}

	c.mu.Lock()
	c.written = append(c.written, b[:n]...)
	c.mu.Unlock()
	return n, nil
}

func (c *chunkedConn) Written() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.written)
}

// stalledConn accepts nothing and reports no error, the short write that must
// not be mistaken for a delivery.
type stalledConn struct {
	*TestConn
}

func (c *stalledConn) Write(b []byte) (int, error) {
	return 0, nil
}

// TestConcurrentConnectDisconnectBroadcast hammers the manager with clients
// connecting and disconnecting while broadcasts are in flight. Run under -race
// it covers the connection map iteration and the event queue lifecycle.
//
// The number of connections is bounded on purpose: an unbounded churn loop
// exhausts the ephemeral ports of the machine running the suite.
func TestConcurrentConnectDisconnectBroadcast(t *testing.T) {
	const clients = 8
	const rounds = 15

	manager := NewTCPManager()
	if err := manager.StartListening("127.0.0.1:0"); err != nil {
		t.Fatalf("Error starting the listener: %s", err)
	}

	addr := manager.GetAddr()
	go manager.ServeConnections(func(_ int, _ []byte, conn *Connection) error { //nolint:errcheck
		conn.SendEvent("v000ok\n") //nolint:errcheck
		return nil
	})

	var churn sync.WaitGroup
	for i := 0; i < clients; i++ {
		churn.Add(1)
		go func() {
			defer churn.Done()
			for round := 0; round < rounds; round++ {
				client, err := net.Dial("tcp", addr)
				if err != nil {
					t.Errorf("Error connecting to the server: %s", err)
					return
				}

				client.Write([]byte("r000\n")) //nolint:errcheck
				buf := make([]byte, 64)
				client.SetReadDeadline(time.Now().Add(200 * time.Millisecond)) //nolint:errcheck
				client.Read(buf)                                               //nolint:errcheck
				client.Close()
			}
		}()
	}

	done := make(chan struct{})
	var broadcasters sync.WaitGroup
	for i := 0; i < 4; i++ {
		broadcasters.Add(1)
		go func() {
			defer broadcasters.Done()
			for {
				select {
				case <-done:
					return
				default:
				}

				if _, err := manager.Broadcast("a000hello\n"); err != nil {
					t.Errorf("Error broadcasting message: %s", err)
					return
				}
			}
		}()
	}

	churn.Wait()
	close(done)
	broadcasters.Wait()
	manager.Close()
}

// TestBroadcastLateCallbackDoesNotPanic covers the event processor answering
// after the broadcast deadline expired: the callback channel outlives the
// broadcast, so the late response is dropped instead of crashing the process.
func TestBroadcastLateCallbackDoesNotPanic(t *testing.T) {
	conn := NewConnection("slow", &slowConn{TestConn: &TestConn{}, delay: 400 * time.Millisecond}, 1024, 200*time.Millisecond)
	go conn.EventProcessor()
	defer conn.Close()

	manager := TCPManager{
		connections: map[string]Connection{conn.ID: conn},
		quit:        make(chan interface{}),
	}

	output, err := manager.Broadcast("Hello World")
	if err != nil {
		t.Fatalf("Error broadcasting message: %s", err)
	}

	// The connection never confirmed in time, so it is not a delivery.
	if output != "0/1/1" {
		t.Fatalf("Expected '0/1/1', got %s", output)
	}

	// Give the processor time to answer into the channel the broadcast left
	// behind. A closed channel would panic here.
	time.Sleep(500 * time.Millisecond)
}

// TestHTTPBroadcastLateCallbackDoesNotPanic is the HTTP counterpart: the SSE
// subscriber confirms after the deadline, so the broadcast reports it as a
// failed delivery and the late response lands on a channel that is still open.
func TestHTTPBroadcastLateCallbackDoesNotPanic(t *testing.T) {
	conn := NewConnection("slow", &slowConn{TestConn: &TestConn{}, delay: 400 * time.Millisecond}, 1024, 200*time.Millisecond)
	go conn.EventProcessor()
	defer conn.Close()

	manager := NewHTTPManager()
	manager.connections[conn.ID] = conn

	output, err := manager.Broadcast("Hello World")
	if err != nil {
		t.Fatalf("Error broadcasting message: %s", err)
	}

	if output != "0/1/1" {
		t.Fatalf("Expected '0/1/1', got %s", output)
	}

	// Give the processor time to answer into the channel the broadcast left
	// behind. A closed channel would panic here.
	time.Sleep(500 * time.Millisecond)
}

// TestBroadcastToClosedConnection covers a connection that is closed while a
// broadcaster still holds a copy of it: the delivery fails, it does not panic
// sending on a closed queue.
func TestBroadcastToClosedConnection(t *testing.T) {
	conn := NewConnection("closed", &TestConn{}, 1024, 200*time.Millisecond)
	go conn.EventProcessor()
	conn.Close()

	manager := TCPManager{
		connections: map[string]Connection{conn.ID: conn},
		quit:        make(chan interface{}),
	}

	output, err := manager.Broadcast("Hello World")
	if err != nil {
		t.Fatalf("Error broadcasting message: %s", err)
	}

	if output != "0/1/1" {
		t.Fatalf("Expected '0/1/1', got %s", output)
	}
}

// TestPartialWriteIsCompleted verifies that a connection that only accepts a
// few bytes per call still receives the whole event.
func TestPartialWriteIsCompleted(t *testing.T) {
	network := &chunkedConn{TestConn: &TestConn{}, chunk: 3}
	conn := NewConnection("chunked", network, 1024, 200*time.Millisecond)
	go conn.EventProcessor()
	defer conn.Close()

	callback := make(chan string, 1)
	conn.Enqueue(Event{
		id:       "event",
		data:     []byte("Hello World"),
		callback: callback,
		timeout:  time.Now().Add(time.Second),
	})

	select {
	case response := <-callback:
		if response != "event OK" {
			t.Fatalf("Expected 'event OK', got %s", response)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for the event response")
	}

	if network.Written() != "Hello World" {
		t.Fatalf("Expected the full message to be written, got %q", network.Written())
	}
}

// TestShortWriteIsReportedAsError verifies that a write that makes no progress
// is not acknowledged as a delivery.
func TestShortWriteIsReportedAsError(t *testing.T) {
	conn := NewConnection("stalled", &stalledConn{TestConn: &TestConn{}}, 1024, 200*time.Millisecond)
	go conn.EventProcessor()
	defer conn.Close()

	callback := make(chan string, 1)
	conn.Enqueue(Event{
		id:       "event",
		data:     []byte("Hello World"),
		callback: callback,
		timeout:  time.Now().Add(time.Second),
	})

	select {
	case response := <-callback:
		if response != "event ERROR" {
			t.Fatalf("Expected 'event ERROR', got %s", response)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for the event response")
	}
}

// TestClosingConnectionFailsQueuedEvents verifies that the events still queued
// when a connection goes away are answered instead of being left hanging until
// their deadline.
func TestClosingConnectionFailsQueuedEvents(t *testing.T) {
	conn := NewConnection("draining", &slowConn{TestConn: &TestConn{}, delay: 100 * time.Millisecond}, 1024, 200*time.Millisecond)
	go conn.EventProcessor()

	callback := make(chan string, 4)
	for i := 0; i < 4; i++ {
		conn.Enqueue(Event{
			id:       "event",
			data:     []byte("Hello World"),
			callback: callback,
			timeout:  time.Now().Add(5 * time.Second),
		})
	}

	conn.Close()

	deadline := time.After(2 * time.Second)
	for i := 0; i < 4; i++ {
		select {
		case <-callback:
		case <-deadline:
			t.Fatalf("Only %d of the 4 queued events were answered", i)
		}
	}
}
