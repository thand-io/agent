package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers/aws"
)

// newNotReadyMockProvider creates a mock AWS provider that has been
// initialized (so it satisfies the Provider interface and has role data)
// but has NOT been marked ready. This simulates the window between provider
// creation and synchronization completion.
func newNotReadyMockProvider(t *testing.T, name string) models.Provider {
	t.Helper()
	p := aws.NewMockAwsProvider()
	err := p.Initialize(name, models.ProviderConfig{
		Name:     name,
		Provider: "aws",
		Enabled:  true,
	})
	require.NoError(t, err)
	// Deliberately NOT calling p.SetReady()
	return p
}

// newReadyMockProvider creates a mock AWS provider that is fully initialized
// and marked ready.
func newReadyMockProvider(t *testing.T, name string) models.Provider {
	t.Helper()
	p := newNotReadyMockProvider(t, name)
	p.SetReady()
	return p
}

// newReadinessTestConfig creates a Config with the given providers already
// added to providerInstances. Providers are NOT marked ready unless the caller
// does so explicitly.
func newReadinessTestConfig(providers map[string]models.Provider) *Config {
	return &Config{
		mode:              ModeServer,
		providerInstances: providers,
	}
}

// ---------------------------------------------------------------------------
// awaitProviderReadiness
// ---------------------------------------------------------------------------

func TestAwaitProviderReadiness_AllReady(t *testing.T) {
	p1 := newReadyMockProvider(t, "aws-prod")
	p2 := newReadyMockProvider(t, "gcp-prod")

	cfg := newReadinessTestConfig(map[string]models.Provider{
		"aws-prod": p1,
		"gcp-prod": p2,
	})

	// Should return almost instantly — both already ready
	done := make(chan struct{})
	go func() {
		cfg.awaitProviderReadiness()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("awaitProviderReadiness should return immediately when all providers are ready")
	}
}

func TestAwaitProviderReadiness_NoProviders(t *testing.T) {
	cfg := newReadinessTestConfig(nil)

	// Should return immediately with no providers
	done := make(chan struct{})
	go func() {
		cfg.awaitProviderReadiness()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("awaitProviderReadiness should return immediately with no providers")
	}
}

func TestAwaitProviderReadiness_WaitsForPending(t *testing.T) {
	p1 := newReadyMockProvider(t, "aws-prod")
	p2 := newNotReadyMockProvider(t, "aws-staging")
	p2.SetPending() // pending but not ready

	cfg := newReadinessTestConfig(map[string]models.Provider{
		"aws-prod":    p1,
		"aws-staging": p2,
	})

	done := make(chan struct{})
	go func() {
		cfg.awaitProviderReadiness()
		close(done)
	}()

	// Should NOT be done yet — p2 is pending
	select {
	case <-done:
		t.Fatal("awaitProviderReadiness returned before all providers were ready")
	case <-time.After(100 * time.Millisecond):
		// expected
	}

	// Now mark p2 ready
	p2.SetReady()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("awaitProviderReadiness did not return after all providers became ready")
	}
}

func TestAwaitProviderReadiness_DelayedReady(t *testing.T) {
	p := newNotReadyMockProvider(t, "aws-prod")

	cfg := newReadinessTestConfig(map[string]models.Provider{
		"aws-prod": p,
	})

	done := make(chan struct{})
	go func() {
		cfg.awaitProviderReadiness()
		close(done)
	}()

	// Simulate async sync completing after a delay
	time.AfterFunc(200*time.Millisecond, func() {
		p.SetReady()
	})

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("awaitProviderReadiness did not return after provider became ready")
	}
}

// ---------------------------------------------------------------------------
// anyProviderNotReady
// ---------------------------------------------------------------------------

func TestAnyProviderNotReady_AllReady(t *testing.T) {
	p1 := newReadyMockProvider(t, "aws-prod")
	p2 := newReadyMockProvider(t, "aws-staging")

	cfg := newReadinessTestConfig(map[string]models.Provider{
		"aws-prod":    p1,
		"aws-staging": p2,
	})

	assert.False(t, cfg.anyProviderNotReady([]string{"aws-prod", "aws-staging"}))
}

func TestAnyProviderNotReady_OneNotReady(t *testing.T) {
	p1 := newReadyMockProvider(t, "aws-prod")
	p2 := newNotReadyMockProvider(t, "aws-staging")

	cfg := newReadinessTestConfig(map[string]models.Provider{
		"aws-prod":    p1,
		"aws-staging": p2,
	})

	assert.True(t, cfg.anyProviderNotReady([]string{"aws-prod", "aws-staging"}))
}

func TestAnyProviderNotReady_ProviderNotFound(t *testing.T) {
	p1 := newReadyMockProvider(t, "aws-prod")

	cfg := newReadinessTestConfig(map[string]models.Provider{
		"aws-prod": p1,
	})

	// Unknown provider names should be skipped, not cause a false positive
	assert.False(t, cfg.anyProviderNotReady([]string{"does-not-exist"}))
}

