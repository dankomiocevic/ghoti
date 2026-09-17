package telemetry

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func startTestServer(t *testing.T) *Server {
	t.Helper()

	srv := NewServer(Config{Enabled: true, Addr: "127.0.0.1:0"})
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start metrics server: %s", err)
	}
	t.Cleanup(srv.Stop)
	return srv
}

func TestServerServesMetricsEndpoint(t *testing.T) {
	resetGlobal()
	Enable()
	RecordRequest(time.Millisecond)

	srv := startTestServer(t)

	resp, err := http.Get("http://" + srv.Addr() + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics failed: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected text exposition content type, got %q", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %s", err)
	}
	if !strings.Contains(string(body), "ghoti_requests_total 1") {
		t.Errorf("expected ghoti_requests_total 1 in body, got:\n%s", body)
	}
}

func TestServerOnlyServesMetricsPath(t *testing.T) {
	resetGlobal()
	srv := startTestServer(t)

	resp, err := http.Get("http://" + srv.Addr() + "/")
	if err != nil {
		t.Fatalf("GET / failed: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for /, got %d", resp.StatusCode)
	}
}

func TestServerStartFailsOnUnbindableAddr(t *testing.T) {
	srv := NewServer(Config{Enabled: true, Addr: "256.256.256.256:1"})
	if err := srv.Start(); err == nil {
		srv.Stop()
		t.Fatalf("expected an error binding to an invalid address")
	}
}

func TestServerStopReleasesListener(t *testing.T) {
	resetGlobal()
	srv := NewServer(Config{Enabled: true, Addr: "127.0.0.1:0"})
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start metrics server: %s", err)
	}
	addr := srv.Addr()
	srv.Stop()

	if _, err := http.Get("http://" + addr + "/metrics"); err == nil {
		t.Errorf("expected connection to fail after Stop")
	}
}

func TestServerStopWithoutStartDoesNotPanic(t *testing.T) {
	srv := NewServer(Config{Enabled: true, Addr: "127.0.0.1:0"})
	srv.Stop()
}

func TestServerAddrIsEmptyBeforeStart(t *testing.T) {
	srv := NewServer(Config{Enabled: true, Addr: "127.0.0.1:0"})
	if addr := srv.Addr(); addr != "" {
		t.Errorf("expected empty addr before Start, got %q", addr)
	}
}

func TestServerStopReturnsAfterListenerFailure(t *testing.T) {
	resetGlobal()
	srv := NewServer(Config{Enabled: true, Addr: "127.0.0.1:0"})
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start metrics server: %s", err)
	}

	// Kill the listener underneath the server, as the OS would on an
	// interface going away, and wait for the serving goroutine to notice.
	srv.ln.Close()
	srv.wg.Wait()

	done := make(chan struct{})
	go func() {
		srv.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(shutdownTimeout + time.Second):
		t.Fatalf("Stop hung after the listener failed")
	}
}

func TestServerStopDoesNotWaitForeverOnStuckClient(t *testing.T) {
	resetGlobal()

	prev := shutdownTimeout
	shutdownTimeout = 100 * time.Millisecond
	t.Cleanup(func() { shutdownTimeout = prev })

	srv := NewServer(Config{Enabled: true, Addr: "127.0.0.1:0"})
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start metrics server: %s", err)
	}

	// A client that connects but never sends a request keeps a connection
	// open that graceful shutdown cannot drain.
	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatalf("failed to connect: %s", err)
	}
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		srv.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(shutdownTimeout + time.Second):
		t.Fatalf("Stop hung on a client that never sent a request")
	}
}
