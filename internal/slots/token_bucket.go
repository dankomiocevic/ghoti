package slots

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/dankomiocevic/ghoti/internal/auth"
)

type tokenBucketSlot struct {
	users        map[string]string
	value        int
	size         int
	period       int64
	rate         int
	window       int64
	tokensPerReq int
	now          func() time.Time
	mu           sync.Mutex
}

func newTokenBucketSlot(periodString string, bucketSize, refreshRate, tokensPerReq int, users map[string]string) (*tokenBucketSlot, error) {
	if bucketSize < 1 {
		return nil, fmt.Errorf("bucket size must be bigger than zero")
	}

	if refreshRate > bucketSize {
		return nil, fmt.Errorf("refresh rate cannot be bigger than the bucket size")
	}

	if refreshRate < 1 {
		return nil, fmt.Errorf("refresh rate cannot be zero")
	}

	if tokensPerReq > bucketSize {
		return nil, fmt.Errorf("tokens per request cannot be bigger than the bucket size")
	}

	if tokensPerReq < 1 {
		return nil, fmt.Errorf("tokens per request cannot be zero")
	}

	var period int64
	switch periodString {
	case "second":
		period = 1
	case "minute":
		period = 60
	case "hour":
		period = 3600
	default:
		return nil, fmt.Errorf("period value is invalid on token_bucket slot: %s", periodString)
	}

	return &tokenBucketSlot{value: refreshRate, size: bucketSize, period: period, rate: refreshRate, window: currentWindow(time.Now, period), tokensPerReq: tokensPerReq, users: users, now: time.Now}, nil
}

func currentWindow(now func() time.Time, period int64) int64 {
	return now().Unix() / period
}

func (m *tokenBucketSlot) Read() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	current := currentWindow(m.now, m.period)
	elapsed := current - m.window
	if elapsed > 0 {
		m.window = current
		m.refill(elapsed)
	} else if elapsed < 0 {
		// The clock moved backwards: don't hand out tokens for time that never
		// passed, just resync so refills resume from the new window.
		m.window = current
	}

	retVal := min(m.value, m.tokensPerReq)

	m.value -= retVal
	return strconv.Itoa(retVal)
}

// refill credits rate tokens for every elapsed period, saturating at size.
// The multiplication is guarded so a huge clock jump cannot overflow.
func (m *tokenBucketSlot) refill(elapsed int64) {
	missing := int64(m.size - m.value)
	rate := int64(m.rate)
	if elapsed >= (missing+rate-1)/rate {
		m.value = m.size
		return
	}
	// elapsed*rate < missing <= size here, so this cannot overflow.
	m.value += int(elapsed * rate)
}

func (m *tokenBucketSlot) CanRead(u *auth.User) bool {
	if len(m.users) == 0 {
		return true
	}

	return m.users[u.Name] == "r" || m.users[u.Name] == "a"
}

func (m *tokenBucketSlot) CanWrite(u *auth.User) bool {
	return false
}

func (m *tokenBucketSlot) Write(data string, from net.Conn) (string, error) {
	return "", fmt.Errorf("token bucket slots cannot be used to write")
}
