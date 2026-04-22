package config

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

func TestBootstrapDeviceWithLoginServerMergesRemoteProviderDefinitions(t *testing.T) {
	t.Parallel()

	registerResponse := RegistrationResponse{
		Success: true,
		Providers: &ProviderDefinitionsConfig{
			Definitions: map[string]models.ProviderConfig{
				"oauth2-directory": {
					Name:        "Directory Login",
					Description: "Remote OAuth2 provider",
					Provider:    "oauth2",
					Enabled:     true,
					Config: &models.BasicConfig{
						"client_id":     "test-client-id",
						"client_secret": "test-client-secret",
						"auth_url":      "https://auth.example.com/oauth2/auth",
						"token_url":     "https://auth.example.com/oauth2/token",
						"redirect_url":  "http://localhost/callback",
					},
				},
			},
		},
	}

	loginServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/preflight":
			var req PreflightRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decoding preflight request: %v", err)
			}
			if got, want := req.Identifier.String(), common.GetDeviceID().String(); got != want {
				t.Fatalf("preflight identifier = %q, want %q", got, want)
			}
			w.WriteHeader(http.StatusOK)
		case "/api/v1/register":
			var req RegistrationRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decoding registration request: %v", err)
			}
			if got, want := req.Identifier.String(), common.GetDeviceID().String(); got != want {
				t.Fatalf("registration identifier = %q, want %q", got, want)
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(registerResponse); err != nil {
				t.Fatalf("encoding registration response: %v", err)
			}
		case "/api/v1/sync":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer loginServer.Close()

	cfg := &Config{
		mode: ModeAgent,
		Login: models.LoginConfig{
			Endpoint: &model.Endpoint{
				EndpointConfig: &model.EndpointConfiguration{
					URI: &model.LiteralUri{Value: loginServer.URL},
				},
			},
		},
		Thand: models.ThandConfig{
			Endpoint: loginServer.URL,
		},
		Providers: ProviderDefinitionsConfig{
			Definitions: map[string]models.ProviderConfig{
				"local-elevation": {
					Name:        "Local Elevation",
					Description: "Local privilege elevation provider",
					Provider:    "local",
					Enabled:     true,
				},
			},
		},
	}

	if err := cfg.InitializeProviders(); err != nil {
		t.Fatalf("InitializeProviders() error = %v", err)
	}

	if cfg.HasProvider("oauth2-directory") {
		t.Fatal("oauth2-directory provider already present before bootstrap")
	}

	if err := cfg.BootstrapDeviceWithLoginServer(); err != nil {
		t.Fatalf("BootstrapDeviceWithLoginServer() error = %v", err)
	}

	if !cfg.HasProvider("oauth2-directory") {
		t.Fatal("oauth2-directory provider missing after bootstrap")
	}
}

func TestPublishCurrentAgentRouteUsesCanonicalDeviceIdentity(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		mode: ModeAgent,
		Environment: models.EnvironmentConfig{
			Name:     "workstation-alpha",
			Hostname: "workstation-alpha.example.test",
			Platform: models.Local,
		},
	}

	var published models.DeviceConnectionState
	err := cfg.publishCurrentAgentRoute(context.Background(), func(ctx context.Context, state models.DeviceConnectionState) error {
		published = state
		return nil
	})
	if err != nil {
		t.Fatalf("publishCurrentAgentRoute() error = %v", err)
	}

	if got, want := published.DeviceID, common.GetDeviceID().String(); got != want {
		t.Fatalf("DeviceID = %q, want %q", got, want)
	}
	if got, want := published.TaskQueue, "thand_local_workstation_alpha"; got != want {
		t.Fatalf("TaskQueue = %q, want %q", got, want)
	}
}
