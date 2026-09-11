package cluster

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureLogs runs fn with the default logger pointed at a buffer set to the
// most verbose level, and returns everything that was written.
func captureLogs(t *testing.T, fn func()) string {
	t.Helper()

	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)

	fn()
	return buf.String()
}

// A failed join attempt must never disclose the cluster secret. Anybody able
// to reach the endpoint can trigger this log line, so leaking the configured
// password here hands the shared secret to whoever reads the logs.
func TestFailedJoinDoesNotLogCredentials(t *testing.T) {
	const (
		serverUser   = "cluster_user"
		serverPass   = "the_shared_cluster_secret"
		attackerUser = "attacker_user"
		attackerPass = "attacker_guess"
	)

	config := ClusterConfig{Node: "node1", User: serverUser, Pass: serverPass, ManagerAddr: "localhost:5345"}
	s := &joinServer{
		addr:    "localhost:5345",
		user:    serverUser,
		pass:    serverPass,
		nodeID:  "node1",
		cluster: newTestCluster(config),
	}

	output := captureLogs(t, func() {
		req := httptest.NewRequest("POST", "/join", strings.NewReader(`{"id":"node2","addr":"localhost:5346"}`))
		req.SetBasicAuth(attackerUser, attackerPass)
		w := httptest.NewRecorder()

		s.handleJoin(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected the request to be rejected, got %d", w.Code)
		}
	})

	for _, secret := range []string{serverPass, attackerPass} {
		if strings.Contains(output, secret) {
			t.Fatalf("credential %q leaked into the logs: %s", secret, output)
		}
	}
}

func TestFailedRemoveDoesNotLogCredentials(t *testing.T) {
	const (
		serverUser = "cluster_user"
		serverPass = "the_shared_cluster_secret"
	)

	config := ClusterConfig{Node: "node1", User: serverUser, Pass: serverPass, ManagerAddr: "localhost:5345"}
	cluster := newTestCluster(config)
	cluster.leader = "node1"

	s := &joinServer{
		addr:    "localhost:5345",
		user:    serverUser,
		pass:    serverPass,
		nodeID:  "node1",
		cluster: cluster,
	}

	output := captureLogs(t, func() {
		req := httptest.NewRequest("POST", "/remove", strings.NewReader(`{"id":"node2"}`))
		req.SetBasicAuth("attacker_user", "attacker_guess")
		w := httptest.NewRecorder()

		s.handleRemove(w, req)
	})

	if strings.Contains(output, serverPass) {
		t.Fatalf("cluster secret leaked into the logs: %s", output)
	}
}
