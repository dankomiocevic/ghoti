package cluster

import (
	"encoding/json"
	"net"
	"net/http"
)

type joinServer struct {
	addr    string
	user    string
	pass    string
	cluster Cluster
	ln      net.Listener
}

func (s *joinServer) Start() error {
	server := http.Server{
		Handler: s,
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln

	http.Handle("/", s)

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
	s.ln.Close()
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
