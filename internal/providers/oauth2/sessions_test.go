package oauth2

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thand-io/agent/internal/models"
	"golang.org/x/oauth2"
)

func TestConfigSchemaValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		schema  ConfigSchema
		wantErr bool
	}{
		{
			name: "valid with authority only",
			schema: ConfigSchema{
				ClientID:     "client-id",
				ClientSecret: "client-secret",
				Authority:    "https://issuer.example.com",
			},
		},
		{
			name: "valid with explicit endpoints only",
			schema: ConfigSchema{
				ClientID:     "client-id",
				ClientSecret: "client-secret",
				AuthURL:      "https://issuer.example.com/auth",
				TokenURL:     "https://issuer.example.com/token",
			},
		},
		{
			name: "invalid with neither authority nor explicit endpoints",
			schema: ConfigSchema{
				ClientID:     "client-id",
				ClientSecret: "client-secret",
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.schema.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validate: %v", err)
			}
		})
	}
}

func TestNormalizeDiscoveryURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		authority string
		want      string
	}{
		{
			name:      "issuer base URL gets well-known suffix",
			authority: "https://issuer.example.com/realm/test",
			want:      "https://issuer.example.com/realm/test/.well-known/openid-configuration",
		},
		{
			name:      "existing well-known URL is preserved",
			authority: "https://issuer.example.com/.well-known/openid-configuration",
			want:      "https://issuer.example.com/.well-known/openid-configuration",
		},
		{
			name:      "query and fragment are preserved on issuer URL",
			authority: "https://issuer.example.com/realm/test?foo=bar#frag",
			want:      "https://issuer.example.com/realm/test/.well-known/openid-configuration?foo=bar#frag",
		},
		{
			name:      "query and fragment are preserved on discovery URL",
			authority: "https://issuer.example.com/.well-known/openid-configuration/?foo=bar#frag",
			want:      "https://issuer.example.com/.well-known/openid-configuration?foo=bar#frag",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeDiscoveryURL(tc.authority); got != tc.want {
				t.Fatalf("normalizeDiscoveryURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolvedConfigUsesDiscoveryAndExplicitOverrides(t *testing.T) {
	var mu sync.Mutex
	discoveryHits := 0

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case oidcDiscoveryPath:
			mu.Lock()
			discoveryHits++
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authorization_endpoint": server.URL + "/discovered/auth",
				"token_endpoint":         server.URL + "/discovered/token",
				"userinfo_endpoint":      server.URL + "/discovered/userinfo",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"authority":     server.URL,
		"auth_url":      server.URL + "/explicit/auth",
		"token_url":     server.URL + "/explicit/token",
		"userinfo_url":  server.URL + "/explicit/userinfo",
	})

	resolved, err := provider.getResolvedConfig(context.Background())
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	if resolved.AuthURL != server.URL+"/explicit/auth" {
		t.Fatalf("expected explicit auth_url override, got %q", resolved.AuthURL)
	}
	if resolved.TokenURL != server.URL+"/explicit/token" {
		t.Fatalf("expected explicit token_url override, got %q", resolved.TokenURL)
	}
	if resolved.UserInfoURL != server.URL+"/explicit/userinfo" {
		t.Fatalf("expected explicit userinfo_url override, got %q", resolved.UserInfoURL)
	}

	_, err = provider.getResolvedConfig(context.Background())
	if err != nil {
		t.Fatalf("resolve cached config: %v", err)
	}

	mu.Lock()
	gotDiscoveryHits := discoveryHits
	mu.Unlock()
	if gotDiscoveryHits != 0 {
		t.Fatalf("expected explicit endpoints to skip discovery, got %d discovery fetches", gotDiscoveryHits)
	}
}

func TestResolvedConfigUsesDiscoveryForMissingOverrides(t *testing.T) {
	var mu sync.Mutex
	discoveryHits := 0

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case oidcDiscoveryPath:
			mu.Lock()
			discoveryHits++
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authorization_endpoint": server.URL + "/discovered/auth",
				"token_endpoint":         server.URL + "/discovered/token",
				"userinfo_endpoint":      server.URL + "/discovered/userinfo",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"authority":     server.URL,
		"token_url":     server.URL + "/explicit/token",
	})

	resolved, err := provider.getResolvedConfig(context.Background())
	if err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	if resolved.AuthURL != server.URL+"/discovered/auth" {
		t.Fatalf("expected discovered auth_url, got %q", resolved.AuthURL)
	}
	if resolved.TokenURL != server.URL+"/explicit/token" {
		t.Fatalf("expected explicit token_url override, got %q", resolved.TokenURL)
	}
	if resolved.UserInfoURL != server.URL+"/discovered/userinfo" {
		t.Fatalf("expected discovered userinfo_url, got %q", resolved.UserInfoURL)
	}

	mu.Lock()
	gotDiscoveryHits := discoveryHits
	mu.Unlock()
	if gotDiscoveryHits != 1 {
		t.Fatalf("expected discovery to be fetched once for missing overrides, got %d", gotDiscoveryHits)
	}
}

