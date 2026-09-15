package slots

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/dankomiocevic/ghoti/internal/auth"
)

func loadBucketSlot(t *testing.T) Slot {
	v := viper.New()

	v.Set("kind", "token_bucket")
	v.Set("bucket_size", 200)
	v.Set("refresh_rate", 100)
	v.Set("period", "second")
	v.Set("tokens_per_req", 20)
	v.Set("users.read", "r")
	v.Set("users.write", "w")
	v.Set("users.allu", "a")

	slot, err := GetSlot(v, nil, "")
	if err != nil {
		t.Fatalf("Slot must not return error: %s", err)
	}
	return slot
}

func TestTokenBucketSmoke(t *testing.T) {
	slot := loadBucketSlot(t)

	tokenSlot := slot.(*tokenBucketSlot)
	if tokenSlot.value != 100 {
		t.Errorf("Bucket must be started with 100 value: %d", tokenSlot.value)
	}
}

func TestTokenBucketRead(t *testing.T) {
	readUser, _ := auth.GetUser("read", "pass")
	writeUser, _ := auth.GetUser("write", "pass")
	allUser, _ := auth.GetUser("allu", "pass")

	slot := loadBucketSlot(t)

	if !slot.CanRead(&readUser) {
		t.Fatalf("we should be able to read with the read user")
	}

	if slot.CanRead(&writeUser) {
		t.Fatalf("we should not be able to read with the read user")
	}

	if !slot.CanRead(&allUser) {
		t.Fatalf("we should be able to read with the read/write user")
	}
}

func TestTokenBucketWrite(t *testing.T) {
	readUser, _ := auth.GetUser("read", "pass")
	writeUser, _ := auth.GetUser("write", "pass")
	allUser, _ := auth.GetUser("allu", "pass")

	slot := loadBucketSlot(t)

	if slot.CanWrite(&readUser) {
		t.Fatalf("we should not be able to write with any user")
	}

	if slot.CanWrite(&writeUser) {
		t.Fatalf("we should not be able to write with any user")
	}

	if slot.CanWrite(&allUser) {
		t.Fatalf("we should not be able to write with any user")
	}
}

func TestTokenBucketMissingConfig(t *testing.T) {
	v := viper.New()

	v.Set("kind", "token_bucket")

	_, err := GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error for missing config")
	}

	v.Set("bucket_size", 10)
	_, err = GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error for missing config")
	}
}

func TestTokenBucketWrongSize(t *testing.T) {
	v := viper.New()

	v.Set("kind", "token_bucket")
	v.Set("bucket_size", 0)

	_, err := GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error")
	}

	v.Set("bucket_size", "A")

	_, err = GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error")
	}
}

func TestTokenBucketWrongTokensPerReq(t *testing.T) {
	v := viper.New()

	v.Set("kind", "token_bucket")
	v.Set("period", "second")
	v.Set("bucket_size", 10)
	v.Set("refresh_rate", 10)
	v.Set("tokens_per_req", 12)

	_, err := GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error")
	}

	v.Set("tokens_per_req", 12)

	_, err = GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error")
	}

	v.Set("tokens_per_req", "A")

	_, err = GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error")
	}
}

func TestTokenBucketWrongRefreshRate(t *testing.T) {
	v := viper.New()

	v.Set("kind", "token_bucket")
	v.Set("period", "second")
	v.Set("bucket_size", 10)
	v.Set("refresh_rate", 11)

	_, err := GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error")
	}

	v.Set("tokens_per_request", "A")

	_, err = GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error")
	}

	v.Set("tokens_per_request", 0)

	_, err = GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error")
	}
}

func TestTokenBucketWrongPeriod(t *testing.T) {
	v := viper.New()

	v.Set("kind", "token_bucket")
	v.Set("period", "pepe")
	v.Set("bucket_size", 10)

	_, err := GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error")
	}

	v.Set("period", "")

	_, err = GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error")
	}

	v.Set("period", 0)

	_, err = GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error")
	}
}

func TestTokenBucketLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test on Short mode")
	}

	slot := loadBucketSlot(t)

	// Wait until the bucket is full (200 tokens)
	time.Sleep(1 * time.Second)
	i := 0
	for slot.Read() == "20" {
		i++
		if i > 11 {
			t.Fatalf("Slot must return zero after requesting all the tokens")
		}
	}

	if i < 10 {
		t.Fatalf("Slot must return at least 200 correct tokens. Correct: %d", i)
	}

	if slot.Read() != "0" {
		t.Fatalf("Slot must return zero after consuming all the tokens")
	}

	// Wait until the bucket gets a refresh (100 tokens)
	time.Sleep(1 * time.Second)

	i = 0
	for slot.Read() == "20" {
		i++
		if i > 5 {
			t.Fatalf("Slot must return zero after requesting all the tokens")
		}
	}

	if i < 5 {
		t.Fatalf("Slot must return at least 100 correct tokens. Correct: %d", i)
	}
}

