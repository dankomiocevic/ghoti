package slots

import (
	"testing"
	"time"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/spf13/viper"
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
	read_user, _ := auth.GetUser("read", "pass")
	write_user, _ := auth.GetUser("write", "pass")
	all_user, _ := auth.GetUser("allu", "pass")

	slot := loadBucketSlot(t)

	if !slot.CanRead(&read_user) {
		t.Fatalf("we should be able to read with the read user")
	}

	if slot.CanRead(&write_user) {
		t.Fatalf("we should not be able to read with the read user")
	}

	if !slot.CanRead(&all_user) {
		t.Fatalf("we should be able to read with the read/write user")
	}
}

func TestTokenBucketWrite(t *testing.T) {
	read_user, _ := auth.GetUser("read", "pass")
	write_user, _ := auth.GetUser("write", "pass")
	all_user, _ := auth.GetUser("allu", "pass")

	slot := loadBucketSlot(t)

	if slot.CanWrite(&read_user) {
		t.Fatalf("we should not be able to write with any user")
	}

	if slot.CanWrite(&write_user) {
		t.Fatalf("we should not be able to write with any user")
	}

	if slot.CanWrite(&all_user) {
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
