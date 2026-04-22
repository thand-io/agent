package thand

import (
	"errors"
	"fmt"
	"strings"

	"go.temporal.io/sdk/temporal"
)

// unwrapTemporalError unwraps ActivityError and extracts the underlying error message.
// It handles the Temporal error chain: ActivityError → ApplicationError → cause
// Returns the extracted error or the original error if no special handling is needed.
func unwrapTemporalError(err error) error {
	if err == nil {
		return nil
	}

	var foundError error

	// First unwrap ActivityError if present, then check the underlying error type
	var activityErr *temporal.ActivityError
	unwrappedErr := err
	if errors.As(err, &activityErr) {
		if innerErr := errors.Unwrap(activityErr); innerErr != nil {
			unwrappedErr = innerErr
		}
	}

	// Handle different Temporal error types
	switch {
	case temporal.IsApplicationError(unwrappedErr):
		var appErr *temporal.ApplicationError
		if errors.As(unwrappedErr, &appErr) {
			foundError = fmt.Errorf("application error: %v", appErr.Message())
		}
	default:
		foundError = err
	}

	return foundError
}

func isTemporalApplicationErrorType(err error, targetType string) bool {
	if err == nil {
		return false
	}

	var activityErr *temporal.ActivityError
	if errors.As(err, &activityErr) {
		if innerErr := errors.Unwrap(activityErr); innerErr != nil {
			err = innerErr
		}
	}

	var appErr *temporal.ApplicationError
	return errors.As(err, &appErr) && appErr.Type() == targetType
}

func isDeviceRouteUnavailableError(err error) bool {
	return isTemporalApplicationErrorType(err, "DeviceRouteUnavailable")
}

func isTemporalTimeoutError(err error) bool {
	var timeoutErr *temporal.TimeoutError
	return errors.As(err, &timeoutErr)
}

func temporalErrorMessage(err error) string {
	if err == nil {
		return ""
	}

	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) {
		return appErr.Message()
	}

	return err.Error()
}

func isTransientBrokerRevokeError(err error) bool {
	message := strings.ToLower(temporalErrorMessage(err))
	if message == "" {
		return false
	}

	transientNeedles := []string{
		"underlying connection interrupted",
		"connection interrupted",
		"connection invalidated",
		"session manually canceled",
	}

	for _, needle := range transientNeedles {
		if strings.Contains(message, needle) {
			return true
		}
	}

	return false
}