func TestTokenBucketNotMatchingValues(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test on Short mode")
	}

	v := viper.New()

	v.Set("kind", "token_bucket")
	v.Set("bucket_size", 111)
	v.Set("refresh_rate", 65)
	v.Set("period", "second")
	v.Set("tokens_per_req", 7)

	slot, err := GetSlot(v, nil, "")
	if err != nil {
		t.Fatalf("Slot must not return error: %s", err)
	}

	// Wait until the bucket is full (111 tokens)
	time.Sleep(1 * time.Second)
	i := 0
	var value string
	for i <= 16 {
		i++
		value = slot.Read()
		if value != "7" {
			break
		}
	}
	if i > 16 {
		t.Fatalf("Slot must have no more tokens after 16 requests")
	}

	if value != "6" {
		t.Fatalf("Slot must return the missing 6 tokens")
	}

	if slot.Read() != "0" {
		t.Fatalf("Slot must return zero after consuming all the tokens")
	}

	// Wait until the bucket gets a refresh (65 tokens)
	time.Sleep(1 * time.Second)

	i = 0
	for slot.Read() == "7" {
		i++
		if i > 9 {
			t.Fatalf("Slot must return zero after requesting all the tokens")
		}
	}

	if i < 9 {
		t.Fatalf("Slot must return at least 111 correct tokens. Correct: %d", i)
	}
}

func TestTokenBucketZeroBucketSize(t *testing.T) {
	v := viper.New()

	v.Set("kind", "token_bucket")
	v.Set("bucket_size", 0)
	v.Set("refresh_rate", 10)
	v.Set("period", "second")
	v.Set("tokens_per_req", 5)

	_, err := GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error for zero bucket size")
	}
}

func TestTokenBucketZeroRefreshRate(t *testing.T) {
	v := viper.New()

	v.Set("kind", "token_bucket")
	v.Set("bucket_size", 100)
	v.Set("refresh_rate", 0)
	v.Set("period", "second")
	v.Set("tokens_per_req", 5)

	_, err := GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error for zero refresh rate")
	}
}

func TokenBucketWriteMethodReturnsError(t *testing.T) {
	slot := loadBucketSlot(t)

	// Try writing to the token bucket slot
	result, err := slot.Write("some data", nil)

	if err == nil {
		t.Fatalf("Write method should return an error for token bucket slots")
	}

	if result != "" {
		t.Fatalf("Write method should return empty string, got: %s", result)
	}

	expectedErrorMsg := "Token bucket slots cannot be used to write"
	if err.Error() != expectedErrorMsg {
		t.Fatalf("Expected error message '%s', got: '%s'", expectedErrorMsg, err.Error())
	}
}

func TestTokenBucketWithMinutePeriod(t *testing.T) {
	v := viper.New()

	v.Set("kind", "token_bucket")
	v.Set("bucket_size", 100)
	v.Set("refresh_rate", 50)
	v.Set("period", "minute")
	v.Set("tokens_per_req", 10)

	slot, err := GetSlot(v, nil, "")
	if err != nil {
		t.Fatalf("Slot must not return error for minute period: %s", err)
	}

	tokenSlot := slot.(*tokenBucketSlot)
	if tokenSlot.period != 60 {
		t.Fatalf("Period should be 60 seconds for minute period, got: %d", tokenSlot.period)
	}
}

func TestTokenBucketWithHourPeriod(t *testing.T) {
	v := viper.New()

	v.Set("kind", "token_bucket")
	v.Set("bucket_size", 100)
	v.Set("refresh_rate", 50)
	v.Set("period", "hour")
	v.Set("tokens_per_req", 10)

	slot, err := GetSlot(v, nil, "")
	if err != nil {
		t.Fatalf("Slot must not return error for hour period: %s", err)
	}

	tokenSlot := slot.(*tokenBucketSlot)
	if tokenSlot.period != 3600 {
		t.Fatalf("Period should be 3600 seconds for hour period, got: %d", tokenSlot.period)
	}
}

