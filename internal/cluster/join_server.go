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
	addr    string
	user    string
	pass    string
	join    string
	nodeID  string
	cluster *BullyCluster
	ln      net.Listener
	server  *http.Server
	wg      sync.WaitGroup
}

func (s *joinServer) Start() error {
	slog.Info("Starting Cluster Join server")
	if len(s.join) < 1 {
		slog.Info("Starting bootstrapped node")
		s.cluster.Bootstrap()
	} else {
		slog.Info("Requesting to join cluster",
			slog.String("node_id", s.nodeID),
		)
		peers, leader, err := requestToJoin(s.join, s.cluster.config.ManagerAddr, s.nodeID, s.user, s.pass)
		if err != nil {
			return err
		}

		for id, addr := range peers {
			if id != s.nodeID {
				s.cluster.Join(id, addr)
			}
		}
		s.cluster.SetLeader(leader)
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
	switch r.URL.Path {
	case "/join":
		s.handleJoin(w, r)
	case "/remove":
		s.handleRemove(w, r)
	case "/election":
		s.handleElection(w, r)
	case "/coordinator":
		s.handleCoordinator(w, r)
	case "/heartbeat":
		s.handleHeartbeat(w, r)
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

	// Respond with current peer list and leader
	peers := s.cluster.GetPeers()
	peers[s.nodeID] = s.cluster.config.ManagerAddr

	response := map[string]interface{}{
		"peers":  peers,
		"leader": s.cluster.GetLeader(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	// Notify existing peers about the new node
	go s.notifyPeersOfNewNode(nodeID, remoteAddr)
}

func (s *joinServer) notifyPeersOfNewNode(newNodeID, newNodeAddr string) {
	peers := s.cluster.GetPeers()
	client := &http.Client{}

	for id, addr := range peers {
		if id == newNodeID || id == s.nodeID {
			continue
		}
		b, err := json.Marshal(map[string]string{"id": newNodeID, "addr": newNodeAddr})
		if err != nil {
			continue
		}
		req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/join", addr), bytes.NewReader(b))
		if err != nil {
			continue
		}
		req.Header.Add("Content-Type", "application/json")
		req.SetBasicAuth(s.user, s.pass)
		resp, err := client.Do(req)
		if err != nil {
			slog.Warn("Failed to notify peer of new node",
				slog.String("peer", id),
				slog.Any("error", err),
			)
		} else {
			resp.Body.Close()
		}
	}
}

func (s *joinServer) handleRemove(w http.ResponseWriter, r *http.Request) {
	slog.Info("Received request to be removed from cluster")

	if !s.cluster.IsLeader() {
		slog.Warn("Request to remove must be sent to Leader, I am not the leader")
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

func (s *joinServer) handleElection(w http.ResponseWriter, r *http.Request) {
	slog.Info("Received election message")
	user, pass, _ := r.BasicAuth()

	if user != s.user || pass != s.pass {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Respond OK to indicate this node is alive
	w.WriteHeader(http.StatusOK)

	// Start our own election since we have a higher ID
	go s.cluster.StartElection()
}

func (s *joinServer) handleCoordinator(w http.ResponseWriter, r *http.Request) {
	slog.Info("Received coordinator message")
	user, pass, _ := r.BasicAuth()

	if user != s.user || pass != s.pass {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	m := map[string]string{}
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		slog.Debug("JSON request cannot be decoded")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	nodeID, ok := m["id"]
	if !ok {
		slog.Debug("JSON request doesn't contain node ID")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	s.cluster.SetLeader(nodeID)
	slog.Info("New leader set from coordinator message",
		slog.String("leader", nodeID),
	)

	w.WriteHeader(http.StatusOK)
}

func (s *joinServer) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	s.cluster.mu.RLock()
	isUp := s.cluster.isUp
	s.cluster.mu.RUnlock()

	if isUp {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func requestToJoin(joinAddr, managerAddr, nodeID, user, pass string) (map[string]string, string, error) {
	b, err := json.Marshal(map[string]string{"addr": managerAddr, "id": nodeID})
	if err != nil {
		return nil, "", err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/join", joinAddr), bytes.NewReader(b))
	if err != nil {
		return nil, "", err
	}
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth(user, pass)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("failed to join cluster, response status: %s", resp.Status)
	}

	// Parse response for peers and leader
	var result struct {
		Peers  map[string]string `json:"peers"`
		Leader string            `json:"leader"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", fmt.Errorf("failed to decode join response: %w", err)
	}

	return result.Peers, result.Leader, nil
}
