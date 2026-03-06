package config

import (
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// TestProcessProviderDefinitions_KeyIsPreserved verifies that the YAML map key
// used in a ProviderDefinitions entry becomes the map key in the returned
// map[string]ProviderConfig and is not altered in any way.
func TestProcessProviderDefinitions_KeyIsPreserved(t *testing.T) {
	cfg := &Config{mode: ModeServer}

	yamlKey := "my-custom-provider"
	defs := []*models.ProviderDefinitions{
		{
			Version: version.Must(version.NewVersion("1.0.0")),
			Providers: map[string]models.ProviderConfig{
				yamlKey: {
					Name:     "My Custom Provider",
					Provider: "aws",
					Enabled:  true,
				},
			},
		},
	}

	result := cfg.processProviderDefinitions(defs)

	require.Len(t, result, 1, "expected exactly one provider")
	_, exists := result[yamlKey]
	assert.True(t, exists, "expected the map key to be %q", yamlKey)

	// Ensure no other keys snuck in
	for key := range result {
		assert.Equal(t, yamlKey, key, "map key should match the original YAML key exactly")
	}
}
