package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
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
}

func (s *joinServer) Start() error {
	if len(s.join) < 1 {
		future := s.cluster.Bootstrap()
		err := future.Error()
		if err != nil {
			return err
		}
	} else {
		err := requestToJoin(s.join, s.clusterBind, s.nodeID)
		if err != nil {
			return err
		}
	}

	server := http.Server{
		Handler: s,
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln

	go func() {
		err := server.Serve(s.ln)
		if err != nil {
			// TODO: Add logs
			return
		}
	}()

	return nil
}

func (s *joinServer) Close() {
	return
}

func (s *joinServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/join" {
		s.handleJoin(w, r)
	}
}

func (s *joinServer) handleJoin(w http.ResponseWriter, r *http.Request) {
	user, pass, ok := r.BasicAuth()

	if user != s.user || pass != s.pass {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	m := map[string]string{}
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if len(m) != 2 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	remoteAddr, ok := m["addr"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	nodeID, ok := m["id"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := s.cluster.Join(nodeID, remoteAddr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func requestToJoin(joinAddr, raftAddr, nodeID string) error {
	b, err := json.Marshal(map[string]string{"addr": raftAddr, "id": nodeID})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/join", joinAddr), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	req.SetBasicAuth("my_user", "my_pass")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.Status != "200 OK" {
		return fmt.Errorf("Failed to join cluster, response status: %s", resp.Status)
	}

	return nil
}
