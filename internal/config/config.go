package config

import (
	"fmt"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/slots"
	"github.com/spf13/viper"
)

type Config struct {
	TcpAddr string
	Slots   [1000]slots.Slot
	Users   map[string]auth.User
}

func DefaultConfig() *Config {
	return &Config{
		TcpAddr: "localhost:9090",
		Slots:   [1000]slots.Slot{},
		Users:   make(map[string]auth.User),
	}
}

func LoadConfig() (*Config, error) {
	config := DefaultConfig()

	err := viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to load server config: %w", err)
		}
	}

	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal server config: %w", err)
	}

	config.LoadUsers()
	config.ConfigureSlots()
	return config, nil
}

func (c *Config) LoadUsers() {
	if viper.IsSet("users") {
		usersMap := viper.GetStringMap("users")
		for key, value := range usersMap {
			pass := fmt.Sprintf("%v", value)
			u, _ := auth.GetUser(key, pass)
			c.Users[key] = u
		}
	}
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

func (c *Config) Verify() error {
	return nil
}
