package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/hashicorp/go-version"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// viperFromYAML loads YAML into Viper, unmarshals it into a Config,
// and returns the resulting *Config.
func viperFromYAML(t *testing.T, yamlContent string) *Config {
	t.Helper()

	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(yamlContent), 0600))

	v := viper.New()
	v.SetConfigFile(configFile)
	require.NoError(t, v.ReadInConfig())

	var config Config
	err := v.Unmarshal(&config, viper.DecodeHook(
		mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			stringToEndpointHookFunc(),
		),
	))
	require.NoError(t, err, "failed to unmarshal config")

	return &config
}

// TestInlineProviderDefinitions verifies that providers defined inline
// in the YAML (not via path/url/vault) are picked up in Definitions.
func TestInlineProviderDefinitions(t *testing.T) {
	yaml := `
providers:
  aws-dev:
    name: AWS Development
    description: Development AWS environment
    provider: aws
    config:
      region: us-west-2
      access_key_id: AKIA_TEST
      secret_access_key: SECRET_TEST
    enabled: true
  gcp-prod:
    name: GCP Production
    description: Production GCP environment
    provider: gcp
    config:
      project_id: my-project-123
    enabled: true
`

	config := viperFromYAML(t, yaml)

	t.Logf("Providers.Definitions = %+v", config.Providers.Definitions)
	t.Logf("Providers.Path = %q", config.Providers.Path)
	t.Logf("Providers.Vault = %q", config.Providers.Vault)

	require.NotNil(t, config.Providers.Definitions, "Definitions map should not be nil")
	assert.Len(t, config.Providers.Definitions, 2, "should have 2 inline provider definitions")

	awsDev, ok := config.Providers.Definitions["aws-dev"]
	assert.True(t, ok, "should have aws-dev provider")
	assert.Equal(t, "AWS Development", awsDev.Name)
	assert.Equal(t, "Development AWS environment", awsDev.Description)
	assert.Equal(t, "aws", awsDev.Provider)
	assert.True(t, awsDev.Enabled)
	require.NotNil(t, awsDev.Config, "aws-dev config should not be nil")
	region, regionOk := awsDev.Config.GetString("region")
	assert.True(t, regionOk, "region should exist")
	assert.Equal(t, "us-west-2", region)
	accessKey, accessKeyOk := awsDev.Config.GetString("access_key_id")
	assert.True(t, accessKeyOk, "access_key_id should exist")
	assert.Equal(t, "AKIA_TEST", accessKey)

	gcpProd, ok := config.Providers.Definitions["gcp-prod"]
	assert.True(t, ok, "should have gcp-prod provider")
	assert.Equal(t, "GCP Production", gcpProd.Name)
	assert.Equal(t, "gcp", gcpProd.Provider)
	assert.True(t, gcpProd.Enabled)
	require.NotNil(t, gcpProd.Config, "gcp-prod config should not be nil")
	projectID, projectIDOk := gcpProd.Config.GetString("project_id")
	assert.True(t, projectIDOk, "project_id should exist")
	assert.Equal(t, "my-project-123", projectID)
}

// TestInlineProviderDefinitionsWithPath verifies that inline providers
// coexist with the path field without interfering.
func TestInlineProviderDefinitionsWithPath(t *testing.T) {
	yaml := `
providers:
  path: ./config/providers
  aws-inline:
    name: AWS Inline
    provider: aws
    config:
      region: eu-west-1
    enabled: true
`

	config := viperFromYAML(t, yaml)

	t.Logf("Providers.Path = %q", config.Providers.Path)
	t.Logf("Providers.Definitions = %+v", config.Providers.Definitions)

	assert.Equal(t, "./config/providers", config.Providers.Path)

	require.NotNil(t, config.Providers.Definitions, "Definitions map should not be nil")
	assert.Len(t, config.Providers.Definitions, 1, "should have 1 inline provider definition")

	awsInline, ok := config.Providers.Definitions["aws-inline"]
	assert.True(t, ok, "should have aws-inline provider")
	assert.Equal(t, "AWS Inline", awsInline.Name)
	assert.Equal(t, "aws", awsInline.Provider)
	assert.True(t, awsInline.Enabled)
}

