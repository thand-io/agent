//go:build !thand_dev

package common

import (
	"testing"
)

func TestGetDeviceIDIgnoresDevOverrideInProductionBuild(t *testing.T) {
	t.Setenv("THAND_DEV_DEVICE_ID_OVERRIDE", "11111111-2222-3333-4444-555555555555")

	got := GetDeviceID()
	want := getMachineDerivedDeviceID()

	if got != want {
		t.Fatalf("GetDeviceID() = %q, want machine-derived %q", got, want)
	}
}

func TestGetClientIdentifierMatchesDeviceID(t *testing.T) {
	if got, want := GetClientIdentifier(), GetDeviceID(); got != want {
		t.Fatalf("GetClientIdentifier() = %q, want %q", got, want)
	}
}