func TestTokenBucketWithInvalidPeriod(t *testing.T) {
	users := make(map[string]string)

	_, err := newTokenBucketSlot("invalid", 100, 50, 10, users)
	if err == nil {
		t.Fatalf("Expected error when creating token bucket with invalid period")
	}

	expectedError := "period value is invalid on token_bucket slot: invalid"
	if err.Error() != expectedError {
		t.Fatalf("Expected error message '%s', got '%s'", expectedError, err.Error())
	}
}

func TokenBucketNegativeTokensPerRequest(t *testing.T) {
	users := make(map[string]string)

	_, err := newTokenBucketSlot("second", 100, 50, -5, users)
	if err == nil {
		t.Fatalf("Expected error when creating token bucket with negative tokens per request")
	}

	expectedError := "Tokens per request cannot be zero"
	if err.Error() != expectedError {
		t.Fatalf("Expected error message '%s', got '%s'", expectedError, err.Error())
	}
}

func TokenBucketEmptyUserMap(t *testing.T) {
	// Create a token bucket with an empty users map
	slot, err := newTokenBucketSlot("second", 100, 50, 10, map[string]string{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Create a user
	user, _ := auth.GetUser("anyone", "pass")

	// Should have permission when user map is empty
	if !slot.CanRead(&user) {
		t.Fatalf("Expected any user to have read permission when users map is empty")
	}

	if slot.CanWrite(&user) {
		t.Fatalf("Expected no write permission regardless of users map")
	}
}

// fakeClock lets tests move the token bucket's clock deterministically.
type fakeClock struct {
	mu  sync.Mutex
	sec int64
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Unix(c.sec, 0)
}

func (c *fakeClock) set(sec int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sec = sec
}

// newClockedBucket returns a token bucket driven by a fake clock that starts
// at the beginning of the given window.
func newClockedBucket(t *testing.T, period string, bucketSize, refreshRate, tokensPerReq int, startSec int64) (*tokenBucketSlot, *fakeClock) {
	t.Helper()
	slot, err := newTokenBucketSlot(period, bucketSize, refreshRate, tokensPerReq, nil)
	if err != nil {
		t.Fatalf("Slot must not return error: %s", err)
	}
	clock := &fakeClock{sec: startSec}
	slot.now = clock.now
	slot.window = currentWindow(clock.now, slot.period)
	return slot, clock
}

// drain reads until the bucket returns zero and returns the total tokens handed out.
func drain(t *testing.T, slot *tokenBucketSlot) int {
	t.Helper()
	total := 0
	for i := 0; i < 100000; i++ {
		n, err := strconv.Atoi(slot.Read())
		if err != nil {
			t.Fatalf("Read must return an integer: %s", err)
		}
		if n < 0 {
			t.Fatalf("Read must never return a negative number of tokens: %d", n)
		}
		if n == 0 {
			return total
		}
		total += n
	}
	t.Fatalf("bucket never drained")
	return total
}

func TestTokenBucketInitialState(t *testing.T) {
	slot, _ := newClockedBucket(t, "second", 200, 100, 20, 1000)

	// The bucket starts with refresh_rate tokens, not bucket_size.
	if slot.value != 100 {
		t.Fatalf("bucket must start with refresh_rate tokens, got %d", slot.value)
	}

	// Tokens are available immediately, without waiting for the first period.
	if got := slot.Read(); got != "20" {
		t.Fatalf("first read must return tokens_per_req, got %s", got)
	}
	if got := drain(t, slot); got != 80 {
		t.Fatalf("bucket must hand out the remaining 80 initial tokens, got %d", got)
	}
}

func TestTokenBucketNoRefillWithinSameWindow(t *testing.T) {
	slot, clock := newClockedBucket(t, "second", 200, 100, 20, 1000)

	if got := drain(t, slot); got != 100 {
		t.Fatalf("expected 100 initial tokens, got %d", got)
	}

	// Still inside the same one-second window: nothing must be refilled.
	clock.set(1000)
	if got := slot.Read(); got != "0" {
		t.Fatalf("no refill expected within the same window, got %s", got)
	}
}

func TestTokenBucketRefillOnWindowBoundary(t *testing.T) {
	slot, clock := newClockedBucket(t, "second", 200, 100, 20, 1000)
	drain(t, slot)

	// Crossing exactly one boundary adds exactly one refresh_rate.
	clock.set(1001)
	if got := drain(t, slot); got != 100 {
		t.Fatalf("one window boundary must add refresh_rate tokens, got %d", got)
	}

	// And the next boundary adds another one.
	clock.set(1002)
	if got := drain(t, slot); got != 100 {
		t.Fatalf("next window boundary must add refresh_rate tokens, got %d", got)
	}
}

func TestTokenBucketMultiplePeriodsAccumulate(t *testing.T) {
	slot, clock := newClockedBucket(t, "second", 1000, 100, 50, 1000)
	drain(t, slot)

	// Three full periods elapsed: 3 * refresh_rate must be credited, not 1 * refresh_rate.
	clock.set(1003)
	if got := drain(t, slot); got != 300 {
		t.Fatalf("three elapsed periods must add 300 tokens, got %d", got)
	}
}

func TestTokenBucketLongIdleSaturatesAtBucketSize(t *testing.T) {
	slot, clock := newClockedBucket(t, "second", 200, 100, 20, 1000)
	drain(t, slot)

	// Idle for many periods: the bucket must be full, and never exceed bucket_size.
	clock.set(1000 + 60)
	if got := drain(t, slot); got != 200 {
		t.Fatalf("long idle must fill the bucket to bucket_size, got %d", got)
	}
}

func TestTokenBucketMinutePeriodMultipleWindows(t *testing.T) {
	slot, clock := newClockedBucket(t, "minute", 500, 100, 100, 0)
	drain(t, slot)

	// 59 seconds later is still the same minute window.
	clock.set(59)
	if got := slot.Read(); got != "0" {
		t.Fatalf("no refill expected within the same minute, got %s", got)
	}

	// 2 minutes and a bit later: two windows crossed.
	clock.set(150)
	if got := drain(t, slot); got != 200 {
		t.Fatalf("two elapsed minutes must add 200 tokens, got %d", got)
	}
}

func TestTokenBucketHugeClockJumpDoesNotOverflow(t *testing.T) {
	slot, clock := newClockedBucket(t, "second", 1<<40, 1<<40, 1<<40, 1000)
	drain(t, slot)

	// elapsed * rate would overflow int64; the result must saturate at bucket_size.
	clock.set(1<<62 + 1000)
	if got := slot.Read(); got != strconv.Itoa(1<<40) {
		t.Fatalf("huge clock jump must saturate at bucket_size, got %s", got)
	}
	if slot.value != 0 {
		t.Fatalf("bucket must be empty after handing out bucket_size tokens, got %d", slot.value)
	}
}

func TestTokenBucketClockMovingBackwardsGrantsNoTokens(t *testing.T) {
	slot, clock := newClockedBucket(t, "second", 200, 100, 20, 1000)
	drain(t, slot)

	// The clock steps backwards (e.g. NTP correction): this must not be a free refill.
	clock.set(990)
	if got := slot.Read(); got != "0" {
		t.Fatalf("clock moving backwards must not grant tokens, got %s", got)
	}

	// Once the clock advances past the (new) window, exactly one refresh is credited.
	clock.set(991)
	if got := drain(t, slot); got != 100 {
		t.Fatalf("one boundary after a backwards step must add refresh_rate tokens, got %d", got)
	}
}

func TestTokenBucketConcurrentReadsNeverOverIssue(t *testing.T) {
	const (
		size    = 1000
		rate    = 100
		perReq  = 7
		workers = 32
	)
	slot, clock := newClockedBucket(t, "second", size, rate, perReq, 1000)

	// Fill the bucket completely, then hammer it from many goroutines while
	// moving the clock. Total tokens issued must never exceed what the bucket
	// could have legitimately accumulated.
	clock.set(1000 + size)

	var issued int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				n, err := strconv.Atoi(slot.Read())
				if err != nil || n < 0 || n > perReq {
					t.Errorf("invalid read result %q (%v)", strconv.Itoa(n), err)
					return
				}
				atomic.AddInt64(&issued, int64(n))
			}
		}()
	}
	wg.Wait()

	// The clock was fixed during the run, so only the initial full bucket was available.
	if issued != size {
		t.Fatalf("expected exactly %d tokens issued under concurrency, got %d", size, issued)
	}
	if slot.value < 0 {
		t.Fatalf("bucket value must never go negative, got %d", slot.value)
	}

	// Advancing two periods afterwards credits exactly 2 * rate.
	clock.set(1000 + size + 2)
	if got := drain(t, slot); got != 2*rate {
		t.Fatalf("two periods after concurrent drain must add %d tokens, got %d", 2*rate, got)
	}
}