// TestInlineRoleDefinitions verifies that roles defined inline
// are picked up in the Definitions map.
func TestInlineRoleDefinitions(t *testing.T) {
	yaml := `
roles:
  admin_role:
    name: Admin
    description: Full admin access
    authenticators:
      - google_oauth2
    workflows:
      - approval_workflow
    providers:
      - aws-dev
    inherits:
      - arn:aws:iam::aws:policy/AdministratorAccess
    enabled: true
  reader_role:
    name: Reader
    description: Read only access
    providers:
      - aws-dev
    enabled: true
`

	config := viperFromYAML(t, yaml)

	t.Logf("Roles.Definitions = %+v", config.Roles.Definitions)

	require.NotNil(t, config.Roles.Definitions, "Definitions map should not be nil")
	assert.Len(t, config.Roles.Definitions, 2, "should have 2 inline role definitions")

	admin, ok := config.Roles.Definitions["admin_role"]
	assert.True(t, ok, "should have admin_role")
	assert.Equal(t, "Admin", admin.Name)
	assert.Equal(t, "Full admin access", admin.Description)
	assert.Equal(t, []string{"google_oauth2"}, admin.Authenticators)
	assert.Equal(t, []string{"approval_workflow"}, admin.Workflows)
	assert.Equal(t, []string{"aws-dev"}, admin.Providers)
	assert.Equal(t, []string{"arn:aws:iam::aws:policy/AdministratorAccess"}, admin.Inherits)
	assert.True(t, admin.Enabled)

	reader, ok := config.Roles.Definitions["reader_role"]
	assert.True(t, ok, "should have reader_role")
	assert.Equal(t, "Reader", reader.Name)
	assert.True(t, reader.Enabled)
}

// TestInlineRoleDefinitionsWithPath verifies that inline roles
// coexist with the path field.
func TestInlineRoleDefinitionsWithPath(t *testing.T) {
	yaml := `
roles:
  path: ./config/roles
  vault: secret/roles
  inline_role:
    name: Inline Role
    description: A role defined inline along with path and vault
    providers:
      - gcp-prod
    enabled: true
`

	config := viperFromYAML(t, yaml)

	assert.Equal(t, "./config/roles", config.Roles.Path)
	assert.Equal(t, "secret/roles", config.Roles.Vault)

	require.NotNil(t, config.Roles.Definitions, "Definitions map should not be nil")
	assert.Len(t, config.Roles.Definitions, 1, "should have 1 inline role definition")

	role, ok := config.Roles.Definitions["inline_role"]
	assert.True(t, ok, "should have inline_role")
	assert.Equal(t, "Inline Role", role.Name)
}

// TestInlineWorkflowDefinitions verifies that workflows defined inline
// are picked up in the Definitions map.
func TestInlineWorkflowDefinitions(t *testing.T) {
	yaml := `
workflows:
  simple_workflow:
    name: Simple Workflow
    description: A basic approval workflow
    enabled: true
`

	config := viperFromYAML(t, yaml)

	t.Logf("Workflows.Definitions = %+v", config.Workflows.Definitions)

	require.NotNil(t, config.Workflows.Definitions, "Definitions map should not be nil")
	assert.Len(t, config.Workflows.Definitions, 1, "should have 1 inline workflow definition")

	wf, ok := config.Workflows.Definitions["simple_workflow"]
	assert.True(t, ok, "should have simple_workflow")
	assert.Equal(t, "Simple Workflow", wf.Name)
	assert.Equal(t, "A basic approval workflow", wf.Description)
	assert.True(t, wf.Enabled)
}

// TestInlineWorkflowDefinitionsWithPath verifies that inline workflows
// coexist with the path field.
func TestInlineWorkflowDefinitionsWithPath(t *testing.T) {
	yaml := `
workflows:
  path: ./config/workflows
  inline_wf:
    name: Inline Workflow
    description: Defined inline with path
    enabled: true
`

	config := viperFromYAML(t, yaml)

	assert.Equal(t, "./config/workflows", config.Workflows.Path)

	require.NotNil(t, config.Workflows.Definitions, "Definitions map should not be nil")
	assert.Len(t, config.Workflows.Definitions, 1, "should have 1 inline workflow definition")

	wf, ok := config.Workflows.Definitions["inline_wf"]
	assert.True(t, ok, "should have inline_wf")
	assert.Equal(t, "Inline Workflow", wf.Name)
}

// TestInlineProviderDisabled verifies that the enabled=false flag is respected.
func TestInlineProviderDisabled(t *testing.T) {
	yaml := `
providers:
  disabled-provider:
    name: Disabled Provider
    provider: aws
    config:
      region: us-east-1
    enabled: false
`

	config := viperFromYAML(t, yaml)

	require.NotNil(t, config.Providers.Definitions)
	assert.Len(t, config.Providers.Definitions, 1)

	p, ok := config.Providers.Definitions["disabled-provider"]
	assert.True(t, ok, "should have disabled-provider")
	assert.Equal(t, "Disabled Provider", p.Name)
	assert.False(t, p.Enabled, "provider should be disabled")
}

