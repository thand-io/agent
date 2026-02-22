package clock

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const procUptimePath = "/proc/uptime"

// LinuxClock reads monotonic uptime from /proc/uptime.
type LinuxClock struct {
	started    time.Time
	readUptime func() ([]byte, error)
}

// NewLinuxClock constructs a Linux clock implementation.
func NewLinuxClock() *LinuxClock {
	return &LinuxClock{
		started:    time.Now(),
		readUptime: func() ([]byte, error) { return os.ReadFile(procUptimePath) },
	}
}

// NowMonoNS returns monotonic nanoseconds since boot when available.
func (c *LinuxClock) NowMonoNS() int64 {
	raw, err := c.readUptime()
	if err != nil {
		// Fallback keeps the process functional if /proc/uptime is unavailable.
		return time.Since(c.started).Nanoseconds()
	}

	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return time.Since(c.started).Nanoseconds()
	}

	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return time.Since(c.started).Nanoseconds()
	}

	return int64(seconds * float64(time.Second))
}

// NowWallUTC returns the current wall time in UTC.
func (*LinuxClock) NowWallUTC() time.Time {
	return time.Now().UTC()
}