func TestResolvedConfigSerializesConcurrentDiscovery(t *testing.T) {
	var mu sync.Mutex
	discoveryHits := 0
	firstRequest := make(chan struct{}, 1)
	releaseDiscovery := make(chan struct{})

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != oidcDiscoveryPath {
			http.NotFound(w, r)
			return
		}

		mu.Lock()
		discoveryHits++
		currentHits := discoveryHits
		mu.Unlock()

		if currentHits == 1 {
			firstRequest <- struct{}{}
		}

		<-releaseDiscovery

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"authorization_endpoint": server.URL + "/discovered/auth",
			"token_endpoint":         server.URL + "/discovered/token",
			"userinfo_endpoint":      server.URL + "/discovered/userinfo",
		})
	}))
	defer server.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"authority":     server.URL,
	})

	const callers = 16
	errs := make(chan error, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup

	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := provider.getResolvedConfig(context.Background())
			errs <- err
		}()
	}

	close(start)
	<-firstRequest
	time.Sleep(100 * time.Millisecond)
	close(releaseDiscovery)

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("resolve config: %v", err)
		}
	}

	mu.Lock()
	gotDiscoveryHits := discoveryHits
	mu.Unlock()
	if gotDiscoveryHits != 1 {
		t.Fatalf("expected a single discovery fetch across concurrent callers, got %d", gotDiscoveryHits)
	}
}

func TestCreateSessionUsesIDTokenClaimsAndOmitsOAuthTokens(t *testing.T) {
	idToken := createTestIDToken(t, map[string]any{
		"sub":            "user-123",
		"email":          "user@example.com",
		"name":           "Example User",
		"email_verified": true,
	})

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")

			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "very-large-access-token",
				"refresh_token": "very-large-refresh-token",
				"id_token":      idToken,
				"token_type":    "Bearer",
				"expires_in":    3600,
			})
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":   "user-123",
				"email": "userinfo@example.com",
				"name":  "Userinfo User",
			})
		default:
			http.Error(w, "unexpected token request path", http.StatusBadRequest)
		}
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

	if session == nil {
		t.Fatal("expected session")
	}

	if session.User == nil {
		t.Fatal("expected user")
	}

	if session.User.Email != "user@example.com" {
		t.Fatalf("expected user email to be preserved, got %q", session.User.Email)
	}
	if session.User.Username != "" {
		t.Fatalf("expected username to be empty when no username claims are present, got %q", session.User.Username)
	}

	if session.Expiry.IsZero() {
		t.Fatal("expected expiry to be set")
	}

	if session.Token == "" {
		t.Fatal("expected id token to be preserved")
	}

	if session.AccessToken == "" {
		t.Fatal("expected access token to be preserved")
	}

	if session.RefreshToken == "" {
		t.Fatal("expected refresh token to be preserved")
	}
}

