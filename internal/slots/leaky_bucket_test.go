package slots

import (
	"testing"
	"time"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/spf13/viper"
)

func loadLeakySlot(t *testing.T) Slot {
	v := viper.New()

	v.Set("kind", "leaky_bucket")
	v.Set("bucket_size", 200)
	v.Set("refresh_rate", 100)
	v.Set("users.read", "r")
	v.Set("users.write", "w")
	v.Set("users.allu", "a")

	slot, err := GetSlot(v, nil, "")
	if err != nil {
		t.Fatalf("Slot must not return error: %s", err)
	}
	return slot
}

func TestLeakyBucketSmoke(t *testing.T) {
	slot := loadLeakySlot(t)

	leakySlot := slot.(*leakyBucketSlot)
	if leakySlot.value != 0 {
		t.Errorf("Bucket must be started with 0 value: %d", leakySlot.value)
	}
}

func TestLeakyBucketRead(t *testing.T) {
	read_user, _ := auth.GetUser("read", "pass")
	write_user, _ := auth.GetUser("write", "pass")
	all_user, _ := auth.GetUser("allu", "pass")

	slot := loadLeakySlot(t)
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

func TestLeakyBucketWrite(t *testing.T) {
	read_user, _ := auth.GetUser("read", "pass")
	write_user, _ := auth.GetUser("write", "pass")
	all_user, _ := auth.GetUser("allu", "pass")

	slot := loadLeakySlot(t)
	if slot.CanWrite(&read_user) {
		t.Fatalf("we should not be able to write with the read user")
	}

	if slot.CanWrite(&write_user) {
		t.Fatalf("we should not be able to write with the write user")
	}

	if slot.CanWrite(&all_user) {
		t.Fatalf("we should not be able to write with the read/write user")
	}
}

func TestLeakyBucketMissingConfig(t *testing.T) {
	v := viper.New()

	v.Set("kind", "leaky_bucket")
	_, err := GetSlot(v, nil, "")

	if err == nil {
		t.Fatalf("Slot must return error")
	}
}

func TestLeakyBucketInvalidConfig(t *testing.T) {
	v := viper.New()

	v.Set("kind", "leaky_bucket")
	v.Set("bucket_size", 0)
	_, err := GetSlot(v, nil, "")

	if err == nil {
		t.Fatalf("Slot must return error")
	}
}

func TestLeakyBucketUseAllTokens(t *testing.T) {
	slot := loadLeakySlot(t)

	leakySlot := slot.(*leakyBucketSlot)
	for i := 0; i < 200; i++ {
		if leakySlot.Read() != "1" {
			t.Fatalf("Failed on %d, we should be able to read all tokens", i)
		}
	}

	if leakySlot.Read() != "0" {
		t.Fatalf("We should not be able to read more tokens")
	}

	time.Sleep(100 * time.Millisecond)

	if leakySlot.Read() != "1" {
		t.Fatalf("We should be able to read tokens again")
	}

	if leakySlot.Read() != "0" {
		t.Fatalf("We should not be able to read more tokens")
	}
}