// TestInlineProviderNestedConfig verifies that deeply nested config maps
// are preserved when loaded inline.
func TestInlineProviderNestedConfig(t *testing.T) {
	yaml := `
providers:
  gcp-nested:
    name: GCP Nested
    provider: gcp
    config:
      project_id: my-project
      credentials:
        type: service_account
        project_id: my-project
        client_email: test@my-project.iam.gserviceaccount.com
    enabled: true
`

	config := viperFromYAML(t, yaml)

	require.NotNil(t, config.Providers.Definitions)

	p, ok := config.Providers.Definitions["gcp-nested"]
	assert.True(t, ok, "should have gcp-nested provider")
	require.NotNil(t, p.Config, "config should not be nil")

	projectID, projectIDOk := p.Config.GetString("project_id")
	assert.True(t, projectIDOk, "project_id should exist")
	assert.Equal(t, "my-project", projectID)

	// Verify nested credentials
	creds, credsOk := p.Config.GetMap("credentials")
	require.True(t, credsOk, "credentials should exist")
	require.NotNil(t, creds, "credentials should not be nil")
	assert.Equal(t, "service_account", creds["type"])
	assert.Equal(t, "test@my-project.iam.gserviceaccount.com", creds["client_email"])
}

// TestInlineFullConfig verifies that a complete config with inline
// providers, roles, and workflows all load together correctly.
func TestInlineFullConfig(t *testing.T) {
	yaml := `
server:
  host: "0.0.0.0"
  port: 5225

providers:
  my-aws:
    name: My AWS
    provider: aws
    config:
      region: us-east-1
    enabled: true

roles:
  my-role:
    name: My Role
    description: Test role
    providers:
      - my-aws
    enabled: true

workflows:
  my-workflow:
    name: My Workflow
    description: Test workflow
    enabled: true
`

	config := viperFromYAML(t, yaml)

	// Verify server config still works
	assert.Equal(t, "0.0.0.0", config.Server.Host)
	assert.Equal(t, 5225, config.Server.Port)

	// Verify providers
	require.NotNil(t, config.Providers.Definitions)
	assert.Contains(t, config.Providers.Definitions, "my-aws")
	assert.Equal(t, "My AWS", config.Providers.Definitions["my-aws"].Name)

	// Verify roles
	require.NotNil(t, config.Roles.Definitions)
	assert.Contains(t, config.Roles.Definitions, "my-role")
	assert.Equal(t, "My Role", config.Roles.Definitions["my-role"].Name)

	// Verify workflows
	require.NotNil(t, config.Workflows.Definitions)
	assert.Contains(t, config.Workflows.Definitions, "my-workflow")
	assert.Equal(t, "My Workflow", config.Workflows.Definitions["my-workflow"].Name)
}

// TestInlineEmptyConfig verifies that empty sections produce nil/empty Definitions.
func TestInlineEmptyConfig(t *testing.T) {
	yaml := `
providers:
  path: ./config/providers
roles:
  path: ./config/roles
workflows:
  path: ./config/workflows
`

	config := viperFromYAML(t, yaml)

	assert.Equal(t, "./config/providers", config.Providers.Path)
	assert.Equal(t, "./config/roles", config.Roles.Path)
	assert.Equal(t, "./config/workflows", config.Workflows.Path)

	// With no inline definitions, Definitions should be empty (or nil)
	assert.Empty(t, config.Providers.Definitions)
	assert.Empty(t, config.Roles.Definitions)
	assert.Empty(t, config.Workflows.Definitions)
}