func TestAuthorizeSessionUsesDiscoveredAuthURL(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != oidcDiscoveryPath {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"authorization_endpoint": server.URL + "/authorize",
			"token_endpoint":         server.URL + "/token",
			"userinfo_endpoint":      server.URL + "/userinfo",
		})
	}))
	defer server.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"authority":     server.URL,
		"redirect_url":  "http://localhost/default-callback",
		"scopes":        []string{"profile", "email"},
	})

	response, err := provider.AuthorizeSession(context.Background(), &models.AuthorizeUser{
		State: "state-123",
	})
	if err != nil {
		t.Fatalf("authorize session: %v", err)
	}

	authURL, err := url.Parse(response.Url)
	if err != nil {
		t.Fatalf("parse authorize URL: %v", err)
	}
	if authURL.Path != "/authorize" {
		t.Fatalf("expected discovered authorize path, got %q", authURL.Path)
	}

	query := authURL.Query()
	if query.Get("client_id") != "test-client-id" {
		t.Fatalf("expected client_id query param, got %q", query.Get("client_id"))
	}
	if query.Get("redirect_uri") != "http://localhost/default-callback" {
		t.Fatalf("expected redirect_uri query param, got %q", query.Get("redirect_uri"))
	}
	if query.Get("state") != "state-123" {
		t.Fatalf("expected state query param, got %q", query.Get("state"))
	}
	if got := query.Get("scope"); got != "openid profile email" {
		t.Fatalf("expected openid-prefixed scopes, got %q", got)
	}
}

