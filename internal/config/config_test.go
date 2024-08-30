package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestConfigureSlot(t *testing.T) {
	viper.Reset()

	viper.Set("slot_000.kind", "simple_memory")

	config := DefaultConfig()
	config.ConfigureSlots()

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("Failed loading configuration")
	}

	if config.Slots[0] == nil {
		t.Fatalf("slot zero not configured")
	}
}

func TestConfigureTimeoutSlot(t *testing.T) {
	viper.Reset()

	viper.Set("slot_000.kind", "timeout_memory")
	viper.Set("slot_000.timeout", 50)

	config := DefaultConfig()
	config.ConfigureSlots()

	if config.Slots[0] == nil {
		t.Fatalf("slot zero not configured")
	}
}

func TestNotConfigureSlot(t *testing.T) {
	viper.Reset()

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
	viper.Reset()

	viper.Set("slot_000.kind", "unknown")

	config := DefaultConfig()
	config.ConfigureSlots()

	if config.Slots[0] != nil {
		t.Fatalf("slot zero should not be configured")
	}
}
