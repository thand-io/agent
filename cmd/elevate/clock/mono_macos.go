//go:build darwin

package clock

import (
	"time"

	"golang.org/x/sys/unix"
)

// NowMonoNS returns suspend-inclusive monotonic nanoseconds since boot on macOS.
func (c *Clock) NowMonoNS() int64 {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &ts); err != nil {
		return time.Since(c.started).Nanoseconds()
	}
	return ts.Nano()
}
