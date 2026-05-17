package common

import (
	"crypto/sha256"
	"testing"

	"github.com/google/uuid"
)

func TestGetClientIdentifier_UsesEnvUUID(t *testing.T) {
	expected := uuid.New()
	t.Setenv(clientIdentifierEnvVar, expected.String())

	got := GetClientIdentifier()
	if got != expected {
		t.Fatalf("GetClientIdentifier() = %s, expected %s", got.String(), expected.String())
	}
}

func TestGetClientIdentifier_UsesEnvStringHash(t *testing.T) {
	value := "not-a-uuid-but-still-unique"
	t.Setenv(clientIdentifierEnvVar, value)

	got1 := GetClientIdentifier()
	got2 := GetClientIdentifier()
	if got1 != got2 {
		t.Fatalf("expected deterministic UUID for env override; got %s then %s", got1.String(), got2.String())
	}

	hash := sha256.Sum256([]byte(value))
	expected := uuid.UUID(hash[:16])
	if got1 != expected {
		t.Fatalf("GetClientIdentifier() = %s, expected %s", got1.String(), expected.String())
	}
}
