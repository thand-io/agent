package thand

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/testing/temporaltest"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

type executionPlanTestProvider struct {
	*models.BaseProvider
}

func (p *executionPlanTestProvider) ValidateRole(
	ctx context.Context,
	user *models.Identity,
	role *models.Role,
) (map[string]any, error) {
	return map[string]any{}, nil
}

func newExecutionPlanTestProvider(identifier string) *executionPlanTestProvider {
	caps := models.NewProviderCapabilities().WithDefaultProvisioningConfiguration()
	providerCfg := models.ProviderConfig{
		Name:         identifier,
		Provider:     identifier,
		Enabled:      true,
		Capabilities: caps,
		Config:       &models.BasicConfig{},
	}

	provider := &executionPlanTestProvider{
		BaseProvider: models.NewBaseProvider(identifier, providerCfg, caps),
	}
	provider.SetReady()
	return provider
}

type executionPlanTestConfig struct {
	*config.Config
	identities map[string]*models.Identity
}

func (s *executionPlanTestConfig) GetIdentity(byEmail string) (*models.Identity, error) {
	if identity, ok := s.identities[byEmail]; ok {
		return identity, nil
	}
	return nil, fmt.Errorf("identity %q not found", byEmail)
}

func newExecutionPlanWorkflowTask(
	t *testing.T,
	workflowID string,
	req models.ElevateRequestInternal,
) *models.ElevateWorkflowTask {
	t.Helper()

	contextMap := map[string]any{}
	require.NoError(t, common.ConvertInterfaceToInterface(req, &contextMap))

	return models.NewElevateWorkflowTask(&sdkWorkflowsModel.WorkflowTask{
		WorkflowID: workflowID,
		Context:    contextMap,
		Input:      map[string]any{},
		Output:     map[string]any{},
	})
}

func newExecutionPlanRole(identifier, name string) *models.Role {
	return &models.Role{
		Identifier: identifier,
		Name:       name,
		Enabled:    true,
		Permissions: models.RolePermissions{
			Allow: models.RoleStatements{
				{
					Operations: []string{"local:test"},
				},
			},
		},
	}
}

func newExecutionPlanTestConfig(t *testing.T) *executionPlanTestConfig {
	t.Helper()

	cfg := &executionPlanTestConfig{
		Config: config.DefaultConfig(),
		identities: map[string]*models.Identity{
			"user@example.com": {
				ID: "user@example.com",
				User: &models.User{
					Email:    "user@example.com",
					Username: "example-user",
				},
			},
			"second@example.com": {
				ID: "second@example.com",
				User: &models.User{
					Email:    "second@example.com",
					Username: "second-user",
				},
			},
		},
	}

	cfg.AddProvider("test-provider", newExecutionPlanTestProvider("test-provider"))
	cfg.AddProvider("local-elevation", newExecutionPlanTestProvider("local"))
	return cfg
}

func TestBuildExecutionPlanMaterializesAuthorizeRequests(t *testing.T) {
	cfg := newExecutionPlanTestConfig(t)
	req := models.ElevateRequestInternal{
		ElevateRequest: models.ElevateRequest{
			Role:       newExecutionPlanRole("admin", "Admin"),
			Providers:  []string{"test-provider"},
			Workflow:   "aws_simple_elevation",
			Reason:     "maintenance",
			Duration:   "30m",
			Identities: []string{"user@example.com"},
		},
	}

	plan, err := config.BuildExecutionPlan(cfg, "wf-execution-plan", &req)
	require.NoError(t, err)
	require.Len(t, plan.Entries, 1)

	entry := plan.Entries[0]
	require.NotEmpty(t, entry.EntryID)
	assert.Equal(t, "test-provider", entry.ProviderName)
	assert.Empty(t, entry.DeviceID)
	require.NotNil(t, entry.AuthorizeRequest)
	require.NotNil(t, entry.AuthorizeRequest.Identity)
	assert.Equal(t, "user@example.com", entry.AuthorizeRequest.Identity.User.Email)
	assert.Equal(t, "admin", entry.AuthorizeRequest.Role.GetIdentifier())
}

