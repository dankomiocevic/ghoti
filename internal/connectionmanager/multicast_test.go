package connectionmanager

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// This test creates several connections and multicasts a message only to a
// subset of them, checking that non-targeted connections receive nothing and
// that the sender is a fair one.
func TestMulticastOnlyReachesTargets(t *testing.T) {
	var wg sync.WaitGroup

	servers := []net.Conn{}
	clients := []net.Conn{}

	for i := 0; i < 10; i++ {
		server, client := net.Pipe()
		servers = append(servers, server)
		clients = append(clients, client)
	}

	connections := make(map[string]Connection)
	for i, server := range servers {
		id := fmt.Sprintf("%d", i+1)
		c := &Connection{
			ID:          id,
			Quit:        make(chan interface{}),
			Events:      make(chan Event, 10),
			NetworkConn: server,
			Callback:    make(chan string, 10),
			Buffer:      make([]byte, 1024),
			Timeout:     200,
		}
		connections[id] = *c
		go c.EventProcessor()
	}

	manager := TCPManager{
		connections: connections,
		quit:        make(chan interface{}),
		lock:        sync.RWMutex{},
	}

	targets := []net.Conn{servers[2], servers[5]}

	go func() {
		output, err := manager.Multicast("Hello World", targets, 500*time.Millisecond)
		if err != nil {
			t.Errorf("Error multicasting message: %s", err)
		}

		if output.Received != 2 || output.Sent != 2 || output.Errors != 0 {
			t.Errorf("Expected 2/2/0, got %d/%d/%d", output.Received, output.Sent, output.Errors)
		}
		if len(output.Failed) != 0 {
			t.Errorf("Expected no failures, got %v", output.Failed)
		}

		for _, conn := range servers {
			conn.Close()
		}
	}()

	for i, conn := range clients {
		targeted := i == 2 || i == 5

		wg.Add(1)
		go func(conn net.Conn, targeted bool) {
			defer wg.Done()
			conn.SetDeadline(time.Now().Add(time.Second))
			value, _ := io.ReadAll(conn)
			if targeted {
				if string(value) != "Hello World" {
					t.Errorf("Expected 'Hello World', got %q", string(value))
				}
			} else {
				if len(value) != 0 {
					t.Errorf("Non targeted connection received data: %q", string(value))
				}
			}
		}(conn, targeted)
	}

	wg.Wait()
}

// This test checks that a target whose connection fails is reported in Failed.
func TestMulticastReportsFailedTarget(t *testing.T) {
	server, client := net.Pipe()
	client.Close()
	// Closing the client makes writes to server fail once the pipe is drained,
	// but net.Pipe needs a reader on the other side to unblock the writer, so
	// instead we close the server side directly which makes sends fail fast.
	server.Close()

	failingConn := &TestConn{}
	okServer, okClient := net.Pipe()
	defer okClient.Close()

	connections := map[string]Connection{
		"1": {
			ID:          "1",
			Quit:        make(chan interface{}),
			Events:      make(chan Event, 10),
			NetworkConn: server,
			Callback:    make(chan string, 10),
			Buffer:      make([]byte, 1024),
			Timeout:     200,
		},
		"2": {
			ID:          "2",
			Quit:        make(chan interface{}),
			Events:      make(chan Event, 10),
			NetworkConn: okServer,
			Callback:    make(chan string, 10),
			Buffer:      make([]byte, 1024),
			Timeout:     200,
		},
	}

	for _, c := range connections {
		go c.EventProcessor()
	}

	manager := TCPManager{
		connections: connections,
		quit:        make(chan interface{}),
		lock:        sync.RWMutex{},
	}

	var received string
	done := make(chan struct{})
	go func() {
		defer close(done)
		okClient.SetDeadline(time.Now().Add(time.Second))
		buf := make([]byte, len("Hi"))
		n, _ := io.ReadFull(okClient, buf)
		received = string(buf[:n])
	}()

	output, err := manager.Multicast("Hi", []net.Conn{server, okServer, failingConn}, 500*time.Millisecond)

	okServer.Close()
	<-done

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != "Hi" {
		t.Fatalf("expected the healthy target to receive the message, got %q", received)
	}

	if output.Sent != 3 {
		t.Fatalf("expected 3 sent, got %d", output.Sent)
	}
	if output.Errors < 2 {
		t.Fatalf("expected at least 2 errors (closed conn + unknown conn), got %d", output.Errors)
	}

	failedSet := make(map[net.Conn]bool)
	for _, c := range output.Failed {
		failedSet[c] = true
	}
	if !failedSet[server] {
		t.Errorf("closed connection should be reported as failed")
	}
	if !failedSet[failingConn] {
		t.Errorf("unknown connection should be reported as failed")
	}
}