func TestCreateSessionUsesDiscoveredTokenAndUserInfo(t *testing.T) {
	var mu sync.Mutex
	requestPaths := make([]string, 0, 3)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestPaths = append(requestPaths, r.URL.Path)
		mu.Unlock()

		switch r.URL.Path {
		case oidcDiscoveryPath:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authorization_endpoint": server.URL + "/authorize",
				"token_endpoint":         server.URL + "/token",
				"userinfo_endpoint":      server.URL + "/userinfo",
			})
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":   "user-456",
				"email": "oidc@example.com",
				"name":  "OIDC User",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"authority":     server.URL,
		"redirect_url":  "http://localhost/callback",
	})

	session, err := provider.CreateSession(context.Background(), &models.AuthorizeUser{
		Code: "auth-code",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if session == nil || session.User == nil {
		t.Fatal("expected session user")
	}
	if session.User.Email != "oidc@example.com" {
		t.Fatalf("expected userinfo email, got %q", session.User.Email)
	}
	if session.AccessToken == "" {
		t.Fatal("expected access token to be preserved in stored session")
	}
	if session.Token != "" {
		t.Fatalf("expected id token to be empty when token response does not include one, got %q", session.Token)
	}
	if session.RefreshToken != "" {
		t.Fatalf("expected refresh token to be empty when token response does not include one, got %q", session.RefreshToken)
	}
	mu.Lock()
	gotPaths := append([]string(nil), requestPaths...)
	mu.Unlock()
	if len(gotPaths) != 3 {
		t.Fatalf("expected discovery, token, and userinfo requests, got %v", gotPaths)
	}
	if gotPaths[0] != oidcDiscoveryPath || gotPaths[1] != "/token" || gotPaths[2] != "/userinfo" {
		t.Fatalf("unexpected request order: %v", gotPaths)
	}
}

func TestCreateSessionFallsBackToDerivedUserInfoURL(t *testing.T) {
	var mu sync.Mutex
	requestPaths := make([]string, 0, 3)

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestPaths = append(requestPaths, r.URL.Path)
		mu.Unlock()

		switch r.URL.Path {
		case oidcDiscoveryPath:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authorization_endpoint": server.URL + "/protocol/openid-connect/auth",
				"token_endpoint":         server.URL + "/protocol/openid-connect/token",
			})
		case "/protocol/openid-connect/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/protocol/openid-connect/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":   "user-789",
				"email": "fallback@example.com",
				"name":  "Fallback User",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"authority":     server.URL,
		"redirect_url":  "http://localhost/callback",
	})

	session, err := provider.CreateSession(context.Background(), &models.AuthorizeUser{
		Code: "auth-code",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if session == nil || session.User == nil {
		t.Fatal("expected session user")
	}
	if session.User.Email != "fallback@example.com" {
		t.Fatalf("expected fallback userinfo email, got %q", session.User.Email)
	}

	mu.Lock()
	gotPaths := append([]string(nil), requestPaths...)
	mu.Unlock()
	if len(gotPaths) != 3 {
		t.Fatalf("expected discovery, token, and derived userinfo requests, got %v", gotPaths)
	}
	if gotPaths[2] != "/protocol/openid-connect/userinfo" {
		t.Fatalf("expected derived userinfo path, got %q", gotPaths[2])
	}
}

func TestCreateSessionUsesPreferredUsernameByDefault(t *testing.T) {
	idToken := createTestIDToken(t, map[string]any{
		"sub":                "user-123",
		"email":              "user@example.com",
		"name":               "Example User",
		"preferred_username": "example-user",
		"email_verified":     true,
	})

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.Error(w, "unexpected token request path", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-token",
			"id_token":     idToken,
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"auth_url":      tokenServer.URL + "/auth",
		"token_url":     tokenServer.URL + "/token",
	})

	session, err := provider.CreateSession(context.Background(), &models.AuthorizeUser{
		Code:        "test-auth-code",
		RedirectUri: "http://localhost/callback",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if session.User.Username != "example-user" {
		t.Fatalf("expected preferred_username to populate username, got %q", session.User.Username)
	}
}

func TestCreateSessionUsesUsernameFallbackByDefault(t *testing.T) {
	idToken := createTestIDToken(t, map[string]any{
		"sub":            "user-123",
		"email":          "user@example.com",
		"name":           "Example User",
		"username":       "fallback-user",
		"email_verified": true,
	})

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.Error(w, "unexpected token request path", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-token",
			"id_token":     idToken,
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"auth_url":      tokenServer.URL + "/auth",
		"token_url":     tokenServer.URL + "/token",
	})

	session, err := provider.CreateSession(context.Background(), &models.AuthorizeUser{
		Code:        "test-auth-code",
		RedirectUri: "http://localhost/callback",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if session.User.Username != "fallback-user" {
		t.Fatalf("expected username fallback claim to populate username, got %q", session.User.Username)
	}
}

func TestCreateSessionUsesConfiguredUsernameClaim(t *testing.T) {
	idToken := createTestIDToken(t, map[string]any{
		"sub":                "user-123",
		"email":              "user@example.com",
		"name":               "Example User",
		"preferred_username": "preferred-user",
		"custom_username":    "custom-user",
		"email_verified":     true,
	})

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.Error(w, "unexpected token request path", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-token",
			"id_token":     idToken,
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":      "test-client-id",
		"client_secret":  "test-client-secret",
		"auth_url":       tokenServer.URL + "/auth",
		"token_url":      tokenServer.URL + "/token",
		"username_claim": "custom_username",
	})

	session, err := provider.CreateSession(context.Background(), &models.AuthorizeUser{
		Code:        "test-auth-code",
		RedirectUri: "http://localhost/callback",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if session.User.Username != "custom-user" {
		t.Fatalf("expected configured username claim to populate username, got %q", session.User.Username)
	}
}

func TestCreateSessionFallsBackToUserInfoWhenIDTokenLacksUsername(t *testing.T) {
	idToken := createTestIDToken(t, map[string]any{
		"sub":            "user-123",
		"email":          "user@example.com",
		"name":           "Example User",
		"email_verified": true,
	})

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case oidcDiscoveryPath:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authorization_endpoint": server.URL + "/authorize",
				"token_endpoint":         server.URL + "/token",
				"userinfo_endpoint":      server.URL + "/userinfo",
			})
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-token",
				"id_token":     idToken,
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":                "user-123",
				"email":              "userinfo@example.com",
				"name":               "Userinfo User",
				"preferred_username": "userinfo-username",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"authority":     server.URL,
		"redirect_url":  "http://localhost/callback",
	})

	session, err := provider.CreateSession(context.Background(), &models.AuthorizeUser{Code: "auth-code"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if session.User.Username != "userinfo-username" {
		t.Fatalf("expected userinfo username fallback, got %q", session.User.Username)
	}
	if session.User.Email != "user@example.com" {
		t.Fatalf("expected ID token email to remain authoritative, got %q", session.User.Email)
	}
	if session.User.Name != "Example User" {
		t.Fatalf("expected ID token name to remain authoritative, got %q", session.User.Name)
	}
}

func TestCreateSessionFallsBackToConfiguredUsernameClaimFromUserInfoWhenIDTokenLacksUsername(t *testing.T) {
	idToken := createTestIDToken(t, map[string]any{
		"sub":            "user-123",
		"email":          "user@example.com",
		"name":           "Example User",
		"email_verified": true,
	})

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case oidcDiscoveryPath:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authorization_endpoint": server.URL + "/authorize",
				"token_endpoint":         server.URL + "/token",
				"userinfo_endpoint":      server.URL + "/userinfo",
			})
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-token",
				"id_token":     idToken,
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":             "user-123",
				"email":           "userinfo@example.com",
				"name":            "Userinfo User",
				"custom_username": "custom-userinfo",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":      "test-client-id",
		"client_secret":  "test-client-secret",
		"authority":      server.URL,
		"redirect_url":   "http://localhost/callback",
		"username_claim": "custom_username",
	})

	session, err := provider.CreateSession(context.Background(), &models.AuthorizeUser{Code: "auth-code"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if session.User.Username != "custom-userinfo" {
		t.Fatalf("expected configured userinfo username fallback, got %q", session.User.Username)
	}
}

func TestCreateSessionUsesUsernameFromUserInfoFallback(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case oidcDiscoveryPath:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authorization_endpoint": server.URL + "/authorize",
				"token_endpoint":         server.URL + "/token",
				"userinfo_endpoint":      server.URL + "/userinfo",
			})
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":                "user-456",
				"email":              "oidc@example.com",
				"name":               "OIDC User",
				"preferred_username": "oidc-user",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"authority":     server.URL,
		"redirect_url":  "http://localhost/callback",
	})

	session, err := provider.CreateSession(context.Background(), &models.AuthorizeUser{
		Code: "auth-code",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if session.User.Username != "oidc-user" {
		t.Fatalf("expected userinfo username claim to populate username, got %q", session.User.Username)
	}
}

func TestCreateSessionUsesConfiguredUsernameClaimFromUserInfoFallback(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case oidcDiscoveryPath:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"authorization_endpoint": server.URL + "/authorize",
				"token_endpoint":         server.URL + "/token",
				"userinfo_endpoint":      server.URL + "/userinfo",
			})
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":             "user-456",
				"email":           "oidc@example.com",
				"name":            "OIDC User",
				"custom_username": "custom-oidc-user",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":      "test-client-id",
		"client_secret":  "test-client-secret",
		"authority":      server.URL,
		"redirect_url":   "http://localhost/callback",
		"username_claim": "custom_username",
	})

	session, err := provider.CreateSession(context.Background(), &models.AuthorizeUser{
		Code: "auth-code",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if session.User.Username != "custom-oidc-user" {
		t.Fatalf("expected configured userinfo username claim to populate username, got %q", session.User.Username)
	}
}

func TestCreateSessionIgnoresBlankConfiguredUsernameClaim(t *testing.T) {
	idToken := createTestIDToken(t, map[string]any{
		"sub":            "user-123",
		"email":          "user@example.com",
		"name":           "Example User",
		"custom_claim":   "   ",
		"email_verified": true,
	})

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.Error(w, "unexpected token request path", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-token",
			"id_token":     idToken,
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":      "test-client-id",
		"client_secret":  "test-client-secret",
		"auth_url":       tokenServer.URL + "/auth",
		"token_url":      tokenServer.URL + "/token",
		"username_claim": "custom_claim",
	})

	session, err := provider.CreateSession(context.Background(), &models.AuthorizeUser{
		Code:        "test-auth-code",
		RedirectUri: "http://localhost/callback",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if session.User.Username != "" {
		t.Fatalf("expected blank configured username claim to be ignored, got %q", session.User.Username)
	}
}

func TestCreateSessionIgnoresNonStringConfiguredUsernameClaim(t *testing.T) {
	idToken := createTestIDToken(t, map[string]any{
		"sub":            "user-123",
		"email":          "user@example.com",
		"name":           "Example User",
		"custom_claim":   42,
		"email_verified": true,
	})

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.Error(w, "unexpected token request path", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-token",
			"id_token":     idToken,
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":      "test-client-id",
		"client_secret":  "test-client-secret",
		"auth_url":       tokenServer.URL + "/auth",
		"token_url":      tokenServer.URL + "/token",
		"username_claim": "custom_claim",
	})

	session, err := provider.CreateSession(context.Background(), &models.AuthorizeUser{
		Code:        "test-auth-code",
		RedirectUri: "http://localhost/callback",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if session.User.Username != "" {
		t.Fatalf("expected non-string configured username claim to be ignored, got %q", session.User.Username)
	}
}

func TestDeriveUserInfoURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		tokenURL string
		want     string
	}{
		{
			name:     "rewrites terminal token segment",
			tokenURL: "https://issuer.example.com/protocol/openid-connect/token",
			want:     "https://issuer.example.com/protocol/openid-connect/userinfo",
		},
		{
			name:     "preserves query fragment and trailing slash",
			tokenURL: "https://issuer.example.com/token/?a=1#fragment",
			want:     "https://issuer.example.com/userinfo/?a=1#fragment",
		},
		{
			name:     "returns empty for non-token terminal segment",
			tokenURL: "https://issuer.example.com/oauth2/access-token",
			want:     "",
		},
		{
			name:     "returns empty for tokenize path",
			tokenURL: "https://issuer.example.com/oauth2/tokenize",
			want:     "",
		},
		{
			name:     "returns empty for missing path",
			tokenURL: "https://issuer.example.com",
			want:     "",
		},
		{
			name:     "returns empty for invalid URL",
			tokenURL: "://bad-url",
			want:     "",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := deriveUserInfoURL(tc.tokenURL); got != tc.want {
				t.Fatalf("deriveUserInfoURL(%q) = %q, want %q", tc.tokenURL, got, tc.want)
			}
		})
	}
}