// TestInlineMultipleProvidersSameType verifies multiple providers of the same
// type (e.g., two AWS providers) are loaded independently.
func TestInlineMultipleProvidersSameType(t *testing.T) {
	yaml := `
providers:
  aws-dev:
    name: AWS Dev
    provider: aws
    config:
      region: us-west-2
    enabled: true
  aws-staging:
    name: AWS Staging
    provider: aws
    config:
      region: us-east-1
    enabled: true
  aws-prod:
    name: AWS Prod
    provider: aws
    config:
      region: eu-west-1
    enabled: false
`

	config := viperFromYAML(t, yaml)

	require.NotNil(t, config.Providers.Definitions)
	assert.Len(t, config.Providers.Definitions, 3, "should have 3 inline provider definitions")

	assert.Equal(t, "AWS Dev", config.Providers.Definitions["aws-dev"].Name)
	devRegion, _ := config.Providers.Definitions["aws-dev"].Config.GetString("region")
	assert.Equal(t, "us-west-2", devRegion)
	assert.True(t, config.Providers.Definitions["aws-dev"].Enabled)

	assert.Equal(t, "AWS Staging", config.Providers.Definitions["aws-staging"].Name)
	stagingRegion, _ := config.Providers.Definitions["aws-staging"].Config.GetString("region")
	assert.Equal(t, "us-east-1", stagingRegion)
	assert.True(t, config.Providers.Definitions["aws-staging"].Enabled)

	assert.Equal(t, "AWS Prod", config.Providers.Definitions["aws-prod"].Name)
	prodRegion, _ := config.Providers.Definitions["aws-prod"].Config.GetString("region")
	assert.Equal(t, "eu-west-1", prodRegion)
	assert.False(t, config.Providers.Definitions["aws-prod"].Enabled)
}

// TestInlineThandProviderEndpoint verifies that the endpoint field inside
// a provider's config map is correctly loaded when defined inline.
// This is the exact scenario that was failing: the thand auth provider's
// endpoint field was not being set when defined inline in the YAML config.
func TestInlineThandProviderEndpoint(t *testing.T) {
	yaml := `
providers:
  thand:
    name: Thand Dev
    provider: thand
    description: Thand Development Provider
    enabled: true
    capabilities:
      authorizer:
        enabled: true
    config:
      endpoint: "https://auth.thand.dev"
`

	config := viperFromYAML(t, yaml)

	require.NotNil(t, config.Providers.Definitions)

	thandProvider, ok := config.Providers.Definitions["thand"]
	require.True(t, ok, "should have thand provider")
	assert.Equal(t, "Thand Dev", thandProvider.Name)
	assert.Equal(t, "thand", thandProvider.Provider)
	assert.Equal(t, "Thand Development Provider", thandProvider.Description)
	assert.True(t, thandProvider.Enabled)

	require.NotNil(t, thandProvider.Config, "thand config should not be nil")

	// This is the critical assertion — endpoint must be a string, not
	// converted to a model.Endpoint by the stringToEndpointHookFunc
	endpoint, endpointOk := thandProvider.Config.GetString("endpoint")
	t.Logf("thand config map: %+v", *thandProvider.Config)
	t.Logf("endpoint value: %q, found: %v", endpoint, endpointOk)

	// Check what type the endpoint value actually is in the map
	if rawVal, exists := (*thandProvider.Config)["endpoint"]; exists {
		t.Logf("endpoint raw type: %T, value: %+v", rawVal, rawVal)
	} else {
		t.Error("endpoint key does not exist in config map at all")
	}

	assert.True(t, endpointOk, "endpoint should be retrievable as a string")
	assert.Equal(t, "https://auth.thand.dev", endpoint,
		"endpoint should match the inline config value")
}

// TestInlineThandProviderEndpointSurvivesApplyProviders verifies the endpoint
// inside a thand provider's config map survives the full ApplyProviders pipeline.
func TestInlineThandProviderEndpointSurvivesApplyProviders(t *testing.T) {
	yaml := `
providers:
  thand:
    name: Thand Dev
    provider: thand
    description: Thand Development Provider
    enabled: true
    capabilities:
      authorizer:
        enabled: true
    config:
      endpoint: "https://auth.thand.dev"
`
	config := viperFromYAML(t, yaml)
	config.mode = ModeServer

	// Run through ApplyProviders (the pipeline ReloadConfig uses)
	result, err := config.ApplyProviders([]*models.ProviderDefinitions{})
	require.NoError(t, err)

	thandDef, ok := result["thand"]
	require.True(t, ok, "thand provider should survive ApplyProviders")
	require.NotNil(t, thandDef.Config, "thand config should not be nil after ApplyProviders")

	endpoint, endpointOk := thandDef.Config.GetString("endpoint")
	t.Logf("After ApplyProviders — endpoint: %q, found: %v", endpoint, endpointOk)
	t.Logf("After ApplyProviders — full config map: %+v", *thandDef.Config)

	assert.True(t, endpointOk, "endpoint should survive ApplyProviders pipeline")
	assert.Equal(t, "https://auth.thand.dev", endpoint)
}

