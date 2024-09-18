package slots

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/spf13/viper"
)

type Slot interface {
	Read() string
	Write(string, net.Conn) (string, error)
	CanRead(*auth.User) bool
	CanWrite(*auth.User) bool
}

func GetSlot(v *viper.Viper) (Slot, error) {
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
		// TODO: validate this data
		timeoutConfig := v.GetInt("timeout")
		return &timeoutSlot{value: "", timeout: time.Duration(timeoutConfig) * time.Second, ttl: time.Time{}, users: users}, nil
	}

	return nil, errors.New("Invalid kind of slot")
}
