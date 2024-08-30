package slots

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/spf13/viper"
)

type Slot interface {
	Read() string
	Write(string, net.Conn) (string, error)
}

type memorySlot struct {
	value string
	mu    sync.Mutex
}

func (m *memorySlot) Read() string {
	return m.value
}

func (m *memorySlot) Write(data string, from net.Conn) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.value = data
	return m.value, nil
}

type timeoutSlot struct {
	value   string
	owner   net.Conn
	timeout time.Duration
	ttl     time.Time
	mu      sync.Mutex
}

func (m *timeoutSlot) Read() string {
	return m.value
}

func (m *timeoutSlot) Write(data string, from net.Conn) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	timeNow := time.Now()
	if timeNow.After(m.ttl) {
		m.owner = from
		m.value = data
		m.ttl = timeNow.Add(m.timeout)

		return m.value, nil
	}

	if from == m.owner {
		m.value = data
		m.ttl = timeNow.Add(m.timeout)

		return m.value, nil
	}

	return "", errors.New("Permission denied to write slot")
}

func GetSlot(v *viper.Viper) (Slot, error) {
	kind := v.GetString("kind")

	if kind == "simple_memory" {
		return &memorySlot{value: ""}, nil
	}

	if kind == "timeout_memory" {
		// TODO: validate this data
		timeoutConfig := v.GetInt("timeout")
		return &timeoutSlot{value: "", timeout: time.Duration(timeoutConfig) * time.Second, ttl: time.Time{}}, nil
	}

	return nil, errors.New("Invalid kind of slot")
}
