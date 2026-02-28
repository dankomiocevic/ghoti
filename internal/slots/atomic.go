package slots

import (
	"fmt"
	"math"
	"net"
	"strconv"
	"sync"

	"github.com/dankomiocevic/ghoti/internal/auth"
)

type atomicSlot struct {
	users map[string]string
	value int64
	mu    sync.RWMutex
}

func (a *atomicSlot) Read() string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if math.MaxInt64 == a.value {
		a.value = 0
	} else {
		a.value++
	}

	return strconv.FormatInt(a.value, 10)
}

func (a *atomicSlot) CanRead(u *auth.User) bool {
	if len(a.users) == 0 {
		return true
	}

	return a.users[u.Name] == "r" || a.users[u.Name] == "a"
}

func (a *atomicSlot) CanWrite(u *auth.User) bool {
	if len(a.users) == 0 {
		return true
	}

	return a.users[u.Name] == "w" || a.users[u.Name] == "a"
}

func (a *atomicSlot) Write(data string, from net.Conn) (string, error) {
	dataInt, err := strconv.ParseInt(data, 10, 64)
	if err != nil {
		return "", fmt.Errorf("data must be an integer")
	}

	if dataInt < 0 {
		return "", fmt.Errorf("data cannot be negative")
	}

	a.mu.Lock()
	a.value = dataInt
	a.mu.Unlock()

	return strconv.FormatInt(dataInt, 10), nil
}
