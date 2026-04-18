package daemon

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/models"
)

func boolPtr(v bool) *bool {
	return &v
}

func TestMergeSessionUserWithIdentity_PreservesFreshSessionFields(t *testing.T) {
	sessionUser := &models.User{
		ID:       "user-123",
		Email:    "user@example.com",
		Username: "fresh-user",
		Name:     "Fresh User",
		Verified: boolPtr(true),
		Source:   "oauth2",
		Groups:   []string{"session-admins"},
	}

	compositeIdentity := &models.Identity{
		ID:    "user@example.com",
		Label: "Stale User",
		User: &models.User{
			ID:       "stale-id",
			Email:    "user@example.com",
			Username: "",
			Name:     "Stale Name",
			Verified: boolPtr(false),
			Source:   "cached-provider",
			Groups:   []string{"session-admins", "identity-auditors"},
		},
	}

	merged := mergeSessionUserWithIdentity(sessionUser, compositeIdentity)

	assert.Equal(t, "user-123", merged.ID)
	assert.Equal(t, "user@example.com", merged.Email)
	assert.Equal(t, "fresh-user", merged.Username)
	assert.Equal(t, "Fresh User", merged.Name)
	assert.Equal(t, "oauth2", merged.Source)
	if assert.NotNil(t, merged.Verified) {
		assert.True(t, *merged.Verified)
	}
	assert.ElementsMatch(t, []string{"session-admins", "identity-auditors"}, merged.Groups)
}

func TestMergeSessionUserWithIdentity_FillsMissingSessionFields(t *testing.T) {
	sessionUser := &models.User{
		ID:     "user-123",
		Email:  "user@example.com",
		Name:   "Fresh User",
		Source: "oauth2",
	}

	compositeIdentity := &models.Identity{
		ID:    "user@example.com",
		Label: "Cached User",
		User: &models.User{
			ID:       "user-123",
			Email:    "user@example.com",
			Username: "cached-user",
			Name:     "Cached User",
			Verified: boolPtr(true),
			Source:   "cached-provider",
			Groups:   []string{"identity-auditors"},
		},
	}

	merged := mergeSessionUserWithIdentity(sessionUser, compositeIdentity)

	assert.Equal(t, "cached-user", merged.Username)
	if assert.NotNil(t, merged.Verified) {
		assert.True(t, *merged.Verified)
	}
	assert.Equal(t, "oauth2", merged.Source)
	assert.ElementsMatch(t, []string{"identity-auditors"}, merged.Groups)
}

func TestMergeSessionUserWithIdentity_LeavesSessionUnchangedWithoutCompositeUser(t *testing.T) {
	sessionUser := &models.User{
		ID:       "user-123",
		Email:    "user@example.com",
		Username: "fresh-user",
		Name:     "Fresh User",
		Source:   "oauth2",
	}

	merged := mergeSessionUserWithIdentity(sessionUser, &models.Identity{ID: "user@example.com"})

	assert.Equal(t, sessionUser, merged)
}

