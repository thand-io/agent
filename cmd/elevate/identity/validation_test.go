package identity

import (
	"strings"
	"testing"
)

func TestValidAccountName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "simple", value: "alice", want: true},
		{name: "underscore", value: "svc_app", want: true},
		{name: "trailing dollar", value: "machine$", want: true},
		{name: "dot and dash", value: "svc.app-1", want: true},
		{name: "empty", value: "", want: false},
		{name: "space", value: "alice smith", want: false},
		{name: "slash", value: "alice/bob", want: false},
		{name: "backslash", value: `DOMAIN\alice`, want: false},
		{name: "upn", value: "alice@example.com", want: false},
		{name: "too long", value: strings.Repeat("a", MaxAccountNameLength+1), want: false},
	}

	for _, tc := range tests {
		if got := ValidAccountName(tc.value); got != tc.want {
			t.Fatalf("%s: got %t want %t for %q", tc.name, got, tc.want, tc.value)
		}
	}
}

func TestValidWindowsAdminGroup(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "default", value: "Administrators", want: true},
		{name: "with space", value: "Remote Desktop Users", want: true},
		{name: "dash", value: "Ops-Admins", want: true},
		{name: "empty", value: "", want: false},
		{name: "semicolon", value: "Administrators; Remove-Item *", want: false},
		{name: "leading digit", value: "123Admins", want: false},
		{name: "too long", value: strings.Repeat("A", MaxWindowsAdminGroupLength+1), want: false},
	}

	for _, tc := range tests {
		if got := ValidWindowsAdminGroup(tc.value); got != tc.want {
			t.Fatalf("%s: got %t want %t for %q", tc.name, got, tc.want, tc.value)
		}
	}
}
