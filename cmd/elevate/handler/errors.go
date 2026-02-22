package handler

import "fmt"

// ErrorCode is a stable, client-facing error category.
type ErrorCode string

const (
	ErrorCodeInvalidRequest ErrorCode = "invalid_request"
	ErrorCodeUnauthorized   ErrorCode = "unauthorized"
	ErrorCodeInternal       ErrorCode = "internal_error"
)

type responseError struct {
	Code  ErrorCode
	Cause error
}

func (e *responseError) Error() string {
	return string(e.Code)
}

func (e *responseError) Unwrap() error {
	return e.Cause
}

func invalidRequestErr(err error) *responseError {
	return &responseError{Code: ErrorCodeInvalidRequest, Cause: err}
}

func unauthorizedErr(err error) *responseError {
	return &responseError{Code: ErrorCodeUnauthorized, Cause: err}
}

func internalErr(err error) *responseError {
	return &responseError{Code: ErrorCodeInternal, Cause: err}
}

func wrapInternal(msg string, err error) *responseError {
	return internalErr(fmt.Errorf("%s: %w", msg, err))
}
