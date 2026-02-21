package clock

import "time"

type PlaceholderClock struct{}

func NewPlaceholderClock() *PlaceholderClock {
	return &PlaceholderClock{}
}

func (PlaceholderClock) NowMonoNS() int64 {
	// Chunk 1 placeholder only. Raw monotonic source is added in later chunk.
	return time.Now().UnixNano()
}

func (PlaceholderClock) NowWallUTC() time.Time {
	return time.Now().UTC()
}
