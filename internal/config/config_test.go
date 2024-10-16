package config

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func resetViper() {
	viper.Reset()
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.SetEnvPrefix("GHOTI")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	configPaths := []string{"/etc/ghoti", "$HOME/.ghoti", ".", "../.."}
	for _, path := range configPaths {
		viper.AddConfigPath(path)
	}
}

func TestConfigureSlot(t *testing.T) {
	resetViper()
	viper.Set("slot_000.kind", "simple_memory")

	config := DefaultConfig()
	config.ConfigureSlots()

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("Failed loading configuration: %s", err)
	}

	if config.Slots[0] == nil {
		t.Fatalf("slot zero not configured")
	}
}

func TestConfigureTimeoutSlot(t *testing.T) {
	resetViper()

	viper.Set("slot_000.kind", "timeout_memory")
	viper.Set("slot_000.timeout", 50)

	config := DefaultConfig()
	config.ConfigureSlots()

	if config.Slots[0] == nil {
		t.Fatalf("slot zero not configured")
	}
}

func TestNotConfigureSlot(t *testing.T) {
	resetViper()

	viper.Set("slot_000.kind", "simple_memory")
	viper.Set("slot_000.timeout", 50)

	config := DefaultConfig()
	config.ConfigureSlots()

	for i := 1; i < 1000; i++ {
		if config.Slots[i] != nil {
			t.Fatalf("slot %d should not be configured", i)
		}
	}
}

func TestConfigureUnknownType(t *testing.T) {
	resetViper()

	viper.Set("slot_000.kind", "unknown")

	config := DefaultConfig()
	config.ConfigureSlots()

	if config.Slots[0] != nil {
		t.Fatalf("slot zero should not be configured")
	}
}

func TestUserSetup(t *testing.T) {
	resetViper()

	viper.Set("users.pepe", "SomePassword")

	config := DefaultConfig()
	config.LoadUsers()

	if config.Users["pepe"].Name != "pepe" {
		t.Fatalf("user name must be pepe")
	}

	if config.Users["pepe"].Password != "SomePassword" {
		t.Fatalf("User pepe password must be SomePassword")
	}
}

func TestEmptyPassword(t *testing.T) {
	resetViper()

	viper.Set("users.pepe", "")

	config := DefaultConfig()
	e := config.LoadUsers()

	if e == nil {
		t.Fatalf("User creation must fail with no password")
	}
}

func TestMultipleUsersSetup(t *testing.T) {
	resetViper()

	viper.Set("users.pepe", "SomePassword")
	viper.Set("users.bobby", "OtherPassword")

	config := DefaultConfig()
	config.LoadUsers()

	if len(config.Users) != 2 {
		t.Fatal("number of users created is wrong")
	}

	if config.Users["pepe"].Name != "pepe" {
		t.Fatalf("user name must be pepe")
	}

	if config.Users["bobby"].Name != "bobby" {
		t.Fatalf("user name must be bobby")
	}

	if config.Users["pepe"].Password != "SomePassword" {
		t.Fatalf("User pepe password must be SomePassword")
	}

	if config.Users["bobby"].Password != "OtherPassword" {
		t.Fatalf("User bobby password must be OtherPassword")
	}
}

func TestClusterConfig(t *testing.T) {
	resetViper()

	viper.Set("cluster.node", "some_node")
	viper.Set("cluster.bind", "10.0.0.1:8765")
	viper.Set("cluster.user", "pepe")
	viper.Set("cluster.pass", "shadow")
	viper.Set("cluster.manager.type", "join_server")
	viper.Set("cluster.manager.join", "10.0.0.31:3456")
	viper.Set("cluster.manager.addr", "10.0.0.1:3456")

	config := DefaultConfig()
	err := config.LoadCluster()
	if err != nil {
		t.Fatalf("cluster configuration failed to load: %s", err)
	}

	if config.Cluster.Bind != "10.0.0.1:8765" {
		t.Fatalf("bind cluster configuration does not match: %s", config.Cluster.Bind)
	}

	if config.Cluster.User != "pepe" {
		t.Fatalf("user cluster configuration does not match: %s", config.Cluster.User)
	}

	if config.Cluster.Pass != "shadow" {
		t.Fatalf("pass cluster configuration does not match: %s", config.Cluster.Pass)
	}

	if config.Cluster.ManagerType != "join_server" {
		t.Fatalf("cluster manager type does not match: %s", config.Cluster.ManagerType)
	}

	if config.Cluster.ManagerJoin != "10.0.0.31:3456" {
		t.Fatalf("cluster manager join does not match: %s", config.Cluster.ManagerJoin)
	}

	if config.Cluster.ManagerAddr != "10.0.0.1:3456" {
		t.Fatalf("cluster manager addr does not match: %s", config.Cluster.ManagerAddr)
	}
}

