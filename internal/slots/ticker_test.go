package slots

import (
	"testing"

	"github.com/spf13/viper"

	"github.com/dankomiocevic/ghoti/internal/auth"
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
	readUser, _ := auth.GetUser("read", "pass")
	writeUser, _ := auth.GetUser("write", "pass")
	allUser, _ := auth.GetUser("allu", "pass")

	slot := loadTickerSlot(t)
	if !slot.CanRead(&readUser) {
		t.Fatalf("we should be able to read with the read user")
	}

	if slot.CanRead(&writeUser) {
		t.Fatalf("we should not be able to read with the write user")
	}

	if !slot.CanRead(&allUser) {
		t.Fatalf("we should be able to read with the read/write user")
	}
}

func TestTickerWritePermissions(t *testing.T) {
	readUser, _ := auth.GetUser("read", "pass")
	writeUser, _ := auth.GetUser("write", "pass")
	allUser, _ := auth.GetUser("allu", "pass")

	slot := loadTickerSlot(t)

	if slot.CanWrite(&readUser) {
		t.Fatalf("we should not be able to write with the read user")
	}

	if !slot.CanWrite(&writeUser) {
		t.Fatalf("we should be able to write with the write user")
	}

	if !slot.CanWrite(&allUser) {
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

func TestTickerMissingConfig(t *testing.T) {
	v := viper.New()

	v.Set("kind", "ticker")
	_, err := GetSlot(v, nil, "")

	if err == nil {
		t.Fatalf("Slot must return error")
	}
}

func TestTickerInvalidRefreshRate(t *testing.T) {
	v := viper.New()

	v.Set("kind", "ticker")
	v.Set("initial_value", 200)
	v.Set("refresh_rate", 0) // Invalid refresh rate

	_, err := GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error for invalid refresh rate")
	}
}

func TestTickerNegativeInitialValue(t *testing.T) {
	v := viper.New()

	v.Set("kind", "ticker")
	v.Set("initial_value", -1) // Invalid initial value
	v.Set("refresh_rate", 100)

	_, err := GetSlot(v, nil, "")
	if err == nil {
		t.Fatalf("Slot must return error for negative initial value")
	}
}

func TestTickerValidConfig(t *testing.T) {
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

func TestTickerCanReadWhenUsersEmpty(t *testing.T) {
	v := viper.New()

	v.Set("kind", "ticker")
	v.Set("initial_value", 200)
	v.Set("refresh_rate", 100)

	slot, err := GetSlot(v, nil, "")
	if err != nil {
		t.Fatalf("Slot must not return error for valid config: %s", err)
	}

	readUser, _ := auth.GetUser("read", "pass")
	if !slot.CanRead(&readUser) {
		t.Fatalf("we should be able to read when users map is empty")
	}
}

func TestTickerCanWriteWhenUsersEmpty(t *testing.T) {
	v := viper.New()

	v.Set("kind", "ticker")
	v.Set("initial_value", 200)
	v.Set("refresh_rate", 100)

	slot, err := GetSlot(v, nil, "")
	if err != nil {
		t.Fatalf("Slot must not return error for valid config: %s", err)
	}

	writeUser, _ := auth.GetUser("write", "pass")
	if !slot.CanWrite(&writeUser) {
		t.Fatalf("we should be able to write when users map is empty")
	}
}

func TestTickerWriteNonIntegerData(t *testing.T) {
	slot := loadTickerSlot(t)

	_, err := slot.Write("non-integer", nil)
	if err == nil {
		t.Fatalf("Error must be returned when storing non-integer value")
	}
}

// newHourTicker returns a ticker with a one-hour tick, so the real clock
// does not move to the next tick while a test runs. Elapsed ticks are
// simulated by moving the slot's window back.
func newHourTicker(t *testing.T, initial int) *tickerSlot {
	t.Helper()
	slot, err := newTickerSlot(3_600_000, initial, map[string]string{})
	if err != nil {
		t.Fatalf("ticker must be created: %s", err)
	}
	return slot
}

// sameTick runs f and reports whether the real clock stayed in one tick of
// an hour, so the result does not depend on an accidental real tick.
func sameTick(f func()) bool {
	before := currentWindowMillis(3_600_000)
	f()
	return before == currentWindowMillis(3_600_000)
}

// Reading a ticker repeatedly within one tick must not change its value,
// which is what makes a read safe to retry.
func TestTickerRepeatedReadsDoNotAdvance(t *testing.T) {
	for attempt := 0; attempt < 3; attempt++ {
		slot := newHourTicker(t, 100)
		slot.window -= 5 // five ticks have elapsed since the slot started

		var reads []string
		if !sameTick(func() {
			for i := 0; i < 10; i++ {
				reads = append(reads, slot.Read())
			}
		}) {
			continue
		}

		for i, v := range reads {
			if v != "95" {
				t.Fatalf("read %d returned %s, every read must return 95", i, v)
			}
		}
		return
	}
	t.Fatal("the clock crossed a tick on every attempt")
}

// The value after some elapsed time must be the same whether the slot was
// read once at the end or many times along the way: reads do not consume
// ticks.
func TestTickerValueDependsOnlyOnElapsedTicks(t *testing.T) {
	for attempt := 0; attempt < 3; attempt++ {
		once := newHourTicker(t, 100)
		often := newHourTicker(t, 100)

		var onceValue, oftenValue string
		var intermediate []string
		if !sameTick(func() {
			// once: 7 ticks pass, then a single read.
			once.window -= 7
			onceValue = once.Read()

			// often: the same 7 ticks pass in steps of 2, 3 and 2, with
			// several reads after each step.
			for _, step := range []int64{2, 3, 2} {
				often.window -= step
				for i := 0; i < 3; i++ {
					intermediate = append(intermediate, often.Read())
				}
			}
			oftenValue = often.Read()
		}) {
			continue
		}

		if onceValue != "93" {
			t.Fatalf("a single read after 7 ticks must return 93, got %s", onceValue)
		}
		if oftenValue != onceValue {
			t.Fatalf("many reads must end at the same value as one read: %s != %s", oftenValue, onceValue)
		}
		want := []string{"98", "98", "98", "95", "95", "95", "93", "93", "93"}
		for i := range want {
			if intermediate[i] != want[i] {
				t.Fatalf("intermediate reads = %v, want %v", intermediate, want)
			}
		}
		return
	}
	t.Fatal("the clock crossed a tick on every attempt")
}

// The countdown stops at zero however many reads happen after it expires.
func TestTickerRepeatedReadsStayAtZero(t *testing.T) {
	slot := newHourTicker(t, 3)
	slot.window -= 10
	for i := 0; i < 5; i++ {
		if v := slot.Read(); v != "0" {
			t.Fatalf("read %d returned %s, an expired ticker must return 0", i, v)
		}
	}
}