// This test checks that a target which is not part of the manager's known
// connections is still counted and reported as a failure, so callers can
// deregister it.
func TestMulticastUnknownTargetIsFailure(t *testing.T) {
	manager := TCPManager{
		connections: make(map[string]Connection),
		quit:        make(chan interface{}),
		lock:        sync.RWMutex{},
	}

	unknown := &TestConn{}
	output, err := manager.Multicast("Hi", []net.Conn{unknown}, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Sent != 1 || output.Errors != 1 || output.Received != 0 {
		t.Fatalf("expected 0/1/1, got %d/%d/%d", output.Received, output.Sent, output.Errors)
	}
	if len(output.Failed) != 1 || output.Failed[0] != unknown {
		t.Fatalf("expected the unknown connection to be reported as failed, got %v", output.Failed)
	}
}

func TestMulticastWithNoTargetsIsNoop(t *testing.T) {
	manager := TCPManager{
		connections: make(map[string]Connection),
		quit:        make(chan interface{}),
		lock:        sync.RWMutex{},
	}

	output, err := manager.Multicast("Hi", nil, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Sent != 0 || output.Errors != 0 || output.Received != 0 || len(output.Failed) != 0 {
		t.Fatalf("expected an empty result, got %+v", output)
	}
}

// TelnetManager.Multicast just delegates to the wrapped TCPManager, but that
// delegation itself needs coverage: without this test the method is entirely
// unexercised.
func TestTelnetManagerMulticastDelegatesToTCPManager(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	connections := map[string]Connection{
		"1": {
			ID:          "1",
			Quit:        make(chan interface{}),
			Events:      make(chan Event, 10),
			NetworkConn: server,
			Callback:    make(chan string, 10),
			Buffer:      make([]byte, 1024),
			Timeout:     200,
		},
	}
	for _, c := range connections {
		go c.EventProcessor()
	}

	manager := &TelnetManager{
		tcpManager: &TCPManager{
			connections: connections,
			quit:        make(chan interface{}),
			lock:        sync.RWMutex{},
		},
	}

	done := make(chan struct{})
	var received string
	go func() {
		defer close(done)
		client.SetDeadline(time.Now().Add(time.Second))
		buf := make([]byte, len("Hi"))
		n, _ := io.ReadFull(client, buf)
		received = string(buf[:n])
	}()

	output, err := manager.Multicast("Hi", []net.Conn{server}, 500*time.Millisecond)
	<-done

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != "Hi" {
		t.Fatalf("expected the target to receive the message, got %q", received)
	}
	if output.Sent != 1 || output.Received != 1 || output.Errors != 0 {
		t.Fatalf("expected 1/1/0, got %+v", output)
	}
}

// HTTPManager.Multicast has its own connection snapshot logic (separate from
// TCPManager's), so it needs its own direct coverage rather than relying on
// the TCP-level tests above.
func TestHTTPManagerMulticastOnlyReachesTargets(t *testing.T) {
	servers := []net.Conn{}
	clients := []net.Conn{}
	connections := make(map[string]Connection)

	for i := 0; i < 3; i++ {
		server, client := net.Pipe()
		servers = append(servers, server)
		clients = append(clients, client)
		defer server.Close()
		defer client.Close()

		id := fmt.Sprintf("%d", i+1)
		c := &Connection{
			ID:          id,
			Quit:        make(chan interface{}),
			Events:      make(chan Event, 10),
			NetworkConn: server,
			Callback:    make(chan string, 10),
			Buffer:      make([]byte, 1024),
			Timeout:     200,
		}
		connections[id] = *c
		go c.EventProcessor()
	}

	manager := &HTTPManager{
		connections: connections,
		lock:        sync.RWMutex{},
	}

	target := servers[1]

	done := make(chan struct{})
	var received string
	go func() {
		defer close(done)
		clients[1].SetDeadline(time.Now().Add(time.Second))
		buf := make([]byte, len("Hey"))
		n, _ := io.ReadFull(clients[1], buf)
		received = string(buf[:n])
	}()

	output, err := manager.Multicast("Hey", []net.Conn{target}, 500*time.Millisecond)
	<-done

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received != "Hey" {
		t.Fatalf("expected the target to receive the message, got %q", received)
	}
	if output.Sent != 1 || output.Received != 1 || output.Errors != 0 {
		t.Fatalf("expected 1/1/0, got %+v", output)
	}

	// The non-targeted clients must not receive anything.
	for _, i := range []int{0, 2} {
		clients[i].SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		buf := make([]byte, 32)
		if _, err := clients[i].Read(buf); err == nil {
			t.Fatalf("client %d should not have received a multicast message", i)
		}
	}
}

// A target whose event queue is already full must be reported as a failure
// immediately, rather than blocking the multicast call.
func TestMulticastFullEventQueueMarksTargetFailed(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	events := make(chan Event, 1)
	events <- Event{} // fill the buffer so the next send takes the "queue full" path.

	connections := map[string]Connection{
		"1": {
			ID:          "1",
			Quit:        make(chan interface{}),
			Events:      events,
			NetworkConn: server,
			Callback:    make(chan string, 10),
			Buffer:      make([]byte, 1024),
			Timeout:     200,
		},
	}
	// Deliberately not starting EventProcessor: we are only checking that a
	// full Events channel is reported as a failed target.

	manager := TCPManager{
		connections: connections,
		quit:        make(chan interface{}),
		lock:        sync.RWMutex{},
	}

	output, err := manager.Multicast("Hi", []net.Conn{server}, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Sent != 1 || output.Errors != 1 || output.Received != 0 {
		t.Fatalf("expected 0/1/1, got %d/%d/%d", output.Received, output.Sent, output.Errors)
	}
	if len(output.Failed) != 1 || output.Failed[0] != server {
		t.Fatalf("expected server to be reported as failed, got %v", output.Failed)
	}
}

// A target that accepts the event but never acknowledges it must be reported
// as failed once the timeout elapses, rather than blocking the call forever.
func TestMulticastTimeoutMarksPendingTargetFailed(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	connections := map[string]Connection{
		"1": {
			ID:          "1",
			Quit:        make(chan interface{}),
			Events:      make(chan Event, 10),
			NetworkConn: server,
			Callback:    make(chan string, 10),
			Buffer:      make([]byte, 1024),
			Timeout:     200,
		},
	}
	// Deliberately not starting EventProcessor: the event is accepted into the
	// queue but never acknowledged, so the call must time out.

	manager := TCPManager{
		connections: connections,
		quit:        make(chan interface{}),
		lock:        sync.RWMutex{},
	}

	start := time.Now()
	output, err := manager.Multicast("Hi", []net.Conn{server}, 50*time.Millisecond)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Multicast should return promptly after the deadline, took %s", elapsed)
	}
	if output.Sent != 1 || output.Errors != 1 || output.Received != 0 {
		t.Fatalf("expected 0/1/1, got %d/%d/%d", output.Received, output.Sent, output.Errors)
	}
	if len(output.Failed) != 1 || output.Failed[0] != server {
		t.Fatalf("expected server to be reported as failed after timeout, got %v", output.Failed)
	}
}
