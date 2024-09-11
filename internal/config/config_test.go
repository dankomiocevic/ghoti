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

func TestUserSetup(t *testing.T) {
	viper.Reset()

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

func TestMultipleUsersSetup(t *testing.T) {
	viper.Reset()

	viper.Set("users.pepe", "SomePassword")
	viper.Set("users.bob", "OtherPassword")

	config := DefaultConfig()
	config.LoadUsers()

	if len(config.Users) != 2 {
		t.Fatal("number of users created is wrong")
	}

	if config.Users["pepe"].Name != "pepe" {
		t.Fatalf("user name must be pepe")
	}

	if config.Users["bob"].Name != "bob" {
		t.Fatalf("user name must be bob")
	}

	if config.Users["pepe"].Password != "SomePassword" {
		t.Fatalf("User pepe password must be SomePassword")
	}

	if config.Users["bob"].Password != "OtherPassword" {
		t.Fatalf("User bob password must be OtherPassword")
	}
}