func TestCreateSessionFailsWhenDerivedUserInfoURLIsInvalid(t *testing.T) {
	var mu sync.Mutex
	userInfoHits := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/access-token":
			if r.Method == http.MethodPost {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token": "access-token",
					"token_type":   "Bearer",
					"expires_in":   3600,
				})
				return
			}

			mu.Lock()
			userInfoHits++
			mu.Unlock()

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sub":   "unexpected-user",
				"email": "wrong@example.com",
				"name":  "Wrong User",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"auth_url":      server.URL + "/oauth2/auth",
		"token_url":     server.URL + "/oauth2/access-token",
		"redirect_url":  "http://localhost/callback",
	})

	_, err := provider.CreateSession(context.Background(), &models.AuthorizeUser{
		Code: "auth-code",
	})
	if err == nil {
		t.Fatal("expected create session to fail when userinfo URL cannot be derived")
	}
	if !strings.Contains(err.Error(), "no userinfo endpoint configured") {
		t.Fatalf("expected no userinfo endpoint error, got %v", err)
	}

	mu.Lock()
	gotUserInfoHits := userInfoHits
	mu.Unlock()
	if gotUserInfoHits != 0 {
		t.Fatalf("expected invalid fallback derivation to avoid userinfo request, got %d calls", gotUserInfoHits)
	}
}

