package config

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/models"
)

func TestInitializeSingleProvider_ClientModeSkipsConfigValidation(t *testing.T) {
	t.Parallel()

	loginServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer loginServer.Close()

	cfg := &Config{
		mode: ModeClient,
		Login: models.LoginConfig{
			Endpoint: &model.Endpoint{
				EndpointConfig: &model.EndpointConfiguration{
					URI: &model.LiteralUri{Value: loginServer.URL},
				},
			},
		},
	}

	providerCfg := &models.ProviderConfig{
		Name:        "JumpCloud OAuth2",
		Description: "Config-less synced provider metadata",
		Provider:    "oauth2",
		Enabled:     true,
		Config: &models.BasicConfig{
			"redirect_url": `${ .THAND_REDIRECT_URL // "http://localhost/callback" }`,
		},
	}

	impl, err := cfg.initializeSingleProvider("oauth2-jumpcloud", providerCfg)
	if err != nil {
		t.Fatalf("initializeSingleProvider() error = %v", err)
	}
	if impl == nil {
		t.Fatal("initializeSingleProvider() returned nil provider")
	}

	if got := fmt.Sprintf("%T", impl); got != "*providers.remoteProviderProxy" {
		t.Fatalf("initializeSingleProvider() type = %s, want *providers.remoteProviderProxy", got)
	}
	if impl.GetIdentifier() != "oauth2-jumpcloud" {
		t.Fatalf("proxy identifier = %q, want %q", impl.GetIdentifier(), "oauth2-jumpcloud")
	}
	if impl.GetProvider() != "oauth2" {
		t.Fatalf("proxy provider = %q, want %q", impl.GetProvider(), "oauth2")
	}
	if got, ok := impl.GetConfig().GetString("redirect_url"); !ok || got != "http://localhost/callback" {
		t.Fatalf("proxy config redirect_url = %q, %v; want %q, true", got, ok, "http://localhost/callback")
	}
}

func TestInitializeSingleProvider_ServerModeStillValidatesConfig(t *testing.T) {
	t.Parallel()

	cfg := &Config{mode: ModeServer}
	providerCfg := &models.ProviderConfig{
		Name:        "JumpCloud OAuth2",
		Description: "Missing required oauth2 credentials",
		Provider:    "oauth2",
		Enabled:     true,
	}

	_, err := cfg.initializeSingleProvider("oauth2-jumpcloud", providerCfg)
	if err == nil {
		t.Fatal("initializeSingleProvider() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "provider config validation failed") {
		t.Fatalf("initializeSingleProvider() error = %v, want provider config validation failure", err)
	}
}