func TestClusterMissingClusterUser(t *testing.T) {
	resetViper()

	viper.Set("cluster.node", "some_node")
	viper.Set("cluster.bind", "10.0.0.1:8765")
	viper.Set("cluster.pass", "shadow")
	viper.Set("cluster.manager.type", "join_server")
	viper.Set("cluster.manager.join", "10.0.0.31:3456")
	viper.Set("cluster.manager.addr", "10.0.0.1:3456")

	config := DefaultConfig()
	err := config.LoadCluster()
	if err == nil {
		t.Fatalf("cluster configuration must fail for missing user")
	}
}

func TestClusterMissingClusterPass(t *testing.T) {
	resetViper()

	viper.Set("cluster.node", "some_node")
	viper.Set("cluster.bind", "10.0.0.1:8765")
	viper.Set("cluster.user", "pepe")
	viper.Set("cluster.manager.type", "join_server")
	viper.Set("cluster.manager.join", "10.0.0.31:3456")
	viper.Set("cluster.manager.addr", "10.0.0.1:3456")

	config := DefaultConfig()
	err := config.LoadCluster()
	if err == nil {
		t.Fatalf("cluster configuration must fail for missing pass")
	}
}

func TestClusterMissingNodeName(t *testing.T) {
	resetViper()

	viper.Set("cluster.bind", "10.0.0.1:8765")
	viper.Set("cluster.user", "pepe")
	viper.Set("cluster.pass", "shadow")
	viper.Set("cluster.manager.type", "join_server")
	viper.Set("cluster.manager.join", "10.0.0.31:3456")
	viper.Set("cluster.manager.addr", "10.0.0.1:3456")

	config := DefaultConfig()
	err := config.LoadCluster()
	if err == nil {
		t.Fatalf("cluster configuration must fail for missing pass")
	}
}

func TestClusterNodeNameTooLong(t *testing.T) {
	resetViper()

	viper.Set("cluster.node", "abcdefghijklmnopqrstuvwxyz1234567890")
	viper.Set("cluster.bind", "10.0.0.1:8765")
	viper.Set("cluster.user", "pepe")
	viper.Set("cluster.pass", "shadow")
	viper.Set("cluster.manager.type", "join_server")
	viper.Set("cluster.manager.join", "10.0.0.31:3456")
	viper.Set("cluster.manager.addr", "10.0.0.1:3456")

	config := DefaultConfig()
	err := config.LoadCluster()
	if err == nil {
		t.Fatalf("cluster configuration must fail for missing pass")
	}
}

func TestClusterMissingManagerType(t *testing.T) {
	resetViper()

	viper.Set("cluster.node", "some_node")
	viper.Set("cluster.bind", "10.0.0.1:8765")
	viper.Set("cluster.user", "pepe")
	viper.Set("cluster.pass", "shadow")
	viper.Set("cluster.manager.join", "10.0.0.31:3456")
	viper.Set("cluster.manager.addr", "10.0.0.1:3456")

	config := DefaultConfig()
	err := config.LoadCluster()
	if err == nil {
		t.Fatalf("cluster configuration must fail for missing pass")
	}
}

func TestClusterDefaultBind(t *testing.T) {
	resetViper()

	viper.Set("cluster.node", "some_node")
	viper.Set("cluster.user", "pepe")
	viper.Set("cluster.pass", "shadow")
	viper.Set("cluster.manager.type", "join_server")
	viper.Set("cluster.manager.join", "10.0.0.31:3456")
	viper.Set("cluster.manager.addr", "10.0.0.1:3456")

	config := DefaultConfig()
	err := config.LoadCluster()
	if err != nil {
		t.Fatalf("error creating cluster configuration: %s", err)
	}

	if config.Cluster.Bind != "localhost:25873" {
		t.Fatalf("bind cluster default configuration does not match: %s", config.Cluster.Bind)
	}
}

func TestLoggingLevel(t *testing.T) {
	resetViper()

	viper.Set("log.level", "warn")
	config := DefaultConfig()

	err := config.ConfigureLogging()
	if err != nil {
		t.Fatalf("error loading logging configuration: %s", err)
	}

	if config.Logging.Level != slog.LevelWarn {
		t.Fatalf("Wrong logging level configured")
	}
}

func TestLoggingWrongLevel(t *testing.T) {
	resetViper()

	viper.Set("log.level", "pepe")
	config := DefaultConfig()

	err := config.ConfigureLogging()
	if err == nil {
		t.Fatalf("Logging configuration must fail with wrong level")
	}
}

func TestLoggingFormat(t *testing.T) {
	resetViper()

	viper.Set("log.format", "json")
	config := DefaultConfig()

	err := config.ConfigureLogging()
	if err != nil {
		t.Fatalf("error loading logging configuration: %s", err)
	}

	if config.Logging.Format != "json" {
		t.Fatalf("Wrong logging format configured")
	}
}

func TestLoggingWrongFormat(t *testing.T) {
	resetViper()

	viper.Set("log.format", "pepe")
	config := DefaultConfig()

	err := config.ConfigureLogging()
	if err == nil {
		t.Fatalf("Logging configuration must fail with wrong format")
	}
}
