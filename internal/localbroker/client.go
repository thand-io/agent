package localbroker

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/localbroker/proto/localbroker/v1"
	"github.com/thand-io/agent/internal/models"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type Operation string

const (
	OperationTimedSudoersGrant  Operation = "timed_sudoers_grant"
	OperationTimedSudoersRevoke Operation = "timed_sudoers_revoke"
	OperationExecCommand        Operation = "exec_command"
	OperationPTYSession         Operation = "pty_session"
	OperationCheckLocalPresence Operation = "check_local_presence"
	OperationPostNotification   Operation = "post_local_notification"

	DefaultControlExecutable = "/Library/Application Support/Thand/PrivilegeBroker/bin/thand-macos-privilege-brokerctl"
	DefaultMachServiceLabel  = "io.thand.agent.privilege-broker"
	ServiceLabelEnvVar       = "THAND_PRIVILEGE_BROKER_SERVICE_LABEL"

	defaultHelperTarget  = "passthrough:///localbroker"
	defaultHelperCommand = "serve"
)

type Client interface {
	GrantTimedSudoers(ctx context.Context, req TimedSudoersGrantRequest) (*TimedSudoersGrantResponse, error)
	RevokeTimedGrant(ctx context.Context, handle string) (*RevokeTimedGrantResponse, error)
	CheckLocalPresence(ctx context.Context, req CheckLocalPresenceRequest) (*CheckLocalPresenceResponse, error)
	PostLocalNotification(ctx context.Context, req PostLocalNotificationRequest) (*PostLocalNotificationResponse, error)
}

type TimedSudoersGrantRequest struct {
	GrantID          string        `json:"grant_id"`
	DeviceID         string        `json:"device_id"`
	TargetUsername   string        `json:"target_username"`
	RoleName         string        `json:"role_name"`
	Duration         time.Duration `json:"duration"`
	DeniedUsernames  []string      `json:"denied_usernames,omitempty"`
	AllowedUIDRanges []string      `json:"allowed_uid_ranges,omitempty"`
	RequestExpiresAt time.Time     `json:"request_expires_at,omitempty"`
}

func (r TimedSudoersGrantRequest) MarshalJSON() ([]byte, error) {
	type timedSudoersGrantRequestJSON struct {
		GrantID          string     `json:"grant_id"`
		DeviceID         string     `json:"device_id"`
		TargetUsername   string     `json:"target_username"`
		RoleName         string     `json:"role_name"`
		Duration         float64    `json:"duration"`
		DeniedUsernames  []string   `json:"denied_usernames,omitempty"`
		AllowedUIDRanges []string   `json:"allowed_uid_ranges,omitempty"`
		RequestExpiresAt *time.Time `json:"request_expires_at,omitempty"`
	}

	var requestExpiresAt *time.Time
	if !r.RequestExpiresAt.IsZero() {
		requestExpiresAt = &r.RequestExpiresAt
	}

	return json.Marshal(timedSudoersGrantRequestJSON{
		GrantID:          r.GrantID,
		DeviceID:         r.DeviceID,
		TargetUsername:   r.TargetUsername,
		RoleName:         r.RoleName,
		Duration:         r.Duration.Seconds(),
		DeniedUsernames:  r.DeniedUsernames,
		AllowedUIDRanges: r.AllowedUIDRanges,
		RequestExpiresAt: requestExpiresAt,
	})
}

func (r *TimedSudoersGrantRequest) UnmarshalJSON(data []byte) error {
	type timedSudoersGrantRequestJSON struct {
		GrantID          string     `json:"grant_id"`
		DeviceID         string     `json:"device_id"`
		TargetUsername   string     `json:"target_username"`
		RoleName         string     `json:"role_name"`
		Duration         float64    `json:"duration"`
		DeniedUsernames  []string   `json:"denied_usernames,omitempty"`
		AllowedUIDRanges []string   `json:"allowed_uid_ranges,omitempty"`
		RequestExpiresAt *time.Time `json:"request_expires_at,omitempty"`
	}

	var decoded timedSudoersGrantRequestJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	request := TimedSudoersGrantRequest{
		GrantID:          decoded.GrantID,
		DeviceID:         decoded.DeviceID,
		TargetUsername:   decoded.TargetUsername,
		RoleName:         decoded.RoleName,
		Duration:         time.Duration(decoded.Duration * float64(time.Second)),
		DeniedUsernames:  decoded.DeniedUsernames,
		AllowedUIDRanges: decoded.AllowedUIDRanges,
	}
	if decoded.RequestExpiresAt != nil {
		request.RequestExpiresAt = *decoded.RequestExpiresAt
	}
	*r = request

	return nil
}

