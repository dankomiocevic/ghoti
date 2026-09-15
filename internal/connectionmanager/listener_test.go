package connectionmanager

import (
	"errors"
	"net"
	"net/http"
	"testing"
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
			return conn.SendEvent("v000hello\n")
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
