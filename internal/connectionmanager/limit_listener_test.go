package connectionmanager

import (
	"net"
	"testing"
	"time"
)

// acceptOne runs Accept in the background and returns the channel it reports
// on, so a test can tell an Accept that is parked from one that returned.
func acceptOne(l net.Listener) <-chan net.Conn {
	out := make(chan net.Conn, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			out <- nil
			return
		}
		out <- c
	}()
	return out
}

func TestLimitListenerHoldsConnectionsOverTheLimit(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l := limitListener(inner, 1)
	defer l.Close()

	first, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	accepted := <-acceptOne(l)
	if accepted == nil {
		t.Fatal("first connection was not accepted")
	}

	second, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	// With one connection live, the second must not be handed out yet.
	pending := acceptOne(l)
	select {
	case <-pending:
		t.Fatal("second connection was accepted while the limit was reached")
	case <-time.After(200 * time.Millisecond):
	}

	// Closing the first connection frees its slot for the waiting one.
	accepted.Close()
	select {
	case c := <-pending:
		if c == nil {
			t.Fatal("Accept failed after a slot was freed")
		}
		c.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("second connection was not accepted after the first closed")
	}
}

func TestLimitListenerClosingAConnectionTwiceFreesOneSlot(t *testing.T) {
	// A double Close must not hand back two slots, or the limit silently grows.
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l := limitListener(inner, 1)
	defer l.Close()

	for i := 0; i < 2; i++ {
		client, err := net.Dial("tcp", l.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		c := <-acceptOne(l)
		if c == nil {
			t.Fatalf("connection %d was not accepted", i)
		}
		c.Close()
		c.Close()
		client.Close()
	}

	a, _ := net.Dial("tcp", l.Addr().String())
	defer a.Close()
	held := <-acceptOne(l)
	if held == nil {
		t.Fatal("connection was not accepted")
	}
	defer held.Close()

	b, _ := net.Dial("tcp", l.Addr().String())
	defer b.Close()
	select {
	case <-acceptOne(l):
		t.Fatal("limit grew: a second connection was accepted alongside a live one")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestLimitListenerZeroMeansUnlimited(t *testing.T) {
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer inner.Close()
	if l := limitListener(inner, 0); l != inner {
		t.Fatal("a limit of 0 should return the listener untouched")
	}
}

func TestLimitListenerCloseUnblocksParkedAccept(t *testing.T) {
	// The serving loops stop when Accept returns an error after Close. An
	// Accept waiting for a free slot must return then too, not stay parked.
	inner, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l := limitListener(inner, 1)

	client, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	held := <-acceptOne(l)
	if held == nil {
		t.Fatal("connection was not accepted")
	}
	defer held.Close()

	parked := acceptOne(l)
	time.Sleep(100 * time.Millisecond)
	l.Close()

	select {
	case c := <-parked:
		if c != nil {
			c.Close()
			t.Fatal("expected an error from Accept after Close, got a connection")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Accept stayed parked after the listener was closed")
	}
}

// readsSomething reports whether any byte arrives on c within timeout.
func readsSomething(c net.Conn, timeout time.Duration) bool {
	c.SetReadDeadline(time.Now().Add(timeout))
	_, err := c.Read(make([]byte, 1))
	return err == nil
}

func TestManagersHonourMaxConnections(t *testing.T) {
	cases := map[string]struct {
		manager ConnectionManager
		request string
	}{
		"tcp":    {NewTCPManager(), "r000\n"},
		"telnet": {NewTelnetManager(), "r000\r\n"},
		"http":   {NewHTTPManager(), "GET /000 HTTP/1.1\r\nHost: x\r\n\r\n"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m := tc.manager
			m.SetMaxConnections(1)
			if err := m.StartListening("127.0.0.1:0"); err != nil {
				t.Fatalf("StartListening: %v", err)
			}
			go m.ServeConnections(echoCallback) //nolint:errcheck
			defer m.Close()

			first, err := net.Dial("tcp", m.GetAddr())
			if err != nil {
				t.Fatal(err)
			}
			defer first.Close()
			// Give the accept loop a moment to take the first connection.
			time.Sleep(50 * time.Millisecond)

			second, err := net.Dial("tcp", m.GetAddr())
			if err != nil {
				t.Fatal(err)
			}
			defer second.Close()
			if _, err := second.Write([]byte(tc.request)); err != nil {
				t.Fatal(err)
			}

			if readsSomething(second, 300*time.Millisecond) {
				t.Fatal("second connection was served while the first held the only slot")
			}

			first.Close()
			if !readsSomething(second, 2*time.Second) {
				t.Fatal("second connection was not served after the first closed")
			}
		})
	}
}