type TimedSudoersGrantResponse struct {
	BrokerHandle   string    `json:"broker_handle"`
	TargetUsername string    `json:"target_username"`
	ExpiresAt      time.Time `json:"expires_at"`
}

type RevokeTimedGrantStatus string

const (
	RevokeTimedGrantStatusRevoked  RevokeTimedGrantStatus = "revoked"
	RevokeTimedGrantStatusNotFound RevokeTimedGrantStatus = "not_found"
)

type RevokeTimedGrantRequest struct {
	BrokerHandle string `json:"broker_handle"`
}

type RevokeTimedGrantResponse struct {
	Status RevokeTimedGrantStatus `json:"status"`
}

type CheckLocalPresenceRequest struct {
	ChallengeID string        `json:"challenge_id"`
	DeviceID    string        `json:"device_id"`
	WorkflowID  string        `json:"workflow_id"`
	TaskName    string        `json:"task_name"`
	Prompt      string        `json:"prompt"`
	Timeout     time.Duration `json:"timeout"`
	RequestedBy string        `json:"requested_by,omitempty"`
	RoleName    string        `json:"role_name,omitempty"`
	Reason      string        `json:"reason,omitempty"`
}

type CheckLocalPresenceResponse struct {
	Approved        bool      `json:"approved"`
	AuthenticatedAt time.Time `json:"authenticated_at,omitempty"`
	FailureReason   string    `json:"failure_reason,omitempty"`
}

type PostLocalNotificationRequest struct {
	NotificationID string `json:"notification_id,omitempty"`
	Title          string `json:"title"`
	Subtitle       string `json:"subtitle,omitempty"`
	Body           string `json:"body"`
	ThreadID       string `json:"thread_id,omitempty"`
}

type PostLocalNotificationResponse struct {
	Posted bool `json:"posted"`
}

type helperSession struct {
	dial        func(context.Context, string) (net.Conn, error)
	wait        func() error
	close       func() error
	diagnostics func() string
}

type helperStarter func(ctx context.Context, executable string, args []string) (*helperSession, error)

type CommandClient struct {
	executable   string
	serviceLabel string
	start        helperStarter
}

func NewCommandClient(_ *models.BasicConfig) *CommandClient {
	client := &CommandClient{
		executable:   DefaultControlExecutable,
		serviceLabel: DefaultMachServiceLabel,
		start:        newDefaultHelperStarter(),
	}

	if serviceLabel, ok := loadServiceLabelFromEnvironment(); ok {
		client.serviceLabel = serviceLabel
	}

	logrus.WithFields(logrus.Fields{
		"broker_executable":                  client.executable,
		"broker_service_label":               client.serviceLabel,
		"service_label_env_override_present": os.Getenv(ServiceLabelEnvVar) != "",
	}).Debug("configured local privilege broker command client")

	return client
}

func loadServiceLabelFromEnvironment() (string, bool) {
	value, ok := os.LookupEnv(ServiceLabelEnvVar)
	if !ok {
		return "", false
	}

	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", false
	}

	return trimmed, true
}
func (c *CommandClient) GrantTimedSudoers(ctx context.Context, req TimedSudoersGrantRequest) (*TimedSudoersGrantResponse, error) {
	return invokeHelperRPC(ctx, c, OperationTimedSudoersGrant, func(ctx context.Context, client localbrokerv1.LocalBrokerControlClient) (*TimedSudoersGrantResponse, error) {
		response, err := client.GrantTimedSudoers(ctx, timedSudoersGrantProtoRequest(req))
		if err != nil {
			return nil, err
		}

		return &TimedSudoersGrantResponse{
			BrokerHandle:   response.GetBrokerHandle(),
			TargetUsername: response.GetTargetUsername(),
			ExpiresAt:      time.UnixMilli(response.GetExpiresAtUnixMillis()).UTC(),
		}, nil
	})
}

