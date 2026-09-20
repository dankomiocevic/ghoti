package config

import (
	"fmt"
	"log/slog"

	"github.com/spf13/viper"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/cluster"
	"github.com/dankomiocevic/ghoti/internal/connectionmanager"
	"github.com/dankomiocevic/ghoti/internal/slots"
	"github.com/dankomiocevic/ghoti/internal/telemetry"
)

var SupportedLogLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

var SupportedLogFormat = map[string]bool{
	"text": true,
	"json": true,
}

var SupportedProtocols = map[string]bool{
	"standard": true,
	"telnet":   true,
	"http":     true,
}

type LoggingConfig struct {
	Level  slog.Level
	Format string
}

type Config struct {
	TCPAddr        string
	MaxConnections int
	Slots          [1000]slots.Slot
	StreamingSlots map[int]bool
	Users          map[string]auth.User
	Cluster        cluster.ClusterConfig
	Logging        LoggingConfig
	Metrics        telemetry.Config
	Connections    connectionmanager.ConnectionManager
	Protocol       string
}

func DefaultConfig() *Config {
	return &Config{
		TCPAddr:        "localhost:9090",
		Slots:          [1000]slots.Slot{},
		StreamingSlots: make(map[int]bool),
		Users:          make(map[string]auth.User),
		Cluster:        cluster.ClusterConfig{},
		Logging:        LoggingConfig{Level: slog.LevelInfo, Format: "text"},
		Protocol:       "standard",
	}
}

func LoadConfig() (*Config, error) {
	config := DefaultConfig()

	err := viper.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load server config: %w", err)
	}

	if viper.IsSet("addr") {
		config.TCPAddr = viper.GetString("addr")
	}

	if viper.IsSet("protocol") {
		config.Protocol = viper.GetString("protocol")

		if !SupportedProtocols[config.Protocol] {
			return nil, fmt.Errorf("protocol not supported: %s", config.Protocol)
		}
	}

	if viper.IsSet("max_connections") {
		config.MaxConnections = viper.GetInt("max_connections")
		if config.MaxConnections < 0 {
			return nil, fmt.Errorf("max_connections must be zero or a positive number, got %d", config.MaxConnections)
		}
	}

	e := config.ConfigureLogging()
	if e != nil {
		return nil, e
	}

	//TODO: Move this out of the config package
	config.Connections = connectionmanager.GetConnectionManager(config.Protocol)

	config.ConfigureSlots()

	e = config.LoadUsers()
	if e != nil {
		return nil, e
	}

	e = config.LoadCluster()
	if e != nil {
		return nil, e
	}

	e = config.LoadMetrics()
	if e != nil {
		return nil, e
	}

	return config, nil
}

func (c *Config) LoadCluster() error {
	if viper.IsSet("cluster") {
		c.Cluster = cluster.ClusterConfig{}
		if !viper.IsSet("cluster.node") {
			return fmt.Errorf("failed to load cluster node configuration, no node provided")
		}
		c.Cluster.Node = viper.GetString("cluster.node")
		// TODO: Only letters, and numbers no spaces
		if len(c.Cluster.Node) > 20 {
			return fmt.Errorf("cluster node name must be less than 20 characters")
		}

		if !viper.IsSet("cluster.bind") {
			c.Cluster.Bind = "localhost:25873"
		} else {
			c.Cluster.Bind = viper.GetString("cluster.bind")
		}

		if !viper.IsSet("cluster.user") || !viper.IsSet("cluster.pass") {
			return fmt.Errorf("failed to load cluster node configuration, no user provided")
		}
		c.Cluster.User = viper.GetString("cluster.user")
		c.Cluster.Pass = viper.GetString("cluster.pass")

		if !viper.IsSet("cluster.manager.type") {
			return fmt.Errorf("failed to load cluster node configuration, no manager provided")
		}
		c.Cluster.ManagerType = viper.GetString("cluster.manager.type")

		if viper.IsSet("cluster.manager.join") {
			c.Cluster.ManagerJoin = viper.GetString("cluster.manager.join")
		}

		if viper.IsSet("cluster.manager.addr") {
			c.Cluster.ManagerAddr = viper.GetString("cluster.manager.addr")
		}

		// The leader endpoint is opt-in: it is only served when explicitly
		// enabled with cluster.leader.enabled.
		c.Cluster.LeaderEnabled = viper.GetBool("cluster.leader.enabled")
	}

	return nil
}

func (c *Config) LoadUsers() error {
	if viper.IsSet("users") {
		usersMap := viper.GetStringMap("users")
		for key, value := range usersMap {
			pass := fmt.Sprintf("%v", value)
			u, e := auth.GetUser(key, pass)
			if e != nil {
				return e
			}

			c.Users[key] = u
		}
	}
	return nil
}

func (c *Config) ConfigureSlots() {
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("slot_%03d", i)
		num := fmt.Sprintf("%03d", i)
		if viper.IsSet(key) {
			sub := viper.Sub(key)
			slot, _ := slots.GetSlot(sub, c.Connections, num)
			c.Slots[i] = slot
			if sub.GetString("kind") == "broadcast" {
				c.StreamingSlots[i] = true
			}
		}
	}
}

func (c *Config) ConfigureLogging() error {
	if viper.IsSet("log.level") {
		logLevel := viper.GetString("log.level")
		lvl, ok := SupportedLogLevels[logLevel]
		if ok {
			c.Logging.Level = lvl
		} else {
			return fmt.Errorf("log level not supported: %s", c.Logging.Level)
		}
	}

	if viper.IsSet("log.format") {
		c.Logging.Format = viper.GetString("log.format")
		if !SupportedLogFormat[c.Logging.Format] {
			return fmt.Errorf("log format not supported: %s", c.Logging.Format)
		}
	}
	return nil
}

func (c *Config) Verify() error {
	return nil
}

// removedMetricsKeys are the keys of the old file-based metrics writer. They
// are rejected explicitly so an upgraded deployment fails fast instead of
// silently producing no metrics.
var removedMetricsKeys = []string{"output_dir", "rotation", "retain", "interval"}

// LoadMetrics reads the optional "metrics:" YAML section and populates c.Metrics.
// Metrics are disabled by default; they must be explicitly enabled with
// "metrics.enabled: true".
func (c *Config) LoadMetrics() error {
	if !viper.IsSet("metrics") {
		return nil
	}

	c.Metrics.Enabled = viper.GetBool("metrics.enabled")
	if !c.Metrics.Enabled {
		return nil
	}

	for _, key := range removedMetricsKeys {
		if viper.IsSet("metrics." + key) {
			return fmt.Errorf("metrics.%s is no longer supported: metrics are served over HTTP at metrics.addr instead of written to files", key)
		}
	}

	c.Metrics.Addr = viper.GetString("metrics.addr")
	if c.Metrics.Addr == "" {
		return fmt.Errorf("metrics.addr is required when metrics is enabled")
	}

	return nil
}