// TestInlineThandProviderEndpointSurvivesReloadConfig verifies the endpoint
// survives a full ReloadConfig cycle (which loads defaults + merges inline).
func TestInlineThandProviderEndpointSurvivesReloadConfig(t *testing.T) {
	yaml := `
providers:
  thand:
    name: Thand Dev
    provider: thand
    description: Thand Development Provider
    enabled: true
    capabilities:
      authorizer:
        enabled: true
    config:
      endpoint: "https://auth.thand.dev"
`
	config := viperFromYAML(t, yaml)
	config.mode = ModeServer

	// Verify inline config before reload
	require.NotNil(t, config.Providers.Definitions["thand"].Config)
	beforeEndpoint, _ := config.Providers.Definitions["thand"].Config.GetString("endpoint")
	t.Logf("Before ReloadConfig — endpoint: %q", beforeEndpoint)

	// ReloadConfig calls LoadProviders which merges defaults + inline
	_ = config.ReloadConfig()

	thandDef, ok := config.Providers.Definitions["thand"]
	require.True(t, ok, "thand provider should exist after ReloadConfig")
	require.NotNil(t, thandDef.Config, "thand config should not be nil after ReloadConfig")

	endpoint, endpointOk := thandDef.Config.GetString("endpoint")
	t.Logf("After ReloadConfig — endpoint: %q, found: %v", endpoint, endpointOk)
	t.Logf("After ReloadConfig — full config map: %+v", *thandDef.Config)

	assert.True(t, endpointOk, "endpoint should survive ReloadConfig")
	assert.Equal(t, "https://auth.thand.dev", endpoint)
}

// TestInlineThandProviderEndpointVsFilePath compares inline loading with
// the file-based loading path to demonstrate the discrepancy.
func TestInlineThandProviderEndpointVsFilePath(t *testing.T) {
	// Simulate what the file-based loading path does:
	// YAML -> yaml.Unmarshal -> JSON -> json.Unmarshal into ProviderDefinitions
	fileYAML := `version: "1.0"
providers:
  thand:
    name: Thand Dev
    provider: thand
    description: Thand Development Provider
    enabled: true
    capabilities:
      authorizer:
        enabled: true
    config:
      endpoint: "https://auth.thand.dev"
`
	var fileDefs models.ProviderDefinitions
	fileResult, err := common.ReadDataToInterface([]byte(fileYAML), fileDefs)
	require.NoError(t, err)

	fileProvider, ok := fileResult.Providers["thand"]
	require.True(t, ok, "file-loaded thand provider should exist")
	require.NotNil(t, fileProvider.Config, "file-loaded config should not be nil")

	fileEndpoint, fileEndpointOk := fileProvider.Config.GetString("endpoint")
	t.Logf("File-loaded endpoint: %q (found: %v)", fileEndpoint, fileEndpointOk)
	if rawVal, exists := (*fileProvider.Config)["endpoint"]; exists {
		t.Logf("File-loaded endpoint raw type: %T, value: %+v", rawVal, rawVal)
	}
	assert.True(t, fileEndpointOk, "file-loaded endpoint should be a string")
	assert.Equal(t, "https://auth.thand.dev", fileEndpoint)

	// Now load inline via Viper
	inlineYAML := `
providers:
  thand:
    name: Thand Dev
    provider: thand
    description: Thand Development Provider
    enabled: true
    capabilities:
      authorizer:
        enabled: true
    config:
      endpoint: "https://auth.thand.dev"
`
	config := viperFromYAML(t, inlineYAML)
	inlineProvider, ok := config.Providers.Definitions["thand"]
	require.True(t, ok, "inline thand provider should exist")
	require.NotNil(t, inlineProvider.Config, "inline config should not be nil")

	inlineEndpoint, inlineEndpointOk := inlineProvider.Config.GetString("endpoint")
	t.Logf("Inline-loaded endpoint: %q (found: %v)", inlineEndpoint, inlineEndpointOk)

	// Dump the raw map to see what type the value is
	if rawVal, exists := (*inlineProvider.Config)["endpoint"]; exists {
		t.Logf("Inline endpoint raw type: %T, value: %+v", rawVal, rawVal)
	} else {
		t.Error("Inline endpoint key missing from config map entirely")
	}

	// Both should produce the same result
	assert.True(t, inlineEndpointOk, "inline endpoint should be retrievable as a string")
	assert.Equal(t, "https://auth.thand.dev", inlineEndpoint,
		"inline endpoint should match the config value")

	// Verify capabilities survived too
	require.NotNil(t, inlineProvider.Capabilities, "inline capabilities should not be nil")
}