func (c *CommandClient) RevokeTimedGrant(ctx context.Context, handle string) (*RevokeTimedGrantResponse, error) {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return nil, status.Error(codes.InvalidArgument, "broker handle is required")
	}

	return invokeHelperRPC(ctx, c, OperationTimedSudoersRevoke, func(ctx context.Context, client localbrokerv1.LocalBrokerControlClient) (*RevokeTimedGrantResponse, error) {
		response, err := client.RevokeTimedGrant(ctx, &localbrokerv1.RevokeTimedGrantRequest{
			BrokerHandle: handle,
		})
		if err != nil {
			return nil, err
		}

		switch response.GetStatus() {
		case localbrokerv1.RevokeTimedGrantResponse_STATUS_REVOKED:
			return &RevokeTimedGrantResponse{Status: RevokeTimedGrantStatusRevoked}, nil
		case localbrokerv1.RevokeTimedGrantResponse_STATUS_NOT_FOUND:
			return &RevokeTimedGrantResponse{Status: RevokeTimedGrantStatusNotFound}, nil
		default:
			return nil, status.Errorf(codes.Internal, "broker returned an unsupported timed sudoers revoke status %q", response.GetStatus().String())
		}
	})
}

func (c *CommandClient) CheckLocalPresence(ctx context.Context, req CheckLocalPresenceRequest) (*CheckLocalPresenceResponse, error) {
	return invokeHelperRPC(ctx, c, OperationCheckLocalPresence, func(ctx context.Context, client localbrokerv1.LocalBrokerControlClient) (*CheckLocalPresenceResponse, error) {
		response, err := client.CheckLocalPresence(ctx, localPresenceProtoRequest(req))
		if err != nil {
			return nil, err
		}

		var authenticatedAt time.Time
		if response.GetAuthenticatedAtUnixMillis() > 0 {
			authenticatedAt = time.UnixMilli(response.GetAuthenticatedAtUnixMillis()).UTC()
		}
		return &CheckLocalPresenceResponse{
			Approved:        response.GetApproved(),
			AuthenticatedAt: authenticatedAt,
			FailureReason:   response.GetFailureReason(),
		}, nil
	})
}

func (c *CommandClient) PostLocalNotification(ctx context.Context, req PostLocalNotificationRequest) (*PostLocalNotificationResponse, error) {
	return invokeHelperRPC(ctx, c, OperationPostNotification, func(ctx context.Context, client localbrokerv1.LocalBrokerControlClient) (*PostLocalNotificationResponse, error) {
		response, err := client.PostLocalNotification(ctx, localNotificationProtoRequest(req))
		if err != nil {
			return nil, err
		}

		return &PostLocalNotificationResponse{
			Posted: response.GetPosted(),
		}, nil
	})
}

func IsRetryableError(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled:
		return true
	default:
		return false
	}
}

func IsNonRetryableError(err error) bool {
	switch status.Code(err) {
	case codes.InvalidArgument, codes.AlreadyExists, codes.FailedPrecondition, codes.PermissionDenied:
		return true
	default:
		return false
	}
}

func invokeHelperRPC[T any](
	ctx context.Context,
	client *CommandClient,
	operation Operation,
	call func(context.Context, localbrokerv1.LocalBrokerControlClient) (T, error),
) (T, error) {
	var zero T
	if ctx == nil {
		ctx = context.Background()
	}

	args := client.helperArguments()
	logrus.WithFields(logrus.Fields{
		"operation":            operation,
		"broker_executable":    client.executable,
		"broker_service_label": client.serviceLabel,
		"broker_args":          args,
	}).Debug("launching local privilege broker helper")

	session, err := client.start(ctx, client.executable, args)
	if err != nil {
		return zero, client.unavailableError("failed to start broker helper", err, "")
	}

	clientConn, err := grpc.DialContext(
		ctx,
		defaultHelperTarget,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(session.dial),
		grpc.WithDisableRetry(),
		grpc.WithBlock(),
	)
	if err != nil {
		_ = session.close()
		waitErr := session.wait()
		return zero, client.unavailableError("failed to connect to broker helper", coalesceErrors(err, waitErr), session.diagnostics())
	}

	grpcClient := localbrokerv1.NewLocalBrokerControlClient(clientConn)
	result, err := call(ctx, grpcClient)
	closeErr := clientConn.Close()
	waitErr := session.wait()
	diagnostics := session.diagnostics()

	if err != nil {
		return zero, client.translateRPCError(err, diagnostics, waitErr)
	}
	if closeErr != nil {
		return zero, client.unavailableError("failed to close broker helper connection", coalesceErrors(closeErr, waitErr), diagnostics)
	}
	if waitErr != nil {
		return zero, client.unavailableError("broker helper exited unexpectedly", waitErr, diagnostics)
	}

	if diagnostics != "" {
		logrus.WithFields(logrus.Fields{
			"operation":            operation,
			"broker_service_label": client.serviceLabel,
			"broker_stderr":        diagnostics,
		}).Debug("local privilege broker helper emitted diagnostics")
	}

	return result, nil
}

