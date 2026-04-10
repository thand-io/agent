package oauth2

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/thand-io/agent/internal/models"
)

func TestCreateSessionOmitsOAuthTokens(t *testing.T) {
	idToken := createTestIDToken(t, map[string]any{
		"sub":            "user-123",
		"email":          "user@example.com",
		"name":           "Example User",
		"email_verified": true,
	})
	var mu sync.Mutex
	var unexpectedTokenPath string

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			mu.Lock()
			unexpectedTokenPath = r.URL.Path
			mu.Unlock()
			http.Error(w, "unexpected token request path", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "very-large-access-token",
			"refresh_token": "very-large-refresh-token",
			"id_token":      idToken,
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	cfg := models.BasicConfig{
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"auth_url":      tokenServer.URL + "/auth",
		"token_url":     tokenServer.URL + "/token",
		"scopes":        []string{"openid", "profile", "email"},
	}

	provider := &oauth2Provider{}
	err := provider.Initialize("oauth2-test", models.ProviderConfig{
		Name:     "OAuth2 Test",
		Provider: Oauth2ProviderName,
		Enabled:  true,
		Config:   &cfg,
	})
	if err != nil {
		t.Fatalf("initialize provider: %v", err)
	}

	session, err := provider.CreateSession(context.Background(), &models.AuthorizeUser{
		Code:        "test-auth-code",
		RedirectUri: "http://localhost/callback",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	mu.Lock()
	gotUnexpectedTokenPath := unexpectedTokenPath
	mu.Unlock()
	if gotUnexpectedTokenPath != "" {
		t.Fatalf("unexpected token request path: %s", gotUnexpectedTokenPath)
	}

	if session == nil {
		t.Fatal("expected session")
	}

	if session.User == nil {
		t.Fatal("expected user")
	}

	if session.User.Email != "user@example.com" {
		t.Fatalf("expected user email to be preserved, got %q", session.User.Email)
	}

	if session.Expiry.IsZero() {
		t.Fatal("expected expiry to be set")
	}

	if session.Token != "" {
		t.Fatalf("expected id token to be omitted, got %q", session.Token)
	}

	if session.AccessToken != "" {
		t.Fatalf("expected access token to be omitted, got %q", session.AccessToken)
	}

	if session.RefreshToken != "" {
		t.Fatalf("expected refresh token to be omitted, got %q", session.RefreshToken)
	}
}

func createTestIDToken(t *testing.T, claims map[string]any) string {
	t.Helper()

	headerBytes, err := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}

	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}

	return base64.RawURLEncoding.EncodeToString(headerBytes) +
		"." + base64.RawURLEncoding.EncodeToString(payloadBytes) +
		"."
}
