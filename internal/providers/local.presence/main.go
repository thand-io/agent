package localpresence

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/localbroker"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	sdkWorkflowsRunner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	ProviderName                     = "local-presence"
	CheckLocalPresenceActivityName   = "CheckLocalPresenceActivity"
	SendNotificationActivityName     = "SendNotificationActivity"
	defaultLocalPresencePrompt       = "Approve this access request on your Mac"
	localPresenceProviderErrorType   = "LocalPresenceProviderActivityError"
	localPresenceBrokerErrorType     = "LocalPresenceBrokerError"
	localPresenceTimeoutReasonNeedle = "timed out"
)

type localPresenceProvider struct {
	*models.BaseProvider

	brokerClient localbroker.Client
	goos         func() string
}

func (p *localPresenceProvider) Initialize(identifier string, provider models.ProviderConfig) error {
	p.BaseProvider = models.NewBaseProvider(identifier, provider, LocalPresenceCapabilities)
	p.goos = func() string { return runtime.GOOS }
	if p.goos() == "darwin" {
		p.brokerClient = localbroker.NewCommandClient(p.GetConfig())
	}
	return nil
}

// SendNotification satisfies the notifier capability advertised by the
// local-presence provider. It maps the incoming NotificationRequest into a
// LocalPresenceApprovalRequest and triggers a presence challenge via the
// macOS helper.
//
// Two delivery shapes are supported:
//
//  1. Fire-and-forget (no SignalTarget on the request) — a denied or
//     timed-out prompt surfaces as a non-retryable application error,
//     mirroring the legacy behaviour used by the form / authorize / revoke
//     notifiers that have no workflow listener waiting.
//  2. Approval flow (SignalTarget populated) — once the broker resolves
//     the prompt the provider dispatches the registered signal-workflow
//     activity with a cloudevent carrying `approved: <bool>`. The workflow
//     listener (see internal/workflows/tasks/providers/thand/approvals.go)
//     consumes that signal exactly the same way it consumes the slack /
//     email approval URLs, so the approve task advances on both approve
//     and deny without aborting the whole workflow.
//
// Transient broker failures remain retryable in both shapes.
func (p *localPresenceProvider) SendNotification(
	ctx models.ProviderContext,
	notification models.NotificationRequest,
) error {
	// When invoked from a Temporal workflow coroutine, dispatch the actual
	// broker RPC as a Temporal activity so it benefits from retry, history,
	// and replay determinism (mirrors the email/slack providers).
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		return p.sendNotificationFromWorkflow(workflowCtx, notification)
	}

	return p.sendNotificationDirect(models.ContextFromProviderContext(ctx), notification)
}

// sendNotificationFromWorkflow is the Temporal-coroutine entry point. It
// runs the broker check as the registered CheckLocalPresenceActivity (so
// the long-blocking Touch ID prompt benefits from retry/replay), and if a
// SignalTarget is attached to the request it then dispatches the
// registered signal-workflow activity to deliver the approve/deny
// cloudevent back to the originating workflow listener.
func (p *localPresenceProvider) sendNotificationFromWorkflow(
	workflowCtx workflow.Context,
	notification models.NotificationRequest,
) error {
	var req models.LocalPresenceApprovalRequest
	if err := common.ConvertInterfaceToInterface(notification, &req); err != nil {
		return temporal.NewNonRetryableApplicationError(
			"failed to parse local presence payload",
			localPresenceProviderErrorType,
			err,
		)
	}

	wfCtx := workflow.WithActivityOptions(workflowCtx, workflow.ActivityOptions{
		StartToCloseTimeout: req.EffectiveTimeout() + 30*time.Second,
		RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
	})

	checkActivity := models.CreateTemporalProviderWorkflowName(
		p.GetIdentifier(), CheckLocalPresenceActivityName,
	)

	var resp *models.LocalPresenceApprovalResponse
	if err := workflow.ExecuteActivity(wfCtx, checkActivity, &req).Get(wfCtx, &resp); err != nil {
		return err
	}

	if req.SignalTarget == nil {
		// Fire-and-forget: preserve legacy semantics for callers that have
		// no workflow listener to receive the signal.
		if resp == nil || !resp.Approved {
			return temporal.NewNonRetryableApplicationError(
				localPresenceFailureReason(resp),
				localPresenceProviderErrorType,
				nil,
			)
		}
		return nil
	}

	approved := resp != nil && resp.Approved
	event := buildLocalPresenceApprovalEvent(workflowCtx, req.SignalTarget, approved)

	target := req.SignalTarget
	signalName := target.SignalName
	if len(signalName) == 0 {
		signalName = sdkWorkflowsModel.TemporalEventSignalName
	}

	if err := workflow.ExecuteActivity(
		wfCtx,
		sdkConstants.TemporalSignalWorkflowActivityName,
		target.WorkflowID,
		target.RunID,
		signalName,
		event,
	).Get(wfCtx, nil); err != nil {
		return err
	}

	return nil
}

