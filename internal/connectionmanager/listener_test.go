package connectionmanager

import (
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/dankomiocevic/ghoti/internal/logging/logtest"
)

func noopCallback(_ int, _ []byte, _ *Connection) error { return nil }

// occupyPort binds a random free port and returns its address, so a manager
// pointed at it is guaranteed to fail binding.
func occupyPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("couldn't open the blocking listener: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l.Addr().String()
}

// Serving before a listener exists must be reported as an error instead of
// dereferencing the nil listener.
func TestServeConnectionsWithoutListener(t *testing.T) {
	managers := map[string]ConnectionManager{
		"tcp":    NewTCPManager(),
		"telnet": NewTelnetManager(),
		"http":   NewHTTPManager(),
	}

	for name, manager := range managers {
		t.Run(name, func(t *testing.T) {
			err := manager.ServeConnections(noopCallback)
			if !errors.Is(err, ErrNotListening) {
				t.Fatalf("expected ErrNotListening, got %v", err)
			}
		})
	}
}

// Closing a manager that never managed to listen must not panic.
func TestCloseWithoutListener(t *testing.T) {
	managers := map[string]ConnectionManager{
		"tcp":    NewTCPManager(),
		"telnet": NewTelnetManager(),
		"http":   NewHTTPManager(),
	}

	for name, manager := range managers {
		t.Run(name, func(t *testing.T) {
			manager.Close()
		})
	}
}

func TestStartListeningOnAddressInUse(t *testing.T) {
	addr := occupyPort(t)

	managers := map[string]ConnectionManager{
		"tcp":    NewTCPManager(),
		"telnet": NewTelnetManager(),
		"http":   NewHTTPManager(),
	}

	for name, manager := range managers {
		t.Run(name, func(t *testing.T) {
			if err := manager.StartListening(addr); err == nil {
				manager.Close()
				t.Fatalf("expected an error binding an address already in use")
			}
		})
	}
}

// The HTTP manager binds in StartListening, so GetAddr must report the port
// the OS actually picked rather than the ":0" it was configured with.
func TestHTTPManagerGetAddrReportsBoundPort(t *testing.T) {
	manager := NewHTTPManager()
	if manager.GetAddr() != "" {
		t.Fatalf("expected an empty address before listening, got %q", manager.GetAddr())
	}

	if err := manager.StartListening("127.0.0.1:0"); err != nil {
		t.Fatalf("Error starting the listener: %s", err)
	}
	defer manager.Close()

	_, port, err := net.SplitHostPort(manager.GetAddr())
	if err != nil {
		t.Fatalf("unexpected address %q: %v", manager.GetAddr(), err)
	}
	if port == "0" || port == "" {
		t.Fatalf("expected a real bound port, got %q", manager.GetAddr())
	}
}

// ServeConnections must serve requests over the listener opened by
// StartListening, and return cleanly once the manager is closed.
func TestHTTPManagerServeConnections(t *testing.T) {
	manager := NewHTTPManager()
	if err := manager.StartListening("127.0.0.1:0"); err != nil {
		t.Fatalf("Error starting the listener: %s", err)
	}

	served := make(chan error, 1)
	go func() {
		served <- manager.ServeConnections(func(_ int, _ []byte, conn *Connection) error {
			return conn.SendResponse("v000hello\n")
		})
	}()

	resp, err := http.Get("http://" + manager.GetAddr() + "/000")
	if err != nil {
		t.Fatalf("couldn't send request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: %d", resp.StatusCode)
	}

	manager.Close()
	if err := <-served; err != nil {
		t.Fatalf("ServeConnections returned an error after Close: %v", err)
	}
}

// A failure inside Serve other than a clean shutdown must be reported.
func TestHTTPManagerServeConnectionsReportsListenerError(t *testing.T) {
	manager := NewHTTPManager()
	if err := manager.StartListening("127.0.0.1:0"); err != nil {
		t.Fatalf("Error starting the listener: %s", err)
	}

	// Closing the listener underneath Serve makes Accept fail, which is not
	// the ErrServerClosed a graceful Shutdown produces.
	manager.listener.Close()

	if err := manager.ServeConnections(noopCallback); err == nil {
		t.Fatalf("expected an error serving over a closed listener")
	}
}

// Closing a manager while a client is still connected has to stop the
// connection handler through its Quit channel. The client never hangs up, so
// the handler only notices when a read times out and it checks Quit again.
func TestCloseWithClientStillConnected(t *testing.T) {
	logtest.EnableDebug(t)

	managers := map[string]struct {
		newManager func() lineManager
		line       string
	}{
		"tcp":    {func() lineManager { return NewTCPManager() }, "r001\n"},
		"telnet": {func() lineManager { return NewTelnetManager() }, "r001\r\n"},
	}

	for name, tt := range managers {
		t.Run(name, func(t *testing.T) {
			manager := tt.newManager()
			if err := manager.StartListening("127.0.0.1:0"); err != nil {
				t.Fatal(err)
			}

			received := make(chan string, 1)
			go manager.ServeConnections(func(size int, buf []byte, c *Connection) error {
				received <- string(buf[:size])
				return nil
			})

			client, err := net.Dial("tcp", manager.GetAddr())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()

			// Wait until the handler is running, otherwise Close could find no
			// connection to stop.
			if _, err := client.Write([]byte(tt.line)); err != nil {
				t.Fatal(err)
			}
			expectReceived(t, received, "r001")

			closed := make(chan struct{})
			go func() {
				manager.Close()
				close(closed)
			}()

			select {
			case <-closed:
			case <-time.After(2 * time.Second):
				t.Fatal("Close did not return while a client was still connected")
			}
		})
	}
}

// Deleting a connection that is not registered, for example because it was
// already deleted, must leave the manager untouched.
func TestTCPManagerDeleteUnknownConnection(t *testing.T) {
	logtest.EnableDebug(t)

	manager := NewTCPManager()
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	connection := manager.Add(server, 41)
	manager.Delete(connection.ID)
	manager.Delete(connection.ID)

	if len(manager.connections) != 0 {
		t.Fatalf("expected no connections left, got %d", len(manager.connections))
	}
}
