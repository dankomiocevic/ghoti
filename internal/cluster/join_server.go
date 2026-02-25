package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
)

type joinServer struct {
	addr        string
	user        string
	pass        string
	join        string
	nodeID      string
	clusterBind string
	cluster     Cluster
	ln          net.Listener
	server      *http.Server
	wg          sync.WaitGroup
}

func (s *joinServer) Start() error {
	slog.Info("Starting Cluster Join server")
	if len(s.join) < 1 {
		slog.Info("Starting bootstrapped node")
		future := s.cluster.Bootstrap()
		err := future.Error()
		if err != nil {
			return err
		}
	} else {
		slog.Info("Requesting to join cluster",
			slog.String("node_id", s.nodeID),
		)
		err := requestToJoin(s.join, s.clusterBind, s.nodeID, s.user, s.pass)
		if err != nil {
			return err
		}
	}

	server := &http.Server{
		Handler: s,
	}
	s.server = server

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		err := server.Serve(s.ln)
		if err != nil && err != http.ErrServerClosed {
			slog.Error("Error starting server",
				slog.Any("error", err),
			)
		}
	}()

	return nil
}

func (s *joinServer) Close() {
	slog.Info("Closing Cluster Join server")
	s.server.Shutdown(context.Background())
	s.wg.Wait()
}

func (s *joinServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/join" {
		s.handleJoin(w, r)
	}

	if r.URL.Path == "/remove" {
		s.handleRemove(w, r)
	}
}

func (s *joinServer) handleJoin(w http.ResponseWriter, r *http.Request) {
	slog.Info("Received request to join cluster")
	user, pass, _ := r.BasicAuth()

	if user != s.user || pass != s.pass {
		slog.Warn("Request to join with wrong username/password",
			"user", user,
			"pass", pass,
			"s_user", s.user,
			"s_pass", s.pass,
		)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	m := map[string]string{}
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		slog.Debug("JSON request cannot be decoded")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if len(m) != 2 {
		slog.Debug("JSON request doesn't have enough elements")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	remoteAddr, ok := m["addr"]
	if !ok {
		slog.Debug("JSON request doesn't contain remote address")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	nodeID, ok := m["id"]
	if !ok {
		slog.Debug("JSON request doesn't contain node ID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := s.cluster.Join(nodeID, remoteAddr)
	if err != nil {
		slog.Warn("Error joining cluster",
			slog.Any("error", err),
		)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *joinServer) handleRemove(w http.ResponseWriter, r *http.Request) {
	slog.Info("Received request to be removed from cluster")

	if !s.cluster.IsLeader() {
		slog.Warn("Request to remove must be sent to Leader, I am not the leader",
			slog.String("state", s.cluster.state().String()),
		)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	user, pass, _ := r.BasicAuth()
	if user != s.user || pass != s.pass {
		slog.Warn("Request to remove with wrong username/password")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	m := map[string]string{}
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		slog.Debug("JSON request cannot be decoded")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if len(m) < 1 {
		slog.Debug("JSON request doesn't have enough elements")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	nodeID, ok := m["id"]
	if !ok {
		slog.Debug("JSON request doesn't contain node ID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := s.cluster.Remove(nodeID)
	if err != nil {
		slog.Warn("Error removing node from cluster",
			slog.Any("error", err),
		)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func requestToJoin(joinAddr, raftAddr, nodeID, user, pass string) error {
	b, err := json.Marshal(map[string]string{"addr": raftAddr, "id": nodeID})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/join", joinAddr), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth(user, pass)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.Status != "200 OK" {
		return fmt.Errorf("failed to join cluster, response status: %s", resp.Status)
	}

	return nil
}