// localPresenceFailureReason extracts a human-readable failure reason from
// a (possibly nil) response, falling back to a generic message when the
// broker did not provide one.
func localPresenceFailureReason(resp *models.LocalPresenceApprovalResponse) string {
	if resp != nil {
		if reason := strings.TrimSpace(resp.FailureReason); reason != "" {
			return reason
		}
	}
	return "local presence challenge was not approved"
}

// buildLocalPresenceApprovalEvent assembles the cloudevent that gets
// signaled to the originating workflow listener. The shape matches the
// slack/email approval URL flow so the same listener filter (see the
// approvals task ListenTaskHandler in
// internal/workflows/tasks/providers/thand/approvals.go) consumes it
// without any additional plumbing.
//
// UUID and timestamp generation are wrapped in workflow.SideEffect to keep
// the workflow coroutine deterministic across replays.
func buildLocalPresenceApprovalEvent(
	wfCtx workflow.Context,
	target *models.LocalPresenceSignalTarget,
	approved bool,
) cloudevents.Event {
	type seed struct {
		ID string    `json:"id"`
		At time.Time `json:"at"`
	}

	var s seed
	_ = workflow.SideEffect(wfCtx, func(workflow.Context) any {
		return seed{ID: uuid.New().String(), At: time.Now().UTC()}
	}).Get(&s)

	eventType := target.EventType
	if len(eventType) == 0 {
		eventType = "com.thand.approval"
	}
	eventSource := target.EventSource
	if len(eventSource) == 0 {
		eventSource = "urn:thand:agent"
	}

	event := cloudevents.NewEvent()
	event.SetSpecVersion("1.0")
	event.SetID(s.ID)
	event.SetTime(s.At)
	event.SetSource(eventSource)
	event.SetType(eventType)
	_ = event.SetData(cloudevents.ApplicationJSON, map[string]any{
		"approved": approved,
	})
	// Attach the approver identity so the approve task listener can
	// resolve who approved/denied. Without this extension the listener
	// logs "Approval event missing user extension" and loops back, which
	// is exactly the symptom seen when the cancel button signaled but
	// had no effect.
	if len(target.UserBase64) > 0 {
		event.SetExtension(sdkConstants.VarsContextUser, target.UserBase64)
	}
	return event
}

func (p *localPresenceProvider) sendNotificationDirect(
	ctx context.Context,
	notification models.NotificationRequest,
) error {
	var req models.LocalPresenceApprovalRequest
	if err := common.ConvertInterfaceToInterface(notification, &req); err != nil {
		return temporal.NewNonRetryableApplicationError(
			"failed to parse local presence payload",
			localPresenceProviderErrorType,
			err,
		)
	}

	resp, err := p.checkLocalPresenceDirect(ctx, &req)
	if err != nil {
		return err
	}

	if resp == nil || !resp.Approved {
		failureReason := ""
		if resp != nil {
			failureReason = strings.TrimSpace(resp.FailureReason)
		}
		if failureReason == "" {
			failureReason = "local presence challenge was not approved"
		}
		return temporal.NewNonRetryableApplicationError(
			failureReason,
			localPresenceProviderErrorType,
			nil,
		)
	}

	return nil
}