// --------------------------------------------------------------------------
// Tests that exercise the Apply* / Load* pipeline to verify inline
// definitions survive the full loading path (the path that ReloadConfig uses).
// --------------------------------------------------------------------------

// TestApplyProviders_InlineDefinitionsMergedWithExternal verifies that
// ApplyProviders merges inline Definitions from the config with externally
// loaded provider definitions.
func TestApplyProviders_InlineDefinitionsMergedWithExternal(t *testing.T) {
	yaml := `
providers:
  my-aws:
    name: My AWS Inline
    provider: aws
    config:
      region: us-west-2
    enabled: true
`
	config := viperFromYAML(t, yaml)
	config.mode = ModeServer

	// Simulate externally loaded providers (e.g. from path or defaults)
	externalProviders := []*models.ProviderDefinitions{
		{
			Version: version.Must(version.NewVersion("1.0")),
			Providers: map[string]models.ProviderConfig{
				"external-gcp": {
					Name:     "External GCP",
					Provider: "gcp",
					Enabled:  true,
					Config:   &models.BasicConfig{"project_id": "ext-project"},
				},
			},
		},
	}

	result, err := config.ApplyProviders(externalProviders)
	require.NoError(t, err)

	t.Logf("ApplyProviders result keys: %v", testMapKeysStr(result))

	// Both external and inline should be present
	assert.Contains(t, result, "external-gcp", "external provider should be in result")
	assert.Contains(t, result, "my-aws", "inline provider should be in result")
	assert.Equal(t, "External GCP", result["external-gcp"].Name)
	assert.Equal(t, "My AWS Inline", result["my-aws"].Name)
}

// TestApplyProviders_InlineOnlyNoExternal verifies that when there are no
// external sources, inline definitions alone produce valid results.
func TestApplyProviders_InlineOnlyNoExternal(t *testing.T) {
	yaml := `
providers:
  inline-aws:
    name: Inline AWS
    provider: aws
    config:
      region: eu-west-1
    enabled: true
  inline-gcp:
    name: Inline GCP
    provider: gcp
    config:
      project_id: test-project
    enabled: true
`
	config := viperFromYAML(t, yaml)
	config.mode = ModeServer

	// Empty external providers list - simulates no path/url/vault
	result, err := config.ApplyProviders([]*models.ProviderDefinitions{})
	require.NoError(t, err)

	t.Logf("ApplyProviders result: %v", testMapKeysStr(result))

	assert.Len(t, result, 2, "both inline providers should be in result")
	assert.Contains(t, result, "inline-aws")
	assert.Contains(t, result, "inline-gcp")
	assert.Equal(t, "Inline AWS", result["inline-aws"].Name)
	assert.Equal(t, "Inline GCP", result["inline-gcp"].Name)
}

// TestApplyProviders_DisabledInlineFilteredOut verifies that disabled inline
// providers are filtered out by processProviderDefinitions.
func TestApplyProviders_DisabledInlineFilteredOut(t *testing.T) {
	yaml := `
providers:
  enabled-aws:
    name: Enabled AWS
    provider: aws
    config:
      region: us-east-1
    enabled: true
  disabled-aws:
    name: Disabled AWS
    provider: aws
    config:
      region: us-west-1
    enabled: false
`
	config := viperFromYAML(t, yaml)
	config.mode = ModeServer

	result, err := config.ApplyProviders([]*models.ProviderDefinitions{})
	require.NoError(t, err)

	// Only the enabled provider should survive
	assert.Len(t, result, 1, "only enabled provider should be in result")
	assert.Contains(t, result, "enabled-aws")
	assert.NotContains(t, result, "disabled-aws", "disabled provider should be filtered out")
}

// TestApplyRoles_InlineDefinitionsMergedWithExternal verifies that ApplyRoles
// merges inline Definitions with externally loaded role definitions.
func TestApplyRoles_InlineDefinitionsMergedWithExternal(t *testing.T) {
	yaml := `
roles:
  inline-admin:
    name: Inline Admin
    description: Admin role from config
    providers:
      - aws-dev
    enabled: true
`
	config := viperFromYAML(t, yaml)
	config.mode = ModeServer

	externalRoles := []*models.RoleDefinitions{
		{
			Version: version.Must(version.NewVersion("1.0")),
			Roles: map[string]models.Role{
				"external-reader": {
					Name:      "External Reader",
					Providers: []string{"gcp-prod"},
					Enabled:   true,
				},
			},
		},
	}

	result, err := config.ApplyRoles(externalRoles)
	require.NoError(t, err)

	t.Logf("ApplyRoles result keys: %v", testMapKeysStr(result))

	assert.Contains(t, result, "external-reader", "external role should be in result")
	assert.Contains(t, result, "inline-admin", "inline role should be in result")
	assert.Equal(t, "External Reader", result["external-reader"].Name)
	assert.Equal(t, "Inline Admin", result["inline-admin"].Name)
}

