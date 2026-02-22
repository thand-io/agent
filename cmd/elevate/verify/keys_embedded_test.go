package verify

import (
	"strings"
	"testing"
)

func TestLoadEmbeddedTrustedKeys(t *testing.T) {
	keys, err := loadEmbeddedTrustedKeys()
	if err != nil {
		t.Fatalf("loadEmbeddedTrustedKeys failed: %v", err)
	}

	if len(keys) == 0 {
		t.Fatal("expected at least one embedded trusted key")
	}

	if _, ok := keys["thand-server-current"]; !ok {
		t.Fatal("expected thand-server-current key id")
	}
	if _, ok := keys["thand-server-next"]; !ok {
		t.Fatal("expected thand-server-next key id")
	}

	for keyID, keyText := range keys {
		if strings.TrimSpace(keyText) == "" {
			t.Fatalf("expected non-empty key text for %s", keyID)
		}
		if !strings.Contains(keyText, "BEGIN PUBLIC KEY") {
			t.Fatalf("expected PEM public key format for %s", keyID)
		}
	}
}