func (p *localPresenceProvider) checkLocalPresenceDirect(
	ctx context.Context,
	req *models.LocalPresenceApprovalRequest,
) (*models.LocalPresenceApprovalResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("local presence request is required")
	}
	if strings.TrimSpace(req.DeviceID) == "" {
		return nil, fmt.Errorf("local presence request is missing device_id")
	}
	if strings.TrimSpace(req.TaskName) == "" {
		return nil, fmt.Errorf("local presence request is missing task_name")
	}
	if p.goos() != "darwin" {
		return nil, fmt.Errorf("local presence checks are only supported on macOS")
	}
	if p.brokerClient == nil {
		return nil, fmt.Errorf("macOS privilege helper is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	timeout := req.EffectiveTimeout()
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = defaultLocalPresencePrompt
	}

	logrus.WithFields(logrus.Fields{
		"challenge_id": strings.TrimSpace(req.ChallengeID),
		"device_id":    strings.TrimSpace(req.DeviceID),
		"workflow_id":  strings.TrimSpace(req.WorkflowID),
		"task_name":    strings.TrimSpace(req.TaskName),
		"timeout":      timeout.String(),
	}).Info("Checking local presence via macOS helper")

	response, err := p.brokerClient.CheckLocalPresence(ctx, localbroker.CheckLocalPresenceRequest{
		ChallengeID: strings.TrimSpace(req.ChallengeID),
		DeviceID:    strings.TrimSpace(req.DeviceID),
		WorkflowID:  strings.TrimSpace(req.WorkflowID),
		TaskName:    strings.TrimSpace(req.TaskName),
		Prompt:      prompt,
		Timeout:     timeout,
		RequestedBy: strings.TrimSpace(req.RequestedBy),
		RoleName:    strings.TrimSpace(req.RoleName),
		Reason:      strings.TrimSpace(req.Reason),
	})
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"challenge_id": strings.TrimSpace(req.ChallengeID),
			"device_id":    strings.TrimSpace(req.DeviceID),
			"workflow_id":  strings.TrimSpace(req.WorkflowID),
			"task_name":    strings.TrimSpace(req.TaskName),
		}).Warn("Local presence helper check failed")
		return nil, wrapLocalPresenceBrokerError(err)
	}

	failureReason := strings.TrimSpace(response.FailureReason)
	logrus.WithFields(logrus.Fields{
		"approved":       response.Approved,
		"challenge_id":   strings.TrimSpace(req.ChallengeID),
		"device_id":      strings.TrimSpace(req.DeviceID),
		"failure_reason": failureReason,
		"workflow_id":    strings.TrimSpace(req.WorkflowID),
		"task_name":      strings.TrimSpace(req.TaskName),
	}).Info("Local presence helper check completed")

	return &models.LocalPresenceApprovalResponse{
		ChallengeID:     strings.TrimSpace(req.ChallengeID),
		DeviceID:        strings.TrimSpace(req.DeviceID),
		Approved:        response.Approved,
		AuthenticatedAt: response.AuthenticatedAt,
		FailureReason:   failureReason,
		Method:          models.LocalPresenceApprovalMethod,
		TimedOut:        localPresenceResponseTimedOut(failureReason),
	}, nil
}

func localPresenceResponseTimedOut(failureReason string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(failureReason)), localPresenceTimeoutReasonNeedle)
}

func wrapLocalPresenceBrokerError(err error) error {
	if err == nil {
		return nil
	}

	if localbroker.IsNonRetryableError(err) {
		return temporal.NewNonRetryableApplicationError(
			err.Error(),
			localPresenceBrokerErrorType,
			err,
		)
	}

	return err
}

func init() {
	providers.Register(ProviderName, &localPresenceProvider{}, LocalPresenceCapabilities, &ConfigSchema{})
}
