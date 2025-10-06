package common

import (
	"fmt"
	"strings"
	"time"

	iso8601 "github.com/senseyeio/duration"
)

func ValidateDuration(duration string) (time.Duration, error) {
	w, err := validateDuration(duration)
	if err != nil {
		return 0, err
	}
	if w < 1*time.Minute {
		return 0, fmt.Errorf("duration must be at least 1 minutes")
	}
	return w, nil
}

func validateDuration(duration string) (time.Duration, error) {

	duration = strings.TrimSpace(duration)

	if parsedDuration, err := time.ParseDuration(duration); err == nil {
		return parsedDuration, nil
	} else if isoDuration, err := iso8601.ParseISO8601(duration); err == nil {
		referenceTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		shiftedTime := isoDuration.Shift(referenceTime)
		return shiftedTime.Sub(referenceTime), nil
	}

	return 0, fmt.Errorf("invalid duration format: %s. Expect ISO 8601 or duration string", duration)
}
