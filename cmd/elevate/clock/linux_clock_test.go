package clock

import (
	"errors"
	"testing"
	"time"
)

func TestLinuxClockNowMonoNS_ParsesProcUptime(t *testing.T) {
	c := NewLinuxClock()
	c.readUptime = func() ([]byte, error) {
		return []byte("12.5 99.0\n"), nil
	}

	got := c.NowMonoNS()
	want := int64(12.5 * float64(time.Second))
	if got != want {
		t.Fatalf("unexpected monotonic ns: got %d want %d", got, want)
	}
}

func TestLinuxClockNowMonoNS_FallbackOnReadError(t *testing.T) {
	c := NewLinuxClock()
	c.started = time.Now().Add(-2 * time.Second)
	c.readUptime = func() ([]byte, error) {
		return nil, errors.New("boom")
	}

	got := c.NowMonoNS()
	if got < int64(1*time.Second) || got > int64(10*time.Second) {
		t.Fatalf("unexpected fallback monotonic ns: got %d", got)
	}
}

func TestLinuxClockNowMonoNS_FallbackOnParseError(t *testing.T) {
	c := NewLinuxClock()
	c.started = time.Now().Add(-1500 * time.Millisecond)
	c.readUptime = func() ([]byte, error) {
		return []byte("not-a-number 0"), nil
	}

	got := c.NowMonoNS()
	if got < int64(1*time.Second) || got > int64(10*time.Second) {
		t.Fatalf("unexpected fallback monotonic ns: got %d", got)
	}
}
