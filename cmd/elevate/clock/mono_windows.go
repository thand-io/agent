//go:build windows

package clock

import (
	"syscall"
	"time"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetTickCount64 = kernel32.NewProc("GetTickCount64")
)

// NowMonoNS returns monotonic nanoseconds since system boot.
func (c *Clock) NowMonoNS() int64 {
	ms, _, callErr := procGetTickCount64.Call()
	if callErr == syscall.Errno(0) {
		return int64(ms) * int64(time.Millisecond)
	}
	return time.Since(c.started).Nanoseconds()
}