func TestBuildExecutionPlanCarriesCanonicalDeviceID(t *testing.T) {
	cfg := newExecutionPlanTestConfig(t)
	cfg.Devices.Definitions = map[string]models.Device{
		"device-alpha": {
			ID:      "device-alpha",
			Name:    "Device Alpha",
			Enabled: true,
		},
	}

	req := models.ElevateRequestInternal{
		ElevateRequest: models.ElevateRequest{
			Role:       newExecutionPlanRole("admin", "Admin"),
			Providers:  []string{"test-provider"},
			Workflow:   "aws_simple_elevation",
			Device:     "device-alpha",
			Reason:     "maintenance",
			Duration:   "30m",
			Identities: []string{"user@example.com", "second@example.com"},
		},
	}

	plan, err := config.BuildExecutionPlan(cfg, "wf-local-sudo", &req)
	require.NoError(t, err)
	require.Len(t, plan.Entries, 2)

	assert.Equal(t, "device-alpha", plan.Entries[0].DeviceID)
	assert.Equal(t, "device-alpha", plan.Entries[1].DeviceID)
	assert.NotEqual(t, plan.Entries[0].EntryID, plan.Entries[1].EntryID)
	assert.NotNil(t, plan.Entries[0].AuthorizeRequest)
	assert.NotNil(t, plan.Entries[1].AuthorizeRequest)
}

func TestEnsureExecutionPlanTemporalBuildsOnceAndCachesPlan(t *testing.T) {
	// TestWorkflowEnvironment does not thread WorkerOptions.BuildID through the
	// lazy activity-worker path, so seed the SDK's process-wide checksum cache
	// before the first ExecuteActivity to keep binary hashing out of the
	// workflow deadlock-detector critical path.
	temporaltest.SeedBinaryChecksum()

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	request := models.ElevateRequestInternal{
		ElevateRequest: models.ElevateRequest{
			Role:       newExecutionPlanRole("admin", "Admin"),
			Providers:  []string{"test-provider"},
			Workflow:   "aws_simple_elevation",
			Reason:     "maintenance",
			Duration:   "30m",
			Identities: []string{"user@example.com"},
		},
	}

	var activityCalls int
	env.RegisterActivityWithOptions(
		func(context.Context, models.ExecutionPlanRequest) (*models.ExecutionPlan, error) {
			activityCalls++
			return &models.ExecutionPlan{
				WorkflowName: request.GetWorkflow(),
				Entries: []models.ExecutionPlanEntry{{
					EntryID:      "entry-1",
					ProviderName: "test-provider",
					AuthorizeRequest: &models.AuthorizeRoleRequest{
						Identity: &models.Identity{
							ID:   "user@example.com",
							User: &models.User{Email: "user@example.com"},
						},
						Role: &models.CompositeRole{
							Role:      *newExecutionPlanRole("admin", "Admin"),
							Composite: false,
						},
						Duration: func() *time.Duration {
							d := 30 * time.Minute
							return &d
						}(),
					},
				}},
			}, nil
		},
		activity.RegisterOptions{Name: models.TemporalBuildExecutionPlanActivityName},
	)

	// Prebuild the workflow task outside the workflow closure so this test is
	// measuring execution-plan caching, not request serialization before the
	// first Temporal yield.
	task := newExecutionPlanWorkflowTask(t, "wf-temporal-plan", request)

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		task.WithTemporalContext(ctx)

		runner := &thandTask{}
		firstPlan, err := runner.ensureExecutionPlan(task, &request)
		if err != nil {
			return err
		}
		secondPlan, err := runner.ensureExecutionPlan(task, &request)
		if err != nil {
			return err
		}
		if firstPlan == nil || secondPlan == nil {
			return fmt.Errorf("execution plan was not returned")
		}
		if firstPlan.Entries[0].EntryID != secondPlan.Entries[0].EntryID {
			return fmt.Errorf("execution plan was not reused from workflow context")
		}
		return nil
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	assert.Equal(t, 1, activityCalls)
}

func TestExecuteRevocationTaskRequiresRecordedExecutionPlan(t *testing.T) {
	cfg := newExecutionPlanTestConfig(t)
	req := models.ElevateRequestInternal{
		ElevateRequest: models.ElevateRequest{
			Role:       newExecutionPlanRole("admin", "Admin"),
			Providers:  []string{"test-provider"},
			Workflow:   "aws_simple_elevation",
			Reason:     "maintenance",
			Duration:   "30m",
			Identities: []string{"user@example.com"},
		},
	}

	task := newExecutionPlanWorkflowTask(t, "wf-revoke-without-plan", req)
	runner := &thandTask{config: cfg}

	output, err := runner.executeRevocationTask(task, "revoke", nil, &req, &RevokeTask{})
	require.Error(t, err)
	assert.Nil(t, output)
	assert.ErrorContains(t, err, "execution plan is missing")
}
