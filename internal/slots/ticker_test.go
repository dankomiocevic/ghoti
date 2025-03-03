package slots

import (
	"testing"
	"time"

	"github.com/dankomiocevic/ghoti/internal/auth"
	"github.com/spf13/viper"
)

func loadTickerSlot(t *testing.T) Slot {
	v := viper.New()

	v.Set("kind", "ticker")
	v.Set("initial_value", 200)
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

func TestTickerSmoke(t *testing.T) {
	slot := loadTickerSlot(t)

	tickerSlot := slot.(*tickerSlot)
	if tickerSlot.value != 200 {
		t.Errorf("Ticker must be started with 200 value: %d", tickerSlot.value)
	}
}

func TestTickerRead(t *testing.T) {
	read_user, _ := auth.GetUser("read", "pass")
	write_user, _ := auth.GetUser("write", "pass")
	all_user, _ := auth.GetUser("allu", "pass")

	slot := loadTickerSlot(t)
	if !slot.CanRead(&read_user) {
		t.Fatalf("we should be able to read with the read user")
	}

	if slot.CanRead(&write_user) {
		t.Fatalf("we should not be able to read with the write user")
	}

	if !slot.CanRead(&all_user) {
		t.Fatalf("we should be able to read with the read/write user")
	}
}

func TestTickerWritePermissions(t *testing.T) {
	read_user, _ := auth.GetUser("read", "pass")
	write_user, _ := auth.GetUser("write", "pass")
	all_user, _ := auth.GetUser("allu", "pass")

	slot := loadTickerSlot(t)

	if slot.CanWrite(&read_user) {
		t.Fatalf("we should not be able to write with the read user")
	}

	if !slot.CanWrite(&write_user) {
		t.Fatalf("we should be able to write with the write user")
	}

	if !slot.CanWrite(&all_user) {
		t.Fatalf("we should be able to write with the read/write user")
	}
}

func TestTickerWrite(t *testing.T) {
	slot := loadTickerSlot(t)

	slot.Write("100", nil)
	if slot.Read() != "100" {
		t.Fatalf("Value must be 100")
	}

	slot.Write("0", nil)
	if slot.Read() != "0" {
		t.Fatalf("Value must be 0")
	}

	_, err := slot.Write("-1", nil)
	if err == nil {
		t.Fatalf("Error must be returned when storing negative value")
	}
}

func TestTickerTick(t *testing.T) {
	slot := loadTickerSlot(t)

	slot.Write("100", nil)
	if slot.Read() != "100" {
		t.Fatalf("Value must be 100")
	}

	time.Sleep(101 * time.Millisecond)
	if slot.Read() != "99" {
		t.Fatalf("Value must be 99")
	}
}

func TestTickerMissingConfig(t *testing.T) {
	v := viper.New()

	v.Set("kind", "ticker")
	_, err := GetSlot(v, nil, "")

	if err == nil {
		t.Fatalf("Slot must return error")
	}
}

func TickerInvalidRefreshRate(t *testing.T) {
	v := viper.New()

	v.Set("kind", "ticker")
	v.Set("initial_value", 200)
	v.Set("refresh_rate", 0) // Invalid refresh rate

	_, err := GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error for invalid refresh rate")
	}
}

func TickerNegativeInitialValue(t *testing.T) {
	v := viper.New()

	v.Set("kind", "ticker")
	v.Set("initial_value", -1) // Invalid initial value
	v.Set("refresh_rate", 100)

	_, err := GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error for negative initial value")
	}
}

func TickerValidConfig(t *testing.T) {
	v := viper.New()

	v.Set("kind", "ticker")
	v.Set("initial_value", 200)
	v.Set("refresh_rate", 100)

	slot, err := GetSlot(v, nil, "")
	if err != nil {
		t.Fatalf("Slot must not return error for valid config: %s", err)
	}

	tickerSlot := slot.(*tickerSlot)
	if tickerSlot.value != 200 {
		t.Errorf("Ticker must be started with 200 value: %d", tickerSlot.value)
	}
	if tickerSlot.rate != 100 {
		t.Errorf("Ticker must have refresh rate of 100: %d", tickerSlot.rate)
	}
}

func TickerCanReadWhenUsersEmpty(t *testing.T) {
	v := viper.New()

	v.Set("kind", "ticker")
	v.Set("initial_value", 200)
	v.Set("refresh_rate", 100)

	slot, err := GetSlot(v, nil, "")
	if err != nil {
		t.Fatalf("Slot must not return error for valid config: %s", err)
	}

	read_user, _ := auth.GetUser("read", "pass")
	if !slot.CanRead(&read_user) {
		t.Fatalf("we should be able to read when users map is empty")
	}
}

func TickerCanWriteWhenUsersEmpty(t *testing.T) {
	v := viper.New()

	v.Set("kind", "ticker")
	v.Set("initial_value", 200)
	v.Set("refresh_rate", 100)

	slot, err := GetSlot(v, nil, "")
	if err != nil {
		t.Fatalf("Slot must not return error for valid config: %s", err)
	}

	write_user, _ := auth.GetUser("write", "pass")
	if !slot.CanWrite(&write_user) {
		t.Fatalf("we should be able to write when users map is empty")
	}
}

func TickerWriteNonIntegerData(t *testing.T) {
	slot := loadTickerSlot(t)

	_, err := slot.Write("non-integer", nil)
	if err == nil {
		t.Fatalf("Error must be returned when storing non-integer value")
	}
}
