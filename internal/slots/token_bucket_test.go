package slots

import (
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

	expectedError := "Period value is invalid on token_bucket slot: invalid"
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