func timedSudoersGrantProtoRequest(req TimedSudoersGrantRequest) *localbrokerv1.GrantTimedSudoersRequest {
	protoRequest := &localbrokerv1.GrantTimedSudoersRequest{
		GrantId:          req.GrantID,
		DeviceId:         req.DeviceID,
		TargetUsername:   req.TargetUsername,
		RoleName:         req.RoleName,
		DurationMillis:   req.Duration.Milliseconds(),
		DeniedUsernames:  append([]string(nil), req.DeniedUsernames...),
		AllowedUidRanges: append([]string(nil), req.AllowedUIDRanges...),
	}
	if !req.RequestExpiresAt.IsZero() {
		requestExpiresAtUnixMillis := req.RequestExpiresAt.UnixMilli()
		protoRequest.RequestExpiresAtUnixMillis = &requestExpiresAtUnixMillis
	}
	return protoRequest
}

func localPresenceProtoRequest(req CheckLocalPresenceRequest) *localbrokerv1.CheckLocalPresenceRequest {
	return &localbrokerv1.CheckLocalPresenceRequest{
		ChallengeId:   req.ChallengeID,
		DeviceId:      req.DeviceID,
		WorkflowId:    req.WorkflowID,
		TaskName:      req.TaskName,
		Prompt:        req.Prompt,
		TimeoutMillis: req.Timeout.Milliseconds(),
		RequestedBy:   req.RequestedBy,
		RoleName:      req.RoleName,
		Reason:        req.Reason,
	}
}

func localNotificationProtoRequest(req PostLocalNotificationRequest) *localbrokerv1.PostLocalNotificationRequest {
	return &localbrokerv1.PostLocalNotificationRequest{
		NotificationId: req.NotificationID,
		Title:          req.Title,
		Subtitle:       req.Subtitle,
		Body:           req.Body,
		ThreadId:       req.ThreadID,
	}
}

func (c *CommandClient) helperArguments() []string {
	args := []string{
		defaultHelperCommand,
		"--service-label", c.serviceLabel,
	}
	return args
}

func (c *CommandClient) translateRPCError(err error, diagnostics string, waitErr error) error {
	rpcStatus, ok := status.FromError(err)
	if !ok {
		return c.unavailableError("broker RPC failed", coalesceErrors(err, waitErr), diagnostics)
	}

	message := strings.TrimSpace(rpcStatus.Message())
	if diagnostics != "" && !strings.Contains(message, diagnostics) && rpcStatus.Code() == codes.Unavailable {
		if message == "" {
			message = diagnostics
		} else {
			message = message + ": " + diagnostics
		}
	}
	if waitErr != nil && rpcStatus.Code() == codes.Unavailable && !strings.Contains(message, waitErr.Error()) {
		if message == "" {
			message = waitErr.Error()
		} else {
			message = message + ": " + waitErr.Error()
		}
	}
	if message == "" {
		message = "broker helper RPC failed"
	}

	return status.Error(rpcStatus.Code(), c.annotateControlError(message))
}

func (c *CommandClient) unavailableError(prefix string, err error, diagnostics string) error {
	message := prefix
	if err != nil {
		message = message + ": " + err.Error()
	}
	diagnostics = strings.TrimSpace(diagnostics)
	if diagnostics != "" && !strings.Contains(message, diagnostics) {
		message = message + ": " + diagnostics
	}
	return status.Error(codes.Unavailable, c.annotateControlError(message))
}

func (c *CommandClient) annotateControlError(message string) string {
	if strings.Contains(message, "Peer forbidden (code signing)") {
		return message + ". The broker client is enforcing strict peer checks. Apple Development-sign the local binaries with `make build`, then reinstall the strict local payload with `sudo make install-macos-privilege-services-dev`."
	}

	if strings.Contains(message, "agent peer identity check failed") {
		return message + ". The broker daemon rejected the agent identity. Verify `bin/thand` is Apple Development-signed by `make build`, then reinstall the local payload with `sudo make install-macos-privilege-services-dev`."
	}

	return message
}

func coalesceErrors(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
