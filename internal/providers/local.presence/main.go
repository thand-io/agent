package localpresence

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/localbroker"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
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
// macOS helper. The challenge is fire-and-forget from the notifier
// interface's perspective: a denied/timed-out prompt surfaces as a
// non-retryable application error, while transient broker failures remain
// retryable.
func (p *localPresenceProvider) SendNotification(
	ctx models.ProviderContext,
	notification models.NotificationRequest,
) error {
	// When invoked from a Temporal workflow coroutine, dispatch the actual
	// broker RPC as a Temporal activity so it benefits from retry, history,
	// and replay determinism (mirrors the email/slack providers).
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		wfCtx := workflow.WithActivityOptions(workflowCtx, workflow.ActivityOptions{
			StartToCloseTimeout: 2 * time.Minute,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		return workflow.ExecuteActivity(
			wfCtx,
			models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), models.SendNotificationActivityName),
			notification,
		).Get(wfCtx, nil)
	}

	return p.sendNotificationDirect(models.ContextFromProviderContext(ctx), notification)
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
