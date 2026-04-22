package thand

import (
	"errors"
	"testing"

	"go.temporal.io/sdk/temporal"
)

func TestIsTransientBrokerRevokeError(t *testing.T) {
	err := temporal.NewApplicationError(
		"broker control command failed: exit status 1: thand-macos-privilege-brokerctl failed: Peer forbidden (code signing)",
		"RevocationError",
	)

	if isTransientBrokerRevokeError(err) {
		t.Fatal("expected broker code-signing failure to be treated as non-transient")
	}
}

func TestIsTransientBrokerRevokeErrorRecognizesConnectionInterruption(t *testing.T) {
	err := temporal.NewApplicationError(
		"broker control command failed: exit status 1: Underlying connection interrupted",
		"RevocationError",
	)

	if !isTransientBrokerRevokeError(err) {
		t.Fatal("expected broker connection interruption to be treated as transient")
	}
}

func TestIsTransientBrokerRevokeErrorRejectsPermanentErrors(t *testing.T) {
	err := errors.New("local username root is denied for privileged access")

	if isTransientBrokerRevokeError(err) {
		t.Fatal("expected permanent validation error to not be treated as transient")
	}
}
