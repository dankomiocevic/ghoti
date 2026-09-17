package run

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/mock"
)

type DummyExit struct {
	mock.Mock
}

func (d *DummyExit) Exit(code int) {
	d.Called(code)
}

func TestNoConfig(t *testing.T) {
	viper.Reset()
	viper.AddConfigPath("whattayoutalkingbout")
	viper.SetConfigName("wrong_name")
	viper.SetConfigType("yaml")

	e := new(DummyExit)
	e.On("Exit", 1).Return()
	runWithExit(e)

	e.AssertExpectations(t)
}

func TestClusterFail(t *testing.T) {
	viper.Reset()
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.SetEnvPrefix("GHOTI")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()

	configPaths := []string{"/etc/ghoti", "$HOME/.ghoti", ".", "../.."}
	for _, path := range configPaths {
		viper.AddConfigPath(path)
	}

	viper.Set("cluster.node", "node1")
	viper.Set("cluster.user", "a")
	viper.Set("cluster.pass", "b")
	viper.Set("cluster.manager.type", "pepe")
	viper.Set("cluster.manager.join", "pepe")
	viper.Set("cluster.manager.addr", "pepe")

	e := new(DummyExit)
	e.On("Exit", 3).Return()
	runWithExit(e)

	e.AssertExpectations(t)
}

func TestServerBindFail(t *testing.T) {
	viper.Reset()
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.SetEnvPrefix("GHOTI")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()

	configPaths := []string{"/etc/ghoti", "$HOME/.ghoti", ".", "../.."}
	for _, path := range configPaths {
		viper.AddConfigPath(path)
	}

	// Occupy a port so the server cannot bind it.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("couldn't open the blocking listener: %v", err)
	}
	defer l.Close()
	viper.Set("addr", l.Addr().String())

	e := new(DummyExit)
	e.On("Exit", 4).Return()
	runWithExit(e)

	e.AssertExpectations(t)
}

func TestMetricsBindFail(t *testing.T) {
	viper.Reset()
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.SetEnvPrefix("GHOTI")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()

	configPaths := []string{"/etc/ghoti", "$HOME/.ghoti", ".", "../.."}
	for _, path := range configPaths {
		viper.AddConfigPath(path)
	}

	// Occupy a port so the metrics server cannot bind it.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("couldn't open the blocking listener: %v", err)
	}
	defer l.Close()
	viper.Set("metrics.enabled", true)
	viper.Set("metrics.addr", l.Addr().String())

	e := new(DummyExit)
	e.On("Exit", 5).Return()
	runWithExit(e)

	e.AssertExpectations(t)
}

// freePort asks the kernel for an unused TCP port on the loopback interface.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("couldn't find a free port: %v", err)
	}
	defer l.Close()
	return l.Addr().String()
}

func TestMetricsEndpointServed(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test on Short mode")
	}

	viper.Reset()
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.SetEnvPrefix("GHOTI")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()

	configPaths := []string{"/etc/ghoti", "$HOME/.ghoti", ".", "../.."}
	for _, path := range configPaths {
		viper.AddConfigPath(path)
	}

	serverAddr := freePort(t)
	metricsAddr := freePort(t)
	viper.Set("addr", serverAddr)
	viper.Set("metrics.enabled", true)
	viper.Set("metrics.addr", metricsAddr)

	// runWithExit blocks until a signal arrives; no exit code is expected.
	e := new(DummyExit)
	go runWithExit(e)

	// Wait for the metrics listener to come up.
	metricsURL := "http://" + metricsAddr + "/metrics"
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get(metricsURL)
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("metrics endpoint never came up: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Send one request through the TCP protocol so the counter moves.
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		t.Fatalf("couldn't connect to the server: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("r000\n")); err != nil {
		t.Fatalf("couldn't send request: %v", err)
	}
	if _, err := bufio.NewReader(conn).ReadBytes('\n'); err != nil {
		t.Fatalf("couldn't read response: %v", err)
	}

	resp, err := http.Get(metricsURL)
	if err != nil {
		t.Fatalf("GET /metrics failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading /metrics body: %v", err)
	}

	if !strings.Contains(string(body), "ghoti_requests_total 1") {
		t.Fatalf("expected ghoti_requests_total 1 in scrape, got:\n%s", body)
	}
	e.AssertNotCalled(t, "Exit", mock.Anything)
}
