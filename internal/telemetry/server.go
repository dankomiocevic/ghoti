package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// Config holds the metrics endpoint configuration. It is embedded in the
// top-level application config and populated from the "metrics:" YAML section.
type Config struct {
	// Enabled controls whether metrics are collected and served at all.
	// When false every metric call is a no-op and no listener is opened.
	Enabled bool

	// Addr is the host:port the metrics HTTP server listens on.
	// Required when Enabled is true.
	Addr string
}

// shutdownTimeout bounds how long Stop waits for in-flight scrapes. It is a
// variable so tests can shorten it.
var shutdownTimeout = 5 * time.Second

// Server serves GET /metrics on a dedicated HTTP listener.
type Server struct {
	cfg    Config
	ln     net.Listener
	server *http.Server
	wg     sync.WaitGroup
}

// NewServer creates a metrics server for cfg. Call Start to open the listener.
func NewServer(cfg Config) *Server {
	return &Server{cfg: cfg}
}

// Start binds the listener and begins serving in a background goroutine.
// A bind failure is returned synchronously so callers can abort startup.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	s.ln = ln

	mux := http.NewServeMux()
	mux.Handle("/metrics", Handler())
	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("metrics: started", slog.String("addr", ln.Addr().String()))

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("metrics: server error", slog.Any("error", err))
		}
	}()

	return nil
}

// Addr returns the address the server is listening on, or "" before Start.
func (s *Server) Addr() string {
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Stop shuts the server down and waits for the serving goroutine to exit.
// It is safe to call on a server that was never started.
func (s *Server) Stop() {
	if s.server == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		slog.Warn("metrics: shutdown error", slog.Any("error", err))
	}
	s.wg.Wait()
	slog.Info("metrics: stopped")
}
