package config

import (
	"fmt"
	"log/slog"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/cluster"
	"github.com/dankomiocevic/ghoti/internal/slots"
	"github.com/spf13/viper"
)

var SupportedLogLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelDebug,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

var SupportedLogFormat = map[string]bool{
	"text": true,
	"json": true,
}

type LoggingConfig struct {
	Level  slog.Level
	Format string
}

type Config struct {
	TcpAddr string
	Slots   [1000]slots.Slot
	Users   map[string]auth.User
	Cluster cluster.ClusterConfig
	Logging LoggingConfig
}

func DefaultConfig() *Config {
	return &Config{
		TcpAddr: "localhost:9090",
		Slots:   [1000]slots.Slot{},
		Users:   make(map[string]auth.User),
		Cluster: cluster.ClusterConfig{},
		Logging: LoggingConfig{Level: slog.LevelInfo, Format: "text"},
	}
}

func LoadConfig() (*Config, error) {
	config := DefaultConfig()

	err := viper.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load server config: %w", err)
	}

	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal server config: %w", err)
	}

	if viper.IsSet("addr") {
		config.TcpAddr = viper.GetString("addr")
	}

	e := config.ConfigureLogging()
	if e != nil {
		return nil, e
	}

	config.ConfigureSlots()

	e = config.LoadUsers()
	if e != nil {
		return nil, e
	}

	e = config.LoadCluster()
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
			return fmt.Errorf("Cluster node name must be less than 20 characters")
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
		if viper.IsSet(key) {
			slot, _ := slots.GetSlot(viper.Sub(key))
			c.Slots[i] = slot
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
			return fmt.Errorf("Log level not supported: %s", c.Logging.Level)
		}
	}

	if viper.IsSet("log.format") {
		c.Logging.Format = viper.GetString("log.format")
		if SupportedLogFormat[c.Logging.Format] != true {
			return fmt.Errorf("Log format not supported: %s", c.Logging.Format)
		}
	}
	return nil
}

func (c *Config) Verify() error {
	return nil
}
