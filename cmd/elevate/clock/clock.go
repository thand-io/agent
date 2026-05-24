package clock

import (
	"time"
)

// Clock provides wall-clock and monotonic time for elevate.
type Clock struct {
	started time.Time
}

// NewClock constructs a clock instance.
func NewClock() *Clock {
	return &Clock{
		started: time.Now(),
	}
}

// NowWallUTC returns the current wall time in UTC.
func (*Clock) NowWallUTC() time.Time {
	return time.Now().UTC()
}
