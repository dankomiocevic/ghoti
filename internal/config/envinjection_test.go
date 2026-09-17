package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// Credentials must be injectable from the environment so that a deployment
// does not have to write them into a configuration file that risks being
// committed. This only works when the env key replacer maps dots to
// underscores, otherwise the nested key is unreachable from the environment.
func TestClusterPasswordCanComeFromEnvironment(t *testing.T) {
	t.Setenv("GHOTI_CLUSTER_PASS", "injected_secret")
	t.Setenv("GHOTI_CLUSTER_USER", "injected_user")

	setupViperForTest(t, `
cluster:
  node: node1
  manager:
    type: join_server
    addr: localhost:25873
`)

	config := DefaultConfig()
	if err := config.LoadCluster(); err != nil {
		t.Fatalf("failed to load cluster config: %v", err)
	}

	if config.Cluster.Pass != "injected_secret" {
		t.Errorf("cluster password was not read from the environment, got %q", config.Cluster.Pass)
	}
	if config.Cluster.User != "injected_user" {
		t.Errorf("cluster user was not read from the environment, got %q", config.Cluster.User)
	}
}

func setupViperForTest(t *testing.T, contents string) {
	t.Helper()

	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.SetConfigType("yaml")
	viper.SetEnvPrefix("GHOTI")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadConfig(strings.NewReader(contents)); err != nil {
		t.Fatalf("failed to read test config: %v", err)
	}
}
