//go:build thand_dev

package common

import "testing"

func TestGetDeviceIDHonorsDevOverride(t *testing.T) {
	t.Setenv(deviceIDOverrideEnvVar, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	got := GetDeviceID().String()
	want := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	if got != want {
		t.Fatalf("GetDeviceID() = %q, want %q", got, want)
	}
}

func TestGetClientIdentifierMatchesDeviceID(t *testing.T) {
	if got, want := GetClientIdentifier(), GetDeviceID(); got != want {
		t.Fatalf("GetClientIdentifier() = %q, want %q", got, want)
	}
}
