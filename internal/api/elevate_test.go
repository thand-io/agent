package api

import (
	"context"
	"errors"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

// ─── Mock types ──────────────────────────────────────────────────────────────

// mockRunner is a configurable WorkflowRunner double that records the last
// arguments it received for post-call assertions.
type mockRunner struct {
	createFn    func(context.Context, models.ElevateRequest) (*models.WorkflowRequest, error)
	resumeFn    func(*models.ElevateWorkflowTask) (*models.ElevateWorkflowTask, error)
	lastElevReq models.ElevateRequest
	lastResWf   *models.ElevateWorkflowTask
}

func (m *mockRunner) CreateElevationWorkflow(ctx context.Context, req models.ElevateRequest) (*models.WorkflowRequest, error) {
	m.lastElevReq = req
	if m.createFn != nil {
		return m.createFn(ctx, req)
	}
	return &models.WorkflowRequest{}, nil
}

func (m *mockRunner) ResumeWorkflow(wf *models.ElevateWorkflowTask) (*models.ElevateWorkflowTask, error) {
	m.lastResWf = wf
	if m.resumeFn != nil {
		return m.resumeFn(wf)
	}
	return wf, nil
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

// newTestService wires a Service with a real config.Config and a mock WorkflowRunner.
// config.DefaultConfig() provides safe zero-value implementations for all the
// ConfigImpl methods; SetMode controls the IsServer() result.
func newTestService(serverMode bool, runner *mockRunner) *Service {
	cfg := config.DefaultConfig()
	if serverMode {
		cfg.SetMode(config.ModeServer)
	} else {
		cfg.SetMode(config.ModeClient)
	}
	return &Service{cfg: cfg, workflows: runner}
}

// newElevateTask returns a minimal ElevateWorkflowTask with a plain map input.
func newElevateTask() *models.ElevateWorkflowTask {
	return models.NewElevateWorkflowTask(&sdkWorkflowsModel.WorkflowTask{
		WorkflowID: "wf-test-001",
		Input:      map[string]any{},
	})
}

// newElevateTaskWithCE returns an ElevateWorkflowTask whose Input is a valid
// CloudEvent so that GetInputAsCloudEvent() returns a non-nil value.
func newElevateTaskWithCE() *models.ElevateWorkflowTask {
	ev := cloudevents.NewEvent()
	ev.SetID("ce-test-id")
	ev.SetSource("test://source")
	ev.SetType("com.test.elevation.requested")
	ev.SetTime(time.Now())

	task := models.NewElevateWorkflowTask(&sdkWorkflowsModel.WorkflowTask{
		WorkflowID: "wf-test-002",
	})
	task.SetInput(ev)
	return task
}

// ─── Elevate tests ────────────────────────────────────────────────────────────

func TestElevate_NotServerMode(t *testing.T) {
	svc := newTestService(false, &mockRunner{})
	_, err := svc.Elevate(context.Background(), ElevationInput{
		Request: models.ElevateRequest{Workflow: "wf"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in server mode")
}

func TestElevate_MissingWorkflow(t *testing.T) {
	svc := newTestService(true, &mockRunner{})
	_, err := svc.Elevate(context.Background(), ElevationInput{
		Request: models.ElevateRequest{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no workflow specified")
}

func TestElevate_NilUser_NoSessionInjected(t *testing.T) {
	runner := &mockRunner{}
	svc := newTestService(true, runner)

	_, err := svc.Elevate(context.Background(), ElevationInput{
		Request: models.ElevateRequest{Workflow: "wf-simple"},
		User:    nil,
	})

	require.NoError(t, err)
	assert.Nil(t, runner.lastElevReq.Session, "session must remain nil when no User is supplied")
}

func TestElevate_UserWithEmail_DefaultsIdentity(t *testing.T) {
	runner := &mockRunner{}
	svc := newTestService(true, runner)
	user := &models.Session{
		User: &models.User{Email: "alice@example.com"},
	}

	_, err := svc.Elevate(context.Background(), ElevationInput{
		Request:      models.ElevateRequest{Workflow: "wf-self"},
		User:         user,
		AuthProvider: "oidc",
	})

	require.NoError(t, err)
	require.Len(t, runner.lastElevReq.Identities, 1)
	assert.Equal(t, "alice@example.com", runner.lastElevReq.Identities[0])
}

func TestElevate_UserWithEmail_ExplicitIdentitiesUnchanged(t *testing.T) {
	runner := &mockRunner{}
	svc := newTestService(true, runner)
	user := &models.Session{
		User: &models.User{Email: "alice@example.com"},
	}
	explicit := []string{"bob@example.com", "carol@example.com"}

	_, err := svc.Elevate(context.Background(), ElevationInput{
		Request:      models.ElevateRequest{Workflow: "wf-explicit", Identities: explicit},
		User:         user,
		AuthProvider: "oidc",
	})

	require.NoError(t, err)
	assert.Equal(t, explicit, runner.lastElevReq.Identities, "caller-supplied identities must not be overridden")
}

func TestElevate_UserNonNil_SessionPopulated(t *testing.T) {
	runner := &mockRunner{}
	svc := newTestService(true, runner)
	user := &models.Session{
		User: &models.User{Email: "dave@example.com"},
	}

	_, err := svc.Elevate(context.Background(), ElevationInput{
		Request:      models.ElevateRequest{Workflow: "wf-session"},
		User:         user,
		AuthProvider: "saml",
	})

	require.NoError(t, err)
	assert.NotNil(t, runner.lastElevReq.Session, "session must be populated from the authenticated user")
}

func TestElevate_RunnerError(t *testing.T) {
	sentinel := errors.New("temporal unavailable")
	runner := &mockRunner{
		createFn: func(_ context.Context, _ models.ElevateRequest) (*models.WorkflowRequest, error) {
			return nil, sentinel
		},
	}
	svc := newTestService(true, runner)

	_, err := svc.Elevate(context.Background(), ElevationInput{
		Request: models.ElevateRequest{Workflow: "wf-err"},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// ─── Resume tests ─────────────────────────────────────────────────────────────

func TestResume_NotServerMode(t *testing.T) {
	svc := newTestService(false, &mockRunner{})
	_, err := svc.Resume(context.Background(), ResumeInput{
		Workflow: newElevateTask(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in server mode")
}

func TestResume_NilWorkflow(t *testing.T) {
	svc := newTestService(true, &mockRunner{})
	_, err := svc.Resume(context.Background(), ResumeInput{Workflow: nil})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow task must not be nil")
}

func TestResume_RunnerReturnsNil_ErrWorkflowNotFound(t *testing.T) {
	runner := &mockRunner{
		resumeFn: func(_ *models.ElevateWorkflowTask) (*models.ElevateWorkflowTask, error) {
			return nil, nil
		},
	}
	svc := newTestService(true, runner)

	_, err := svc.Resume(context.Background(), ResumeInput{Workflow: newElevateTask()})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWorkflowNotFound)
}

func TestResume_NilUser_NoCEModification(t *testing.T) {
	task := newElevateTaskWithCE()
	require.NotNil(t, task.GetInputAsCloudEvent(), "CE input must be parseable before Resume")

	runner := &mockRunner{}
	svc := newTestService(true, runner)

	result, err := svc.Resume(context.Background(), ResumeInput{
		Workflow: task,
		User:     nil,
	})

	require.NoError(t, err)
	require.NotNil(t, result)

	ce := result.GetInputAsCloudEvent()
	require.NotNil(t, ce)
	exts := ce.Extensions()
	assert.Empty(t, exts["user"], "user extension must not be stamped when User is nil")
}

func TestResume_UserSet_WorkflowInputNotCE_NoStamping(t *testing.T) {
	// Input is a plain map — not a CloudEvent — so GetInputAsCloudEvent returns nil
	// and the stamping branch must be skipped entirely.
	task := newElevateTask()
	runner := &mockRunner{}
	svc := newTestService(true, runner)

	result, err := svc.Resume(context.Background(), ResumeInput{
		Workflow: task,
		User:     &models.User{Email: "eve@example.com"},
	})

	require.NoError(t, err)
	assert.NotNil(t, result, "non-CE workflow must pass through without error")
}

func TestResume_UserSet_WithValidCEInput_StampsUserExtension(t *testing.T) {
	task := newElevateTaskWithCE()
	require.NotNil(t, task.GetInputAsCloudEvent(), "CE input must be parseable before Resume")

	runner := &mockRunner{}
	svc := newTestService(true, runner)
	user := &models.User{Email: "frank@example.com"}

	result, err := svc.Resume(context.Background(), ResumeInput{
		Workflow: task,
		User:     user,
	})

	require.NoError(t, err)
	require.NotNil(t, result)

	ce := result.GetInputAsCloudEvent()
	require.NotNil(t, ce, "CE must remain parseable after Resume")

	extVal, ok := ce.Extensions()["user"]
	require.True(t, ok, "user extension must be present after CE stamping")
	assert.Equal(t, user.GetIdentity(), extVal)
}

func TestResume_RunnerError(t *testing.T) {
	sentinel := errors.New("workflow engine offline")
	runner := &mockRunner{
		resumeFn: func(_ *models.ElevateWorkflowTask) (*models.ElevateWorkflowTask, error) {
			return nil, sentinel
		},
	}
	svc := newTestService(true, runner)

	_, err := svc.Resume(context.Background(), ResumeInput{Workflow: newElevateTask()})

	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}