// TestApplyRoles_InlineOnlyNoExternal verifies inline roles alone produce valid results.
func TestApplyRoles_InlineOnlyNoExternal(t *testing.T) {
	yaml := `
roles:
  admin:
    name: Admin
    description: Full access
    providers:
      - my-aws
    enabled: true
`
	config := viperFromYAML(t, yaml)
	config.mode = ModeServer

	result, err := config.ApplyRoles([]*models.RoleDefinitions{})
	require.NoError(t, err)

	assert.Len(t, result, 1)
	assert.Contains(t, result, "admin")
	assert.Equal(t, "Admin", result["admin"].Name)
}

// TestApplyWorkflows_InlineDefinitionsMergedWithExternal verifies that
// ApplyWorkflows merges inline Definitions with externally loaded workflows.
func TestApplyWorkflows_InlineDefinitionsMergedWithExternal(t *testing.T) {
	yaml := `
workflows:
  inline-approval:
    name: Inline Approval
    description: Approval workflow from config
    enabled: true
`
	config := viperFromYAML(t, yaml)
	config.mode = ModeClient // client mode doesn't require workflow.Workflow field

	externalWorkflows := []*models.WorkflowDefinitions{
		{
			Version: version.Must(version.NewVersion("1.0")),
			Workflows: map[string]models.Workflow{
				"external-deploy": {
					Name:    "External Deploy",
					Enabled: true,
				},
			},
		},
	}

	result, err := config.ApplyWorkflows(externalWorkflows)
	require.NoError(t, err)

	t.Logf("ApplyWorkflows result keys: %v", testMapKeysStr(result))

	assert.Contains(t, result, "external-deploy", "external workflow should be in result")
	assert.Contains(t, result, "inline-approval", "inline workflow should be in result")
	assert.Equal(t, "External Deploy", result["external-deploy"].Name)
	assert.Equal(t, "Inline Approval", result["inline-approval"].Name)
}

// TestApplyWorkflows_InlineOnlyNoExternal verifies inline workflows alone produce valid results.
func TestApplyWorkflows_InlineOnlyNoExternal(t *testing.T) {
	yaml := `
workflows:
  my-wf:
    name: My Workflow
    description: Simple workflow
    enabled: true
`
	config := viperFromYAML(t, yaml)
	config.mode = ModeClient

	result, err := config.ApplyWorkflows([]*models.WorkflowDefinitions{})
	require.NoError(t, err)

	assert.Len(t, result, 1)
	assert.Contains(t, result, "my-wf")
	assert.Equal(t, "My Workflow", result["my-wf"].Name)
}