func TestMatchOrigin(t *testing.T) {
	tests := []struct {
		name           string
		origin         string
		pattern        string
		expectedResult bool
	}{
		{
			name:           "exact match",
			origin:         "https://app.thand.io",
			pattern:        "https://app.thand.io",
			expectedResult: true,
		},
		{
			name:           "wildcard subdomain match",
			origin:         "https://foo.app.thand.io",
			pattern:        "https://*.app.thand.io",
			expectedResult: true,
		},
		{
			name:           "wildcard subdomain match with different subdomain",
			origin:         "https://bar.app.thand.io",
			pattern:        "https://*.app.thand.io",
			expectedResult: true,
		},
		{
			name:           "wildcard subdomain match with numbers",
			origin:         "https://test123.app.thand.io",
			pattern:        "https://*.app.thand.io",
			expectedResult: true,
		},
		{
			name:           "wildcard subdomain match with hyphens",
			origin:         "https://my-test-app.app.thand.io",
			pattern:        "https://*.app.thand.io",
			expectedResult: true,
		},
		{
			name:           "no match - different domain",
			origin:         "https://foo.evil.com",
			pattern:        "https://*.app.thand.io",
			expectedResult: false,
		},
		{
			name:           "no match - different suffix",
			origin:         "https://foo.app.thand.com",
			pattern:        "https://*.app.thand.io",
			expectedResult: false,
		},
		{
			name:           "no match - missing subdomain",
			origin:         "https://app.thand.io",
			pattern:        "https://*.app.thand.io",
			expectedResult: false,
		},
		{
			name:           "wildcard with port",
			origin:         "https://foo.app.thand.io:8443",
			pattern:        "https://*.app.thand.io:8443",
			expectedResult: true,
		},
		{
			name:           "http scheme wildcard",
			origin:         "http://foo.app.thand.io",
			pattern:        "http://*.app.thand.io",
			expectedResult: true,
		},
		{
			name:           "scheme mismatch",
			origin:         "http://foo.app.thand.io",
			pattern:        "https://*.app.thand.io",
			expectedResult: false,
		},
		{
			name:           "empty origin",
			origin:         "",
			pattern:        "https://*.app.thand.io",
			expectedResult: false,
		},
		{
			name:           "nested subdomain - allowed by current implementation",
			origin:         "https://sub.foo.app.thand.io",
			pattern:        "https://*.app.thand.io",
			expectedResult: true,
		},
		{
			name:           "allow all wildcard",
			origin:         "https://any.domain.com",
			pattern:        "*",
			expectedResult: true,
		},
		{
			name:           "invalid pattern - missing dot after wildcard",
			origin:         "https://evilexample.com",
			pattern:        "https://*example.com",
			expectedResult: false,
		},
		{
			name:           "invalid pattern - missing dot should not match legitimate subdomain",
			origin:         "https://sub.example.com",
			pattern:        "https://*example.com",
			expectedResult: false,
		},
		{
			name:           "valid pattern with dot - should match",
			origin:         "https://sub.example.com",
			pattern:        "https://*.example.com",
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchOrigin(tt.origin, tt.pattern)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestCORSMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name                string
		origin              string
		method              string
		allowedOrigins      []string
		allowCredentials    bool
		expectedAllowOrigin string
		expectedStatus      int
		expectCORSHeaders   bool
	}{
		{
			name:                "valid origin - wildcard match",
			origin:              "https://test.app.thand.io",
			method:              "GET",
			allowedOrigins:      []string{"https://*.app.thand.io"},
			allowCredentials:    true,
			expectedAllowOrigin: "https://test.app.thand.io",
			expectedStatus:      http.StatusOK,
			expectCORSHeaders:   true,
		},
		{
			name:                "valid origin - exact match",
			origin:              "https://localhost:8080",
			method:              "GET",
			allowedOrigins:      []string{"https://localhost:8080"},
			allowCredentials:    true,
			expectedAllowOrigin: "https://localhost:8080",
			expectedStatus:      http.StatusOK,
			expectCORSHeaders:   true,
		},
		{
			name:                "invalid origin",
			origin:              "https://evil.com",
			method:              "GET",
			allowedOrigins:      []string{"https://*.app.thand.io"},
			allowCredentials:    true,
			expectedAllowOrigin: "",
			expectedStatus:      http.StatusOK,
			expectCORSHeaders:   false,
		},
		{
			name:                "preflight request - valid origin",
			origin:              "https://test.app.thand.io",
			method:              "OPTIONS",
			allowedOrigins:      []string{"https://*.app.thand.io"},
			allowCredentials:    true,
			expectedAllowOrigin: "https://test.app.thand.io",
			expectedStatus:      http.StatusNoContent,
			expectCORSHeaders:   true,
		},
		{
			name:                "preflight request - invalid origin",
			origin:              "https://evil.com",
			method:              "OPTIONS",
			allowedOrigins:      []string{"https://*.app.thand.io"},
			allowCredentials:    true,
			expectedAllowOrigin: "",
			expectedStatus:      http.StatusForbidden,
			expectCORSHeaders:   false,
		},
		{
			name:                "no origin header",
			origin:              "",
			method:              "GET",
			allowedOrigins:      []string{"https://*.app.thand.io"},
			allowCredentials:    true,
			expectedAllowOrigin: "",
			expectedStatus:      http.StatusOK,
			expectCORSHeaders:   false,
		},
		{
			name:                "multiple patterns - first match",
			origin:              "https://foo.app.thand.io",
			method:              "GET",
			allowedOrigins:      []string{"https://*.app.thand.io", "https://*.app.thand.com"},
			allowCredentials:    true,
			expectedAllowOrigin: "https://foo.app.thand.io",
			expectedStatus:      http.StatusOK,
			expectCORSHeaders:   true,
		},
		{
			name:                "multiple patterns - second match",
			origin:              "https://foo.app.thand.com",
			method:              "GET",
			allowedOrigins:      []string{"https://*.app.thand.io", "https://*.app.thand.com"},
			allowCredentials:    true,
			expectedAllowOrigin: "https://foo.app.thand.com",
			expectedStatus:      http.StatusOK,
			expectCORSHeaders:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corsConfig := models.CORSConfig{
				AllowedOrigins:   tt.allowedOrigins,
				AllowCredentials: tt.allowCredentials,
			}

			router := gin.New()
			router.Use(CORSMiddleware(corsConfig))
			router.GET("/test", func(c *gin.Context) {
				c.String(http.StatusOK, "OK")
			})
			router.OPTIONS("/test", func(c *gin.Context) {
				// This shouldn't be reached for preflight - middleware handles it
				c.String(http.StatusOK, "OPTIONS")
			})

			req := httptest.NewRequest(tt.method, "/test", nil)
			if len(tt.origin) != 0 {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.method == "OPTIONS" {
				req.Header.Set("Access-Control-Request-Method", "POST")
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectCORSHeaders {
				assert.Equal(t, tt.expectedAllowOrigin, w.Header().Get("Access-Control-Allow-Origin"))
				if tt.allowCredentials {
					assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
				}
			} else {
				assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}
