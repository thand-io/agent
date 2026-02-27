//go:build linux

package clock

import (
	"time"

	"golang.org/x/sys/unix"
)

// NowMonoNS returns suspend-inclusive monotonic nanoseconds since boot on Linux.
func (c *Clock) NowMonoNS() int64 {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &ts); err != nil {
		return time.Since(c.started).Nanoseconds()
	}
	return ts.Nano()
}