// TestReloadConfig_OverwritesDefinitions demonstrates that after ReloadConfig,
// the original inline Definitions are replaced with the processed result.
// This means inline definitions from the YAML are only used on the FIRST load.
// A second ReloadConfig may lose them if defaults clobber the Definitions field.
func TestReloadConfig_OverwritesDefinitions(t *testing.T) {
	yaml := `
providers:
  my-custom-aws:
    name: Custom AWS
    provider: aws
    config:
      region: us-west-2
    enabled: true

roles:
  my-custom-role:
    name: Custom Role
    description: A custom role
    providers:
      - my-custom-aws
    enabled: true
`

	config := viperFromYAML(t, yaml)
	config.mode = ModeServer

	// Verify inline definitions are present before ReloadConfig
	require.Contains(t, config.Providers.Definitions, "my-custom-aws",
		"inline provider should exist before ReloadConfig")
	require.Contains(t, config.Roles.Definitions, "my-custom-role",
		"inline role should exist before ReloadConfig")

	// Save original inline definitions for comparison
	originalProviderDefs := make(map[string]models.ProviderConfig)
	for k, v := range config.Providers.Definitions {
		originalProviderDefs[k] = v
	}

	// Run ReloadConfig - this calls LoadProviders/LoadRoles/LoadWorkflows
	// which merge inline definitions and then overwrite c.*.Definitions.
	// Note: ReloadConfig may return an error from workflow loading (default
	// workflows have task types that the SDK doesn't support), but providers
	// and roles should still load successfully.
	_ = config.ReloadConfig()

	// After ReloadConfig, verify the inline provider survived the merge
	t.Logf("Providers after ReloadConfig: %v", testMapKeysStr(config.Providers.Definitions))
	t.Logf("Roles after ReloadConfig: %v", testMapKeysStr(config.Roles.Definitions))

	assert.Contains(t, config.Providers.Definitions, "my-custom-aws",
		"inline provider should survive ReloadConfig")
	assert.Equal(t, "Custom AWS", config.Providers.Definitions["my-custom-aws"].Name)

	assert.Contains(t, config.Roles.Definitions, "my-custom-role",
		"inline role should survive ReloadConfig")
	assert.Equal(t, "Custom Role", config.Roles.Definitions["my-custom-role"].Name)

	// Now run ReloadConfig AGAIN - THIS is where the bug could manifest.
	// The first ReloadConfig replaced Definitions with processed results
	// (which includes defaults + inline). The second ReloadConfig reads those
	// as "inline" definitions, so they should still be present.
	_ = config.ReloadConfig()

	t.Logf("Providers after 2nd ReloadConfig: %v", testMapKeysStr(config.Providers.Definitions))
	t.Logf("Roles after 2nd ReloadConfig: %v", testMapKeysStr(config.Roles.Definitions))

	assert.Contains(t, config.Providers.Definitions, "my-custom-aws",
		"inline provider should survive second ReloadConfig")
	assert.Contains(t, config.Roles.Definitions, "my-custom-role",
		"inline role should survive second ReloadConfig")
}

// TestApplyProviders_PreservesInlineAfterReplace exercises the scenario where
// ApplyProviders is called twice to verify inline definitions are not lost.
func TestApplyProviders_PreservesInlineAfterReplace(t *testing.T) {
	yaml := `
providers:
  my-inline-provider:
    name: My Inline
    provider: aws
    config:
      region: us-east-1
    enabled: true
`
	config := viperFromYAML(t, yaml)
	config.mode = ModeServer

	// First apply with empty externals - should include inline
	result1, err := config.ApplyProviders([]*models.ProviderDefinitions{})
	require.NoError(t, err)
	assert.Contains(t, result1, "my-inline-provider", "inline provider should be in first apply")

	// Simulate what ReloadConfig does: overwrite Definitions with result
	config.Providers.Definitions = result1

	// Second apply - the "inline" definitions are now the processed ones
	result2, err := config.ApplyProviders([]*models.ProviderDefinitions{})
	require.NoError(t, err)

	t.Logf("result1 keys: %v", testMapKeysStr(result1))
	t.Logf("result2 keys: %v", testMapKeysStr(result2))

	assert.Contains(t, result2, "my-inline-provider",
		"inline provider should survive second ApplyProviders after Definitions replacement")
	assert.Equal(t, "My Inline", result2["my-inline-provider"].Name)
}

// TestApplyProviders_InlineDuplicateKeyWithExternal verifies that when an
// inline provider has the same key as an external one, only one is kept.
func TestApplyProviders_InlineDuplicateKeyWithExternal(t *testing.T) {
	yaml := `
providers:
  shared-provider:
    name: Inline Version
    provider: aws
    config:
      region: us-west-2
    enabled: true
`
	config := viperFromYAML(t, yaml)
	config.mode = ModeServer

	externalProviders := []*models.ProviderDefinitions{
		{
			Version: version.Must(version.NewVersion("1.0")),
			Providers: map[string]models.ProviderConfig{
				"shared-provider": {
					Name:     "External Version",
					Provider: "aws",
					Enabled:  true,
					Config:   &models.BasicConfig{"region": "eu-west-1"},
				},
			},
		},
	}

	result, err := config.ApplyProviders(externalProviders)
	require.NoError(t, err)

	// One of them should win (inline is now processed first, so it takes priority)
	assert.Contains(t, result, "shared-provider")
	t.Logf("shared-provider name after merge: %q", result["shared-provider"].Name)

	// Inline definitions take priority over external ones
	assert.Equal(t, "Inline Version", result["shared-provider"].Name,
		"inline provider should win when keys conflict (processed first)")
}

// testMapKeysStr returns the keys of a map[string]V for test logging.
func testMapKeysStr[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
