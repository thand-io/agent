package vault

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

type hashicorpProvider struct {
	config     *models.BasicConfig
	client     *api.Client
	mountPath  string
	secretPath string
}

func NewHashiCorpProvider(config *models.BasicConfig) models.VaultImpl {
	return &hashicorpProvider{
		config: config,
	}
}

func (h *hashicorpProvider) Initialize() error {
	// Get configuration
	vaultURL, foundVaultURL := h.config.GetString("vault_url")
	if !foundVaultURL {
		return fmt.Errorf("vault_url not found in config")
	}

	// Get mount path (defaults to "secret" if not specified)
	mountPath, foundMountPath := h.config.GetString("mount_path")
	if !foundMountPath {
		mountPath = "secret"
	}
	h.mountPath = mountPath

	// Get secret path (defaults to "data" for KV v2, empty for KV v1)
	secretPath, foundSecretPath := h.config.GetString("secret_path")
	if !foundSecretPath {
		secretPath = "data" // Default to KV v2 format
	}
	h.secretPath = secretPath

	// Create Vault client configuration
	config := api.DefaultConfig()
	config.Address = vaultURL

	// Set timeout
	if timeout, foundTimeout := h.config.GetString("timeout"); foundTimeout {
		if duration, err := common.ValidateDuration(timeout); err == nil {
			config.Timeout = duration
		}
	}

	// Create the client
	client, err := api.NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to create Vault client: %w", err)
	}

	// Set authentication token
	token, foundToken := h.config.GetString("token")
	if foundToken {
		client.SetToken(token)
	} else {
		// Try to get token from environment or token file
		// The Vault client will automatically check VAULT_TOKEN env var
		// and ~/.vault-token file
		if len(client.Token()) == 0 {
			return fmt.Errorf("vault token not found in config, environment (VAULT_TOKEN), or token file")
		}
	}

	h.client = client

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test by checking if we can access the sys/health endpoint
	req := client.NewRequest("GET", "/v1/sys/health")
	_, err = client.RawRequestWithContext(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to connect to Vault server: %w", err)
	}

	return nil
}

func (h *hashicorpProvider) Shutdown() error {
	// Vault client doesn't require explicit cleanup
	return nil
}

func (h *hashicorpProvider) GetSecret(key string) ([]byte, error) {
	if h.client == nil {
		return nil, fmt.Errorf("HashiCorp Vault client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try KV v2 first
	kvv2 := h.client.KVv2(h.mountPath)
	secret, err := kvv2.Get(ctx, key)
	if err != nil {
		// Try KV v1 if v2 fails
		logical := h.client.Logical()
		kvPath := fmt.Sprintf("%s/%s", h.mountPath, key)
		secret, err := logical.ReadWithContext(ctx, kvPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read secret '%s' from Vault: %w", key, err)
		}
		if secret == nil || secret.Data == nil {
			return nil, fmt.Errorf("secret '%s' not found", key)
		}

		// For KV v1, look for 'value' key or use the key itself
		if value, ok := secret.Data["value"].(string); ok {
			return []byte(value), nil
		}
		if value, ok := secret.Data[key].(string); ok {
			return []byte(value), nil
		}
		return nil, fmt.Errorf("secret '%s' does not contain a string value", key)
	}

	// For KV v2, the secret data is nested under "data"
	if secret.Data == nil {
		return nil, fmt.Errorf("secret '%s' not found", key)
	}

	// Look for 'value' key or use the key itself
	if value, ok := secret.Data["value"].(string); ok {
		return []byte(value), nil
	}
	if value, ok := secret.Data[key].(string); ok {
		return []byte(value), nil
	}

	return nil, fmt.Errorf("secret '%s' does not contain a string value", key)
}

func (h *hashicorpProvider) StoreSecret(key string, value []byte) error {
	if h.client == nil {
		return fmt.Errorf("HashiCorp Vault client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Prepare the secret data
	secretData := map[string]any{
		"value": string(value),
	}

	// Try KV v2 first
	kvv2 := h.client.KVv2(h.mountPath)
	_, err := kvv2.Put(ctx, key, secretData)
	if err != nil {
		// Try KV v1 if v2 fails
		logical := h.client.Logical()
		kvPath := fmt.Sprintf("%s/%s", h.mountPath, key)
		_, err := logical.WriteWithContext(ctx, kvPath, secretData)
		if err != nil {
			return fmt.Errorf("failed to store secret '%s' in Vault: %w", key, err)
		}
	}

	return nil
}
