package slots

import (
	"errors"
	"fmt"
	"net"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/dankomiocevic/ghoti/internal/connection_manager"
	"github.com/spf13/viper"
)

type Slot interface {
	Read() string
	Write(string, net.Conn) (string, error)
	CanRead(*auth.User) bool
	CanWrite(*auth.User) bool
}

func GetSlot(v *viper.Viper, conn connection_manager.ConnectionManager, id string) (Slot, error) {
	kind := v.GetString("kind")
	usersConfig := v.GetStringMap("users")

	users := make(map[string]string)
	if usersConfig != nil {
		for key, value := range usersConfig {
			users[key] = fmt.Sprintf("%v", value)
		}
	}

	if kind == "simple_memory" {
		return &memorySlot{value: "", users: users}, nil
	}

	if kind == "timeout_memory" {
		if !v.IsSet("timeout") {
			return nil, fmt.Errorf("timeout value must be set for timeout_memory slot")
		}
		timeoutConfig := v.GetInt("timeout")
		timeoutSlot, err := newTimeoutSlot(timeoutConfig, users)
		if err != nil {
			return nil, err
		}
		return timeoutSlot, nil
	}

	if kind == "token_bucket" {
		if !v.IsSet("bucket_size") {
			return nil, fmt.Errorf("bucket_size must be set for token_bucket slot")
		}
		bucketSize := v.GetInt("bucket_size")

		if !v.IsSet("period") {
			return nil, fmt.Errorf("period must be set for token_bucket slot")
		}
		periodString := v.GetString("period")

		refreshRate := 1
		if v.IsSet("refresh_rate") {
			refreshRate = v.GetInt("refresh_rate")
		}

		tokensPerReq := 1
		if v.IsSet("tokens_per_req") {
			tokensPerReq = v.GetInt("tokens_per_req")
		}

		tokenBucket, err := newTokenBucketSlot(periodString, bucketSize, refreshRate, tokensPerReq, users)
		if err != nil {
			return nil, err
		}

		return tokenBucket, nil
	}

	if kind == "leaky_bucket" {
		if !v.IsSet("bucket_size") {
			return nil, fmt.Errorf("bucket_size must be set for token_bucket slot")
		}
		bucketSize := v.GetInt("bucket_size")

		refreshRate := 1000
		if v.IsSet("refresh_rate") {
			refreshRate = v.GetInt("refresh_rate")
		}

		leakyBucket, err := newLeakyBucketSlot(bucketSize, refreshRate, users)
		if err != nil {
			return nil, err
		}

		return leakyBucket, nil
	}

	if kind == "ticker" {
		initialValue := 0
		if !v.IsSet("initial_value") {
			return nil, fmt.Errorf("initial_value must be set for ticker slot")
		} else {
			initialValue = v.GetInt("initial_value")
		}

		if initialValue < 0 {
			return nil, fmt.Errorf("Initial value cannot be negative")
		}

		refreshRate := 1000
		if !v.IsSet("refresh_rate") {
			return nil, fmt.Errorf("refresh_rate must be set for ticker slot")
		} else {
			refreshRate = v.GetInt("refresh_rate")
		}

		tickerSlot, err := newTickerSlot(refreshRate, initialValue, users)
		if err != nil {
			return nil, err
		}

		return tickerSlot, nil
	}

	if kind == "broadcast" {
		broadcastSlot, err := newBroadcastSlot(users, conn, id)
		if err != nil {
			return nil, err
		}

		return broadcastSlot, nil
	}

	if kind == "atomic" {
		return &atomicSlot{value: 0, users: users}, nil
	}

	return nil, errors.New("Invalid kind of slot")
}
