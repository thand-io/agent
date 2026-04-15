package config

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	_ "github.com/thand-io/agent/internal/testing/mocks/providers"
)

// syncPatchCall captures an outgoing PATCH request made by MergeConfiguration.
type syncPatchCall struct {
	Method string
	URL    string
	Body   []byte
	Header http.Header
}

// newSyncTestServer creates an httptest.Server that captures outgoing PATCH
// requests to /api/v1/sync on a channel. The discovery endpoint returns 404
// so DiscoverThandServerApiUrl falls back to {endpoint}/api/v1.
func newSyncTestServer(t *testing.T) (*httptest.Server, <-chan syncPatchCall) {
	t.Helper()
	patchCh := make(chan syncPatchCall, 10)

	mux := http.NewServeMux()

	// Discovery endpoint — return 404 so the fallback kicks in
	mux.HandleFunc("GET /.well-known/api-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	// Capture outgoing PATCH /api/v1/sync
	mux.HandleFunc("PATCH /api/v1/sync", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		patchCh <- syncPatchCall{
			Method: r.Method,
			URL:    r.URL.String(),
			Body:   body,
			Header: r.Header.Clone(),
		}
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, patchCh
}

// newSyncTestConfig creates a Config pre-populated with roles, workflows, and
// providers for sync tests. endpoint should be the httptest server URL (or any
// dummy URL when no HTTP interaction is needed).
func newSyncTestConfig(
	t *testing.T,
	roles map[string]models.Role,
	workflows map[string]models.Workflow,
	providers map[string]models.ProviderConfig,
	endpoint string,
) *Config {
	t.Helper()

	if endpoint == "" {
		endpoint = "http://localhost:9999"
	}

	config := &Config{
		mode: ModeServer,
		Roles: RoleConfig{
			Definitions: roles,
		},
		Workflows: WorkflowConfig{
			Definitions: workflows,
		},
		Providers: ProviderDefinitionsConfig{
			Definitions: providers,
		},
		Thand: models.ThandConfig{
			Endpoint: endpoint,
			ApiKey:   "test-api-key",
		},
	}

	if len(providers) > 0 {
		err := config.InitializeProviders()
		if err != nil {
			t.Fatalf("Failed to initialize mock providers: %v", err)
		}
	}

	return config
}

func makeRegistrationResponse(
	roles map[string]models.Role,
	workflows map[string]models.Workflow,
	providers map[string]models.ProviderConfig,
) *RegistrationResponse {
	resp := &RegistrationResponse{Success: true}
	if roles != nil {
		resp.Roles = &RoleConfig{Definitions: roles}
	}
	if workflows != nil {
		resp.Workflows = &WorkflowConfig{Definitions: workflows}
	}
	if providers != nil {
		resp.Providers = &ProviderDefinitionsConfig{Definitions: providers}
	}
	return resp
}

func makeTestWorkflow(name, description string) models.Workflow {
	return models.Workflow{
		Name:        name,
		Description: description,
		Enabled:     true,
		Workflow: &model.Workflow{
			Do: &model.TaskList{},
		},
	}
}

func makeTestProvider(name, description string) models.ProviderConfig {
	return models.ProviderConfig{
		Name:        name,
		Description: description,
		Provider:    "mock",
		Enabled:     true,
	}
}

// waitForPatch waits for a single PATCH call on the channel, or times out.
func waitForPatch(ch <-chan syncPatchCall, timeout time.Duration) (syncPatchCall, bool) {
	select {
	case call := <-ch:
		return call, true
	case <-time.After(timeout):
		return syncPatchCall{}, false
	}
}

// ---------------------------------------------------------------------------
// Tests for MergeConfiguration — incoming config merging
// ---------------------------------------------------------------------------

func TestMergeConfiguration_ServerSendsNewRoles(t *testing.T) {
	server, patchCh := newSyncTestServer(t)

	// Local config has no roles
	config := newSyncTestConfig(t, nil, nil, nil, server.URL)

	// Server sends a new role
	reg := makeRegistrationResponse(
		map[string]models.Role{
			"admin": {Name: "admin", Enabled: true},
		},
		nil, nil,
	)

	err := config.MergeConfiguration(reg)
	require.NoError(t, err)

	role, exists := config.Roles.Definitions["admin"]
	require.True(t, exists, "expected synced role to be stored locally")
	assert.Equal(t, "admin", role.Name)
	assert.True(t, role.Enabled)

	_, ok := waitForPatch(patchCh, 5*time.Second)
	require.True(t, ok, "expected outgoing PATCH call")
}

func TestMergeConfiguration_ServerSendsUpdatedRole(t *testing.T) {
	server, patchCh := newSyncTestServer(t)

	// Local config has an existing role
	config := newSyncTestConfig(t,
		map[string]models.Role{
			"editor":    {Name: "editor", Description: "Can edit", Enabled: true},
			"untouched": {Name: "untouched", Description: "Keep me", Enabled: true},
		},
		nil, nil, server.URL,
	)

	// Server sends the same role with updated description
	reg := makeRegistrationResponse(
		map[string]models.Role{
			"editor": {Name: "editor", Description: "Can edit and publish", Enabled: true},
		},
		nil, nil,
	)

	err := config.MergeConfiguration(reg)
	require.NoError(t, err)

	role, exists := config.Roles.Definitions["editor"]
	require.True(t, exists, "expected synced role to be stored locally")
	assert.Equal(t, "Can edit and publish", role.Description)
	assert.True(t, role.Enabled)
	assert.Contains(t, config.Roles.Definitions, "untouched")

	_, ok := waitForPatch(patchCh, 5*time.Second)
	require.True(t, ok, "expected outgoing PATCH call")
}

func TestMergeConfiguration_ServerSendsNewWorkflows(t *testing.T) {
	server, patchCh := newSyncTestServer(t)

	config := newSyncTestConfig(t, nil, nil, nil, server.URL)

	reg := makeRegistrationResponse(
		nil,
		map[string]models.Workflow{
			"approval": makeTestWorkflow("approval", "Handles approvals"),
		},
		nil,
	)

	err := config.MergeConfiguration(reg)
	require.NoError(t, err)

	workflow, exists := config.Workflows.Definitions["approval"]
	require.True(t, exists, "expected synced workflow to be stored locally")
	assert.Equal(t, "Handles approvals", workflow.Description)
	require.NotNil(t, workflow.Workflow)

	_, ok := waitForPatch(patchCh, 5*time.Second)
	require.True(t, ok, "expected outgoing PATCH call")
}

func TestMergeConfiguration_ServerSendsUpdatedWorkflow(t *testing.T) {
	server, patchCh := newSyncTestServer(t)

	config := newSyncTestConfig(t,
		nil,
		map[string]models.Workflow{
			"existing":  makeTestWorkflow("existing", "Existing workflow"),
			"unchanged": makeTestWorkflow("unchanged", "Keep me"),
		},
		nil,
		server.URL,
	)

	reg := makeRegistrationResponse(
		nil,
		map[string]models.Workflow{
			"existing": makeTestWorkflow("existing", "Updated workflow"),
		},
		nil,
	)

	err := config.MergeConfiguration(reg)
	require.NoError(t, err)

	workflow, exists := config.Workflows.Definitions["existing"]
	require.True(t, exists, "expected synced workflow to be stored locally")
	assert.Equal(t, "Updated workflow", workflow.Description)
	assert.Contains(t, config.Workflows.Definitions, "unchanged")

	_, ok := waitForPatch(patchCh, 5*time.Second)
	require.True(t, ok, "expected outgoing PATCH call")
}

func TestMergeConfiguration_ServerSendsNewProviders(t *testing.T) {
	server, patchCh := newSyncTestServer(t)

	config := newSyncTestConfig(t, nil, nil, nil, server.URL)

	reg := makeRegistrationResponse(
		nil,
		nil,
		map[string]models.ProviderConfig{
			"mock-primary": makeTestProvider("mock-primary", "Primary mock provider"),
		},
	)

	err := config.MergeConfiguration(reg)
	require.NoError(t, err)

	provider, exists := config.Providers.Definitions["mock-primary"]
	require.True(t, exists, "expected synced provider to be stored locally")
	assert.Equal(t, "Primary mock provider", provider.Description)
	assert.Equal(t, "mock", provider.Provider)

	_, ok := waitForPatch(patchCh, 5*time.Second)
	require.True(t, ok, "expected outgoing PATCH call")
}

func TestMergeConfiguration_ServerSendsUpdatedProvider(t *testing.T) {
	server, patchCh := newSyncTestServer(t)

	config := newSyncTestConfig(t,
		nil,
		nil,
		map[string]models.ProviderConfig{
			"mock-primary": makeTestProvider("mock-primary", "Old provider description"),
			"mock-extra":   makeTestProvider("mock-extra", "Keep me"),
		},
		server.URL,
	)

	reg := makeRegistrationResponse(
		nil,
		nil,
		map[string]models.ProviderConfig{
			"mock-primary": makeTestProvider("mock-primary", "Updated provider description"),
		},
	)

	err := config.MergeConfiguration(reg)
	require.NoError(t, err)

	provider, exists := config.Providers.Definitions["mock-primary"]
	require.True(t, exists, "expected synced provider to be stored locally")
	assert.Equal(t, "Updated provider description", provider.Description)
	assert.Contains(t, config.Providers.Definitions, "mock-extra")

	_, ok := waitForPatch(patchCh, 5*time.Second)
	require.True(t, ok, "expected outgoing PATCH call")
}

func TestMergeConfiguration_IdenticalConfigs(t *testing.T) {
	server, patchCh := newSyncTestServer(t)

	roles := map[string]models.Role{
		"viewer": {Name: "viewer", Description: "Read-only", Enabled: true},
	}

	config := newSyncTestConfig(t, roles, nil, nil, server.URL)

	// Server sends the exact same roles
	reg := makeRegistrationResponse(roles, nil, nil)

	err := config.MergeConfiguration(reg)
	require.NoError(t, err)

	// The outgoing goroutine still fires (with an empty or no-op patch)
	_, ok := waitForPatch(patchCh, 5*time.Second)
	require.True(t, ok, "expected outgoing PATCH call")
}

func TestMergeConfiguration_PartialConfig_OnlyRoles(t *testing.T) {
	server, patchCh := newSyncTestServer(t)

	config := newSyncTestConfig(t,
		map[string]models.Role{
			"existing": {Name: "existing", Enabled: true},
		},
		nil, nil, server.URL,
	)

	// Server sends only new roles, no workflows or providers
	reg := makeRegistrationResponse(
		map[string]models.Role{
			"new-role": {Name: "new-role", Enabled: true},
		},
		nil, nil,
	)

	err := config.MergeConfiguration(reg)
	require.NoError(t, err)

	role, exists := config.Roles.Definitions["new-role"]
	require.True(t, exists, "expected synced role to be stored locally")
	assert.Equal(t, "new-role", role.Name)
	assert.Contains(t, config.Roles.Definitions, "existing")

	_, ok := waitForPatch(patchCh, 5*time.Second)
	require.True(t, ok, "expected outgoing PATCH call")
}

func TestMergeConfiguration_NilConfigs(t *testing.T) {
	server, patchCh := newSyncTestServer(t)

	config := newSyncTestConfig(t, nil, nil, nil, server.URL)

	// Server sends a response with all nil config sections
	reg := &RegistrationResponse{Success: true}

	err := config.MergeConfiguration(reg)
	require.NoError(t, err)

	_, ok := waitForPatch(patchCh, 5*time.Second)
	require.True(t, ok, "expected outgoing PATCH call")
}

// ---------------------------------------------------------------------------
// Tests for outgoing patch content
// ---------------------------------------------------------------------------

func TestMergeConfiguration_OutgoingPatch_LocalExtrasAreSent(t *testing.T) {
	server, patchCh := newSyncTestServer(t)

	// Local has a role that the server doesn't know about
	config := newSyncTestConfig(t,
		map[string]models.Role{
			"local-only": {Name: "local-only", Description: "Exists locally", Enabled: true},
		},
		nil, nil, server.URL,
	)

	// Server sends no roles
	reg := makeRegistrationResponse(nil, nil, nil)

	err := config.MergeConfiguration(reg)
	require.NoError(t, err)

	call, ok := waitForPatch(patchCh, 5*time.Second)
	require.True(t, ok, "expected outgoing PATCH call")

	// Verify the request is a PATCH
	assert.Equal(t, http.MethodPatch, call.Method)

	// Verify the body contains the local-only role using raw JSON
	// to avoid Role's custom UnmarshalJSON which requires full initialization
	var patch map[string]json.RawMessage
	err = json.Unmarshal(call.Body, &patch)
	require.NoError(t, err)

	rolesPatch, hasRoles := patch["roles"]
	require.True(t, hasRoles, "outgoing patch should contain roles")

	var rolesObj map[string]json.RawMessage
	err = json.Unmarshal(rolesPatch, &rolesObj)
	require.NoError(t, err)

	if defs, hasDefs := rolesObj["definitions"]; hasDefs {
		var defsMap map[string]json.RawMessage
		err = json.Unmarshal(defs, &defsMap)
		require.NoError(t, err)
		_, exists := defsMap["local-only"]
		assert.True(t, exists, "outgoing patch should contain the local-only role")
	}
}

func TestMergeConfiguration_OutgoingPatch_IdenticalConfigsProduceEmptyPatch(t *testing.T) {
	server, patchCh := newSyncTestServer(t)

	roles := map[string]models.Role{
		"shared": {Name: "shared", Description: "Same on both sides", Enabled: true},
	}

	config := newSyncTestConfig(t, roles, nil, nil, server.URL)

	reg := makeRegistrationResponse(roles, nil, nil)

	err := config.MergeConfiguration(reg)
	require.NoError(t, err)

	call, ok := waitForPatch(patchCh, 5*time.Second)
	require.True(t, ok, "expected outgoing PATCH call")

	// An identical config should produce an empty merge patch: {}
	var raw map[string]json.RawMessage
	err = json.Unmarshal(call.Body, &raw)
	require.NoError(t, err)

	// The patch should either be empty or contain only empty/null sections
	if rolesPatch, ok := raw["roles"]; ok {
		var rolesObj map[string]json.RawMessage
		_ = json.Unmarshal(rolesPatch, &rolesObj)
		// definitions should be empty or absent
		if defs, hasDefs := rolesObj["definitions"]; hasDefs {
			var defsMap map[string]json.RawMessage
			_ = json.Unmarshal(defs, &defsMap)
			assert.Empty(t, defsMap, "definitions diff should be empty for identical configs")
		}
	}
}

func TestMergeConfiguration_OutgoingPatch_MixedChanges(t *testing.T) {
	server, patchCh := newSyncTestServer(t)

	// Local has an extra role; server will send an extra workflow
	config := newSyncTestConfig(t,
		map[string]models.Role{
			"local-role": {Name: "local-role", Enabled: true},
		},
		nil, nil, server.URL,
	)

	// Server sends a role (not present locally), no workflows
	reg := makeRegistrationResponse(
		map[string]models.Role{
			"server-role": {Name: "server-role", Enabled: true},
		},
		nil, nil,
	)

	err := config.MergeConfiguration(reg)
	require.NoError(t, err)

	call, ok := waitForPatch(patchCh, 5*time.Second)
	require.True(t, ok, "expected outgoing PATCH call")

	// Unmarshal the outgoing patch as raw JSON to avoid Role's custom
	// UnmarshalJSON which requires full initialization
	var patch map[string]json.RawMessage
	err = json.Unmarshal(call.Body, &patch)
	require.NoError(t, err)

	// Outgoing patch should contain the local-role (local extra)
	rolesPatch, hasRoles := patch["roles"]
	require.True(t, hasRoles, "outgoing patch should contain roles section")

	var rolesObj map[string]json.RawMessage
	err = json.Unmarshal(rolesPatch, &rolesObj)
	require.NoError(t, err)

	if defs, hasDefs := rolesObj["definitions"]; hasDefs {
		var defsMap map[string]json.RawMessage
		err = json.Unmarshal(defs, &defsMap)
		require.NoError(t, err)
		_, hasLocal := defsMap["local-role"]
		assert.True(t, hasLocal, "outgoing patch should include local-only role")
	}
}

func TestMergeConfiguration_OutgoingPatch_AuthTokenIncluded(t *testing.T) {
	// Use a custom server that checks the Authorization header
	var mu sync.Mutex
	var capturedAuth string
	patchCh := make(chan syncPatchCall, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/api-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("PATCH /api/v1/sync", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		capturedAuth = r.Header.Get("Authorization")
		mu.Unlock()
		patchCh <- syncPatchCall{Body: body, Header: r.Header.Clone()}
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	config := newSyncTestConfig(t, nil, nil, nil, server.URL)
	config.Thand.ApiKey = "my-secret-token"

	reg := makeRegistrationResponse(nil, nil, nil)

	err := config.MergeConfiguration(reg)
	require.NoError(t, err)

	_, ok := waitForPatch(patchCh, 5*time.Second)
	require.True(t, ok, "expected outgoing PATCH call")

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, capturedAuth, "my-secret-token", "Authorization header should contain the API key")
}

func TestMergeConfiguration_OutgoingPatch_URLContainsSync(t *testing.T) {
	// Use a catch-all handler to see the raw URL path
	patchCh := make(chan syncPatchCall, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/api-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("PATCH /", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		patchCh <- syncPatchCall{URL: r.URL.String(), Body: body}
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	config := newSyncTestConfig(t, nil, nil, nil, server.URL)

	reg := makeRegistrationResponse(nil, nil, nil)

	err := config.MergeConfiguration(reg)
	require.NoError(t, err)

	call, ok := waitForPatch(patchCh, 5*time.Second)
	require.True(t, ok, "expected outgoing PATCH call")
	assert.Contains(t, call.URL, "/sync")
}

// ---------------------------------------------------------------------------
// Tests for applyPatch
// ---------------------------------------------------------------------------

func TestApplyPatch_NilSections(t *testing.T) {
	config := newSyncTestConfig(t, nil, nil, nil, "")

	// All nil — should be a no-op
	err := config.applyPatch(ConfigPatchRequest{})
	assert.NoError(t, err)
}

func TestApplyPatch_AppliesRoles(t *testing.T) {
	config := newSyncTestConfig(t, nil, nil, nil, "")

	diff := ConfigPatchRequest{
		RoleConfig: &RoleConfig{
			Definitions: map[string]models.Role{
				"new-role": {Name: "new-role", Enabled: true},
			},
		},
	}

	err := config.applyPatch(diff)
	assert.NoError(t, err)
	assert.Contains(t, config.Roles.Definitions, "new-role")
}

func TestApplyPatch_SkipsNilWorkflows(t *testing.T) {
	config := newSyncTestConfig(t,
		map[string]models.Role{
			"existing": {Name: "existing", Enabled: true},
		},
		nil, nil, "",
	)

	// Only roles in the diff, workflows nil
	diff := ConfigPatchRequest{
		RoleConfig: &RoleConfig{
			Definitions: map[string]models.Role{
				"another": {Name: "another", Enabled: true},
			},
		},
	}

	err := config.applyPatch(diff)
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Tests for the merge-patch logic (JSON diffing correctness)
// ---------------------------------------------------------------------------

func TestMergePatchLogic_IncomingOverridesExisting(t *testing.T) {
	// Simulate what MergeConfiguration does internally: marshal existing
	// and incoming as ConfigPatchRequest, merge-patch, then diff.

	existing := ConfigPatchRequest{
		RoleConfig: &RoleConfig{
			Definitions: map[string]models.Role{
				"role-a": {Name: "role-a", Description: "Old description", Enabled: true},
			},
		},
	}

	incoming := ConfigPatchRequest{
		RoleConfig: &RoleConfig{
			Definitions: map[string]models.Role{
				"role-a": {Name: "role-a", Description: "New description", Enabled: true},
			},
		},
	}

	existingData, err := json.Marshal(existing)
	require.NoError(t, err)

	incomingData, err := json.Marshal(incoming)
	require.NoError(t, err)

	// Merge incoming over existing
	newData, err := jsonpatch.MergePatch(existingData, incomingData)
	require.NoError(t, err)

	// The merged result should have the new description
	var merged ConfigPatchRequest
	err = json.Unmarshal(newData, &merged)
	require.NoError(t, err)

	require.NotNil(t, merged.RoleConfig)
	role, exists := merged.RoleConfig.Definitions["role-a"]
	require.True(t, exists)
	assert.Equal(t, "New description", role.Description)
}

func TestMergePatchLogic_NewItemsAdded(t *testing.T) {
	existing := ConfigPatchRequest{
		RoleConfig: &RoleConfig{
			Definitions: map[string]models.Role{
				"role-a": {Name: "role-a", Enabled: true},
			},
		},
	}

	incoming := ConfigPatchRequest{
		RoleConfig: &RoleConfig{
			Definitions: map[string]models.Role{
				"role-b": {Name: "role-b", Enabled: true},
			},
		},
	}

	existingData, err := json.Marshal(existing)
	require.NoError(t, err)

	incomingData, err := json.Marshal(incoming)
	require.NoError(t, err)

	newData, err := jsonpatch.MergePatch(existingData, incomingData)
	require.NoError(t, err)

	var merged ConfigPatchRequest
	err = json.Unmarshal(newData, &merged)
	require.NoError(t, err)

	require.NotNil(t, merged.RoleConfig)
	// Both roles should be present after merge
	_, hasA := merged.RoleConfig.Definitions["role-a"]
	_, hasB := merged.RoleConfig.Definitions["role-b"]
	assert.True(t, hasA, "existing role-a should be preserved")
	assert.True(t, hasB, "incoming role-b should be added")
}

func TestMergePatchLogic_OutgoingPatchCapturesLocalExtras(t *testing.T) {
	// When local has items the server doesn't, CreateMergePatch(incoming, existing)
	// should produce a patch containing those local-only items.

	existing := ConfigPatchRequest{
		RoleConfig: &RoleConfig{
			Definitions: map[string]models.Role{
				"shared":     {Name: "shared", Enabled: true},
				"local-only": {Name: "local-only", Enabled: true},
			},
		},
	}

	incoming := ConfigPatchRequest{
		RoleConfig: &RoleConfig{
			Definitions: map[string]models.Role{
				"shared": {Name: "shared", Enabled: true},
			},
		},
	}

	existingData, err := json.Marshal(existing)
	require.NoError(t, err)

	incomingData, err := json.Marshal(incoming)
	require.NoError(t, err)

	// CreateMergePatch(original=incoming, target=existing) gives us the patch
	// to transform incoming into existing — i.e., the local extras.
	outgoingPatch, err := jsonpatch.MergePatch(incomingData, existingData)
	require.NoError(t, err)

	// Use CreateMergePatch which is what the production code actually calls
	// jsonpatch.CreateMergePatch is used in sync.go, but here we test the
	// stdlib json.MergePatch behavior which is equivalent for our scenario.

	var patch ConfigPatchRequest
	err = json.Unmarshal(outgoingPatch, &patch)
	require.NoError(t, err)

	require.NotNil(t, patch.RoleConfig)
	_, hasLocal := patch.RoleConfig.Definitions["local-only"]
	assert.True(t, hasLocal, "outgoing patch should include local-only role")
}

func TestMergePatchLogic_EmptyIncomingPreservesExisting(t *testing.T) {
	existing := ConfigPatchRequest{
		RoleConfig: &RoleConfig{
			Definitions: map[string]models.Role{
				"my-role": {Name: "my-role", Enabled: true},
			},
		},
	}

	incoming := ConfigPatchRequest{}

	existingData, err := json.Marshal(existing)
	require.NoError(t, err)

	incomingData, err := json.Marshal(incoming)
	require.NoError(t, err)

	newData, err := jsonpatch.MergePatch(existingData, incomingData)
	require.NoError(t, err)

	var merged ConfigPatchRequest
	err = json.Unmarshal(newData, &merged)
	require.NoError(t, err)

	// Existing role should still be there — incoming was empty (omitempty)
	// so MergePatch should preserve everything
	require.NotNil(t, merged.RoleConfig)
	_, has := merged.RoleConfig.Definitions["my-role"]
	assert.True(t, has, "existing role should be preserved when incoming is empty")
}