func TestAnyProviderNotReady_EmptyList(t *testing.T) {
	cfg := newReadinessTestConfig(nil)
	assert.False(t, cfg.anyProviderNotReady(nil))
	assert.False(t, cfg.anyProviderNotReady([]string{}))
}

func TestAnyProviderNotReady_OnlyUnknownProviders(t *testing.T) {
	p1 := newNotReadyMockProvider(t, "aws-prod")
	cfg := newReadinessTestConfig(map[string]models.Provider{
		"aws-prod": p1,
	})

	// Only asking about providers that don't exist — should return false
	assert.False(t, cfg.anyProviderNotReady([]string{"unknown1", "unknown2"}))
}

// ---------------------------------------------------------------------------
// ReloadRoleIndexes with provider readiness
// ---------------------------------------------------------------------------

func TestReloadRoleIndexes_WaitsForProviders(t *testing.T) {
	providers := map[string]models.ProviderConfig{
		"aws-prod": {
			Name:     "aws-prod",
			Provider: "aws",
		},
	}

	roles := map[string]models.Role{
		"admin": {
			Name:        "Admin",
			Description: "Admin role with AWS managed policy",
			Inherits: []string{
				"arn:aws:iam::aws:policy/AdministratorAccess",
			},
			Providers: []string{"aws-prod"},
			Enabled:   true,
		},
	}

	// newTestConfig marks providers ready immediately
	cfg := newTestConfig(t, roles, providers)

	err := cfg.ReloadRoleIndexes()
	require.NoError(t, err, "ReloadRoleIndexes should succeed when providers are ready")
}

// ---------------------------------------------------------------------------
// resolveCompositeRole with not-ready providers
// ---------------------------------------------------------------------------

func TestCompositeRole_SkipsInheritedRoleWhenProviderNotReady(t *testing.T) {
	// Create config with a NOT-ready provider to simulate the race condition.
	// Use a real mock so the Provider interface is satisfied, but don't mark ready.
	awsProvider := newNotReadyMockProvider(t, "aws-prod")

	// Use a role name that doesn't exist in the mock's loaded role set.
	// This simulates a provider that hasn't finished syncing its roles yet:
	// the lookup will fail, and the readiness check should cause a graceful skip.
	roles := map[string]models.Role{
		"admin": {
			Name:        "Admin",
			Description: "Admin role inheriting an AWS managed policy",
			Inherits: []string{
				"arn:aws:iam::aws:policy/NotYetLoadedPolicy",
			},
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{{
					Operations: []string{"ec2:*"},
				}},
			},
			Providers: []string{"aws-prod"},
			Enabled:   true,
		},
	}

	cfg := &Config{
		mode: ModeServer,
		Roles: RoleConfig{
			Definitions: roles,
		},
	}
	cfg.AddProvider("aws-prod", awsProvider)

	identity := &models.Identity{
		ID:   "test-user",
		User: &models.User{Username: "test", Email: "test@example.com"},
	}

	// When provider is not ready and the role isn't found, the inherited
	// ARN should be skipped gracefully instead of producing an error.
	result, err := cfg.GetCompositeRoleByName(identity, "admin")
	require.NoError(t, err, "Should not error when provider is not ready — ARN should be skipped gracefully")
	require.NotNil(t, result)

	// The custom permissions should still be present
	assert.NotEmpty(t, result.Permissions.Allow, "Custom permissions should still be present")

	// The unresolved ARN should NOT appear in the inherits list
	for _, inherit := range result.Inherits {
		assert.NotContains(t, inherit, "NotYetLoadedPolicy",
			"Unresolved ARN should be skipped, not included in composite role")
	}
}

func TestCompositeRole_ResolvesInheritedRoleWhenProviderReady(t *testing.T) {
	providers := map[string]models.ProviderConfig{
		"aws-prod": {
			Name:     "aws-prod",
			Provider: "aws",
		},
	}

	roles := map[string]models.Role{
		"admin": {
			Name:        "Admin",
			Description: "Admin role inheriting an AWS managed policy",
			Inherits: []string{
				"arn:aws:iam::aws:policy/AdministratorAccess",
			},
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{{
					Operations: []string{"ec2:*"},
				}},
			},
			Providers: []string{"aws-prod"},
			Enabled:   true,
		},
	}

	// newTestConfig marks providers ready, so the ARN should resolve
	cfg := newTestConfig(t, roles, providers)

	identity := &models.Identity{
		ID:   "test-user",
		User: &models.User{Username: "test", Email: "test@example.com"},
	}

	result, err := cfg.GetCompositeRoleByName(identity, "admin")
	require.NoError(t, err)
	require.NotNil(t, result)

	// The managed policy should be resolved into Inherits
	assert.Contains(t, result.Inherits, "AdministratorAccess",
		"Managed policy should be resolved when provider is ready")
}