func TestCreateSessionRejectsUserInfoWithoutSubject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "access-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"email": "missing-sub@example.com",
				"name":  "Missing Subject",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := newTestOAuth2Provider(t, models.BasicConfig{
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"auth_url":      server.URL + "/auth",
		"token_url":     server.URL + "/token",
		"userinfo_url":  server.URL + "/userinfo",
		"redirect_url":  "http://localhost/callback",
	})

	_, err := provider.CreateSession(context.Background(), &models.AuthorizeUser{
		Code: "auth-code",
	})
	if err == nil {
		t.Fatal("expected create session to fail when userinfo response is missing sub")
	}
	if !strings.Contains(err.Error(), "sub") {
		t.Fatalf("expected missing sub error, got %v", err)
	}
}

func TestGetUserInfoFromIDTokenRejectsMissingSubject(t *testing.T) {
	token := &oauth2.Token{}
	token = token.WithExtra(map[string]any{
		"id_token": createTestIDToken(t, map[string]any{
			"email": "missing-sub@example.com",
			"name":  "Missing Subject",
		}),
	})

	_, err := getUserInfoFromIDToken(token, "")
	if err == nil {
		t.Fatal("expected getUserInfoFromIDToken to fail when id_token is missing sub")
	}
	if !strings.Contains(err.Error(), "id_token missing sub") {
		t.Fatalf("expected id_token missing sub error, got %v", err)
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

func newTestOAuth2Provider(t *testing.T, cfg models.BasicConfig) *oauth2Provider {
	t.Helper()

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

	return provider
}
