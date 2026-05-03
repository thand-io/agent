package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	osuser "os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/localbroker"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const LocalProviderName = "local"

var localSudoPermission = models.ProviderPermission{
	ID:          "local-sudo",
	Name:        "local:sudo:*",
	Title:       "Local sudo access",
	Description: "Managed local sudo and privileged command execution",
}

type commandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type leaseRecord struct {
	GrantID     string    `json:"grant_id,omitempty"`
	DeviceID    string    `json:"device_id,omitempty"`
	Username    string    `json:"username,omitempty"`
	SudoersPath string    `json:"sudoers_path,omitempty"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
}

type localElevationLease struct {
	GrantID     string
	DeviceID    string
	Username    string
	SudoersPath string
	LeasePath   string
	ExpiresAt   time.Time
}

type localCommandExecution struct {
	Result          commandResult
	ResolvedCommand []string
}

type localElevationEnforcer interface {
	GrantTimed(username string, meta models.LocalSudoRequestMetadata, roleName string, duration time.Duration) (*localElevationLease, error)
	RunCommand(username string, meta models.LocalSudoRequestMetadata, roleName string) (*localCommandExecution, error)
	Revoke(meta models.LocalSudoAuthorizationMetadata) error
	Reconcile() error
}

type localProvider struct {
	*models.BaseProvider

	lookPath       func(string) (string, error)
	runCommand     func(name string, args ...string) (commandResult, error)
	lookupUser     func(string) (*osuser.User, error)
	readFile       func(string) ([]byte, error)
	readDir        func(string) ([]os.DirEntry, error)
	goos           func() string
	removeFile     func(string) error
	renameFile     func(string, string) error
	writeFile      func(string, []byte, os.FileMode) error
	createTempFile func(dir, pattern string) (*os.File, error)
	chmodFile      func(string, os.FileMode) error
	mkdirAll       func(string, os.FileMode) error
	now            func() time.Time
	afterFunc      func(time.Duration, func()) *time.Timer

	enforcer     localElevationEnforcer
	brokerClient localbroker.Client
}

type sudoersEnforcer struct {
	provider *localProvider

	timerMu sync.Mutex
	timers  map[string]*time.Timer
}

func (p *localProvider) Initialize(identifier string, provider models.ProviderConfig) error {
	p.BaseProvider = models.NewBaseProvider(identifier, provider, LocalCapabilities)
	p.lookPath = exec.LookPath
	p.runCommand = runSystemCommand
	p.lookupUser = osuser.Lookup
	p.readFile = os.ReadFile
	p.readDir = os.ReadDir
	p.goos = func() string { return runtime.GOOS }
	p.removeFile = os.Remove
	p.renameFile = os.Rename
	p.writeFile = os.WriteFile
	p.createTempFile = os.CreateTemp
	p.chmodFile = os.Chmod
	p.mkdirAll = os.MkdirAll
	p.now = func() time.Time { return time.Now().UTC() }
	p.afterFunc = time.AfterFunc
	p.enforcer = newSudoersEnforcer(p)
	if p.goos() == "darwin" {
		p.brokerClient = localbroker.NewCommandClient(p.GetConfig())
	}
	p.SetPermissions([]models.ProviderPermission{localSudoPermission})

	logrus.WithFields(logrus.Fields{
		"provider_identifier": identifier,
		"provider_name":       provider.Name,
		"provider_type":       provider.Provider,
		"goos":                p.goos(),
		"has_inline_config":   provider.Config != nil,
		"provider_config":     p.GetConfig().AsMap(),
	}).Info("initialized local provider")

	if p.goos() != "darwin" {
		if err := p.enforcer.Reconcile(); err != nil {
			logrus.WithError(err).Warn("failed to reconcile existing local elevation leases")
		}
	}

	return nil
}

func newSudoersEnforcer(provider *localProvider) *sudoersEnforcer {
	return &sudoersEnforcer{
		provider: provider,
		timers:   map[string]*time.Timer{},
	}
}

func (p *localProvider) AuthorizeRole(
	ctx models.ProviderContext,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		return p.authorizeRoleTemporal(workflowCtx, req)
	}

	requestContext, cancel := contextFromProviderContext(ctx)
	defer cancel()

	return p.authorizeRoleDirect(requestContext, req)
}

func (p *localProvider) RevokeRole(
	ctx models.ProviderContext,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		return p.revokeRoleTemporal(workflowCtx, req)
	}

	requestContext, cancel := contextFromProviderContext(ctx)
	defer cancel()

	return p.revokeRoleDirect(requestContext, req)
}

func (p *localProvider) authorizeRoleDirect(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	if req == nil || !req.IsValid() {
		return nil, fmt.Errorf("user and role must be provided to authorize local sudo access")
	}

	meta, err := decodeLocalSudoRequestMetadata(req.Role)
	if err != nil {
		return nil, err
	}

	switch p.goos() {
	case "linux", "darwin":
		return p.authorizeUnix(ctx, req, meta)
	case "windows":
		return p.authorizeWindows(req, meta)
	default:
		return nil, fmt.Errorf("local sudo is not supported on %s", p.goos())
	}
}

func (p *localProvider) revokeRoleDirect(
	ctx context.Context,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {
	if req == nil || req.AuthorizeRoleResponse == nil || len(req.AuthorizeRoleResponse.Metadata) == 0 {
		return &models.RevokeRoleResponse{}, nil
	}

	meta, err := decodeLocalSudoAuthorizationMetadata(req.Role)
	if err != nil {
		return nil, err
	}

	if meta.BrokerHandle != "" && (meta.Platform == "darwin" || p.goos() == "darwin") {
		if p.brokerClient == nil {
			return nil, fmt.Errorf("macOS privilege broker is not configured")
		}
		if ctx == nil {
			ctx = context.Background()
		}
		revokeResponse, err := p.brokerClient.RevokeTimedGrant(ctx, meta.BrokerHandle)
		if err != nil {
			return nil, wrapLocalBrokerError(err)
		}
		logFields := logrus.Fields{
			"provider_identifier": p.GetIdentifier(),
			"provider_name":       p.GetName(),
			"broker_handle":       meta.BrokerHandle,
			"broker_status":       revokeResponse.Status,
		}
		switch revokeResponse.Status {
		case localbroker.RevokeTimedGrantStatusRevoked:
			logrus.WithFields(logFields).Debug("revoked brokered macOS local sudo grant")
		case localbroker.RevokeTimedGrantStatusNotFound:
			logrus.WithFields(logFields).Debug("brokered macOS local sudo revoke converged because the lease was already absent")
		}
		return &models.RevokeRoleResponse{}, nil
	}

	if err := p.enforcer.Revoke(meta); err != nil {
		return nil, err
	}

	return &models.RevokeRoleResponse{}, nil
}

func (p *localProvider) authorizeUnix(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
	meta models.LocalSudoRequestMetadata,
) (*models.AuthorizeRoleResponse, error) {
	username, err := p.targetUsername(meta)
	if err != nil {
		return nil, err
	}

	switch meta.Mode {
	case models.LocalSudoModeTimed:
		if req.Duration == nil || *req.Duration <= 0 {
			return nil, fmt.Errorf("timed local sudo requires a positive duration")
		}
		if p.goos() == "darwin" {
			return p.authorizeDarwinTimed(ctx, req, meta, username)
		}
		if _, err := p.validateTargetUsername(username, meta); err != nil {
			return nil, err
		}

		lease, err := p.enforcer.GrantTimed(username, meta, req.Role.GetName(), *req.Duration)
		if err != nil {
			return nil, err
		}

		return localAuthorizeResponse(req, models.LocalSudoAuthorizationMetadata{
			Platform:    p.goos(),
			Mode:        string(meta.Mode),
			GrantID:     meta.GrantID,
			DeviceID:    meta.DeviceID,
			Username:    lease.Username,
			SudoersPath: lease.SudoersPath,
			LeasePath:   lease.LeasePath,
		}), nil
	case models.LocalSudoModeCommand:
		if len(meta.Command) == 0 {
			return nil, fmt.Errorf("privileged command mode requires a command")
		}
		if p.goos() == "darwin" {
			return nil, fmt.Errorf("privileged command mode is not supported on macOS in broker v1; request timed sudo access instead")
		}
		if _, err := p.validateTargetUsername(username, meta); err != nil {
			return nil, err
		}

		execution, err := p.enforcer.RunCommand(username, meta, req.Role.GetName())
		if err != nil {
			return nil, err
		}

		return localAuthorizeResponse(req, models.LocalSudoAuthorizationMetadata{
			Platform:  p.goos(),
			Mode:      string(meta.Mode),
			GrantID:   meta.GrantID,
			DeviceID:  meta.DeviceID,
			Username:  username,
			Command:   execution.ResolvedCommand,
			Stdout:    execution.Result.Stdout,
			Stderr:    execution.Result.Stderr,
			ExitCode:  execution.Result.ExitCode,
			Immediate: true,
		}), nil
	default:
		return nil, fmt.Errorf("unsupported local sudo mode %q", meta.Mode)
	}
}

func (p *localProvider) authorizeWindows(
	req *models.AuthorizeRoleRequest,
	meta models.LocalSudoRequestMetadata,
) (*models.AuthorizeRoleResponse, error) {
	if meta.Mode == models.LocalSudoModeTimed {
		return nil, fmt.Errorf("timed sudo access is not supported on Windows in v1; use a brokered command instead")
	}

	if len(meta.Command) == 0 {
		return nil, fmt.Errorf("Windows sudo requires a command")
	}

	sudoPath, err := p.resolveExecutable(p.GetConfig().GetStringWithDefault("sudo_path", "sudo"), []string{"sudo"})
	if err != nil {
		return nil, fmt.Errorf("Windows Sudo is unavailable: %w", err)
	}

	resolvedCommand, err := p.resolveCommand(meta.Command)
	if err != nil {
		return nil, err
	}

	result, runErr := p.runCommand(sudoPath, resolvedCommand...)
	if runErr != nil {
		return nil, fmt.Errorf("Windows sudo command failed: %w\nstdout:\n%s\nstderr:\n%s", runErr, result.Stdout, result.Stderr)
	}

	return localAuthorizeResponse(req, models.LocalSudoAuthorizationMetadata{
		Platform:  p.goos(),
		Mode:      string(meta.Mode),
		GrantID:   meta.GrantID,
		DeviceID:  meta.DeviceID,
		Command:   resolvedCommand,
		Stdout:    result.Stdout,
		Stderr:    result.Stderr,
		ExitCode:  result.ExitCode,
		Immediate: true,
	}), nil
}

func localAuthorizeResponse(
	req *models.AuthorizeRoleRequest,
	meta models.LocalSudoAuthorizationMetadata,
) *models.AuthorizeRoleResponse {
	response := &models.AuthorizeRoleResponse{
		Roles:    []string{req.Role.GetName()},
		Metadata: map[string]any{},
	}

	if req.Identity != nil && req.Identity.User != nil {
		response.UserId = req.Identity.User.GetMappableIdentifier()
	}

	if len(meta.Platform) > 0 {
		response.Metadata["platform"] = meta.Platform
	}
	if len(meta.Mode) > 0 {
		response.Metadata["mode"] = meta.Mode
	}
	if len(meta.GrantID) > 0 {
		response.Metadata["grant_id"] = meta.GrantID
	}
	if len(meta.DeviceID) > 0 {
		response.Metadata["device_id"] = meta.DeviceID
	}
	if len(meta.Username) > 0 {
		response.Metadata["username"] = meta.Username
	}
	if len(meta.BrokerHandle) > 0 {
		response.Metadata["broker_handle"] = meta.BrokerHandle
	}
	if len(meta.SudoersPath) > 0 {
		response.Metadata["sudoers_path"] = meta.SudoersPath
	}
	if len(meta.LeasePath) > 0 {
		response.Metadata["lease_path"] = meta.LeasePath
	}
	if len(meta.Command) > 0 {
		response.Metadata["command"] = append([]string(nil), meta.Command...)
	}
	if len(meta.Stdout) > 0 {
		response.Metadata["stdout"] = meta.Stdout
	}
	if len(meta.Stderr) > 0 {
		response.Metadata["stderr"] = meta.Stderr
	}
	if meta.ExitCode != 0 {
		response.Metadata["exit_code"] = meta.ExitCode
	}
	if meta.Immediate {
		response.Metadata["immediate"] = true
	}
	if meta.RevokedLocally {
		response.Metadata["revoked_locally"] = true
	}

	return response
}

type uidRange struct {
	Min int
	Max int
}

func (p *localProvider) targetUsername(meta models.LocalSudoRequestMetadata) (string, error) {
	username := strings.TrimSpace(meta.LocalUsername)
	if username == "" {
		return "", fmt.Errorf("local sudo request is missing a resolved local username")
	}
	return username, nil
}

func (p *localProvider) authorizeDarwinTimed(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
	meta models.LocalSudoRequestMetadata,
	username string,
) (*models.AuthorizeRoleResponse, error) {
	if p.brokerClient == nil {
		return nil, fmt.Errorf("macOS privilege broker is not configured")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	brokerRequest := localbroker.TimedSudoersGrantRequest{
		GrantID:          meta.GrantID,
		DeviceID:         meta.DeviceID,
		TargetUsername:   username,
		RoleName:         req.Role.GetName(),
		Duration:         *req.Duration,
		DeniedUsernames:  append([]string(nil), meta.DeniedUsernames...),
		AllowedUIDRanges: append([]string(nil), meta.AllowedUIDRanges...),
	}

	logrus.WithFields(logrus.Fields{
		"provider_identifier": p.GetIdentifier(),
		"provider_name":       p.GetName(),
		"role":                req.Role.GetName(),
		"device_id":           meta.DeviceID,
		"grant_id":            meta.GrantID,
		"target_username":     username,
		"duration":            req.Duration.String(),
		"provider_config":     p.GetConfig().AsMap(),
		"denied_usernames":    brokerRequest.DeniedUsernames,
		"allowed_uid_ranges":  brokerRequest.AllowedUIDRanges,
	}).Info("authorizing brokered macOS timed sudo request")

	grant, err := p.brokerClient.GrantTimedSudoers(ctx, brokerRequest)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"provider_identifier": p.GetIdentifier(),
			"device_id":           meta.DeviceID,
			"grant_id":            meta.GrantID,
			"target_username":     username,
			"provider_config":     p.GetConfig().AsMap(),
		}).Warn("brokered macOS timed sudo authorization failed")
		return nil, wrapLocalBrokerError(err)
	}

	logrus.WithFields(logrus.Fields{
		"provider_identifier": p.GetIdentifier(),
		"device_id":           meta.DeviceID,
		"grant_id":            meta.GrantID,
		"target_username":     grant.TargetUsername,
		"broker_handle":       grant.BrokerHandle,
	}).Info("brokered macOS timed sudo authorization succeeded")

	return localAuthorizeResponse(req, models.LocalSudoAuthorizationMetadata{
		Platform:     p.goos(),
		Mode:         string(meta.Mode),
		GrantID:      meta.GrantID,
		DeviceID:     meta.DeviceID,
		Username:     grant.TargetUsername,
		BrokerHandle: grant.BrokerHandle,
	}), nil
}

func wrapLocalBrokerError(err error) error {
	if err == nil {
		return nil
	}

	if localbroker.IsNonRetryableError(err) {
		return temporal.NewNonRetryableApplicationError(
			err.Error(),
			"LocalBrokerError",
			err,
		)
	}

	return err
}

func contextFromProviderContext(ctx models.ProviderContext) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.Background(), func() {}
	}

	if contextCtx, ok := ctx.(context.Context); ok {
		return contextCtx, func() {}
	}

	if deadline, ok := ctx.Deadline(); ok {
		return context.WithDeadline(context.Background(), deadline)
	}

	return context.Background(), func() {}
}

func (p *localProvider) validateTargetUsername(username string, meta models.LocalSudoRequestMetadata) (string, error) {
	username = strings.TrimSpace(username)
	if len(username) == 0 || strings.ContainsAny(username, " \t\r\n") {
		return "", fmt.Errorf("resolved local username %q is invalid", username)
	}

	if p.isDeniedUsername(username, meta) {
		return "", fmt.Errorf("local username %q is denied for local sudo", username)
	}

	foundUser, err := p.lookupUser(username)
	if err != nil {
		return "", fmt.Errorf("resolved local username %q could not be resolved: %w", username, err)
	}
	if foundUser == nil {
		return "", fmt.Errorf("resolved local username %q could not be resolved", username)
	}

	if err := p.validateAllowedUID(foundUser, meta); err != nil {
		return "", err
	}

	if resolved := strings.TrimSpace(foundUser.Username); resolved != "" {
		return resolved, nil
	}

	return username, nil
}

func (p *localProvider) isDeniedUsername(username string, meta models.LocalSudoRequestMetadata) bool {
	denied := []string{"root", "daemon", "nobody"}
	if len(meta.DeniedUsernames) > 0 {
		denied = append(denied, meta.DeniedUsernames...)
	} else if configured, found := p.GetConfig().GetStringSlice("denied_usernames"); found && len(configured) > 0 {
		denied = append(denied, configured...)
	}

	for _, candidate := range denied {
		if strings.EqualFold(strings.TrimSpace(candidate), username) {
			return true
		}
	}
	return false
}

func (p *localProvider) validateAllowedUID(foundUser *osuser.User, meta models.LocalSudoRequestMetadata) error {
	if foundUser == nil {
		return fmt.Errorf("local user lookup returned nil user")
	}
	uid, err := strconv.Atoi(foundUser.Uid)
	if err != nil {
		return fmt.Errorf("failed to parse UID %q for local user %q: %w", foundUser.Uid, foundUser.Username, err)
	}

	ranges, err := p.allowedUIDRanges(meta)
	if err != nil {
		return err
	}

	for _, allowed := range ranges {
		if uid >= allowed.Min && uid <= allowed.Max {
			return nil
		}
	}

	return fmt.Errorf("local user %q with UID %d is outside allowed UID ranges", foundUser.Username, uid)
}

func (p *localProvider) allowedUIDRanges(meta models.LocalSudoRequestMetadata) ([]uidRange, error) {
	if len(meta.AllowedUIDRanges) > 0 {
		return parseUIDRanges(meta.AllowedUIDRanges)
	}

	if configured, found := p.GetConfig().GetStringSlice("allowed_uid_ranges"); found && len(configured) > 0 {
		return parseUIDRanges(configured)
	}

	if p.goos() != "windows" {
		if derived, err := p.allowedUIDRangesFromLoginDefs(); err == nil && len(derived) > 0 {
			return derived, nil
		}
	}

	switch p.goos() {
	case "darwin":
		return []uidRange{{Min: 500, Max: 60000}}, nil
	default:
		return []uidRange{{Min: 1000, Max: 60000}}, nil
	}
}

func (p *localProvider) allowedUIDRangesFromLoginDefs() ([]uidRange, error) {
	data, err := p.readFile("/etc/login.defs")
	if err != nil {
		return nil, err
	}

	var uidMin, uidMax int
	var hasMin, hasMax bool

	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if comment := strings.Index(line, "#"); comment >= 0 {
			line = strings.TrimSpace(line[:comment])
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		switch fields[0] {
		case "UID_MIN":
			value, err := strconv.Atoi(fields[1])
			if err == nil {
				uidMin = value
				hasMin = true
			}
		case "UID_MAX":
			value, err := strconv.Atoi(fields[1])
			if err == nil {
				uidMax = value
				hasMax = true
			}
		}
	}

	if !hasMin || !hasMax || uidMin > uidMax {
		return nil, fmt.Errorf("login.defs did not define a valid UID_MIN/UID_MAX range")
	}

	return []uidRange{{Min: uidMin, Max: uidMax}}, nil
}

func parseUIDRanges(values []string) ([]uidRange, error) {
	ranges := make([]uidRange, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}

		if strings.Contains(value, "-") {
			parts := strings.SplitN(value, "-", 2)
			minValue, err := strconv.Atoi(strings.TrimSpace(parts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid UID range %q", value)
			}
			maxValue, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid UID range %q", value)
			}
			if minValue > maxValue {
				return nil, fmt.Errorf("invalid UID range %q", value)
			}
			ranges = append(ranges, uidRange{Min: minValue, Max: maxValue})
			continue
		}

		exactValue, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("invalid UID value %q", value)
		}
		ranges = append(ranges, uidRange{Min: exactValue, Max: exactValue})
	}

	if len(ranges) == 0 {
		return nil, fmt.Errorf("no valid UID ranges configured")
	}
	return ranges, nil
}

func (e *sudoersEnforcer) GrantTimed(username string, meta models.LocalSudoRequestMetadata, roleName string, duration time.Duration) (*localElevationLease, error) {
	sudoersPath, err := e.provider.installSudoersGrant(username, nil, roleName, meta.GrantID)
	if err != nil {
		return nil, err
	}

	leasePath, err := e.writeLease(localElevationLease{
		GrantID:     meta.GrantID,
		DeviceID:    meta.DeviceID,
		Username:    username,
		SudoersPath: sudoersPath,
		ExpiresAt:   e.provider.now().Add(duration),
	})
	if err != nil {
		_ = e.provider.removeFile(sudoersPath)
		return nil, err
	}

	lease := &localElevationLease{
		GrantID:     meta.GrantID,
		DeviceID:    meta.DeviceID,
		Username:    username,
		SudoersPath: sudoersPath,
		LeasePath:   leasePath,
		ExpiresAt:   e.provider.now().Add(duration),
	}
	e.scheduleLease(*lease)
	return lease, nil
}

func (e *sudoersEnforcer) RunCommand(username string, meta models.LocalSudoRequestMetadata, roleName string) (*localCommandExecution, error) {
	if len(meta.Command) == 0 {
		return nil, fmt.Errorf("privileged command mode requires a command")
	}

	resolvedCommand, err := e.provider.resolveCommand(meta.Command)
	if err != nil {
		return nil, err
	}

	sudoersPath, err := e.provider.installSudoersGrant(username, resolvedCommand, roleName, meta.GrantID)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := e.provider.removeFile(sudoersPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			logrus.WithError(err).Warn("failed to clean up sudoers grant after command execution")
		}
	}()

	result, runErr := e.provider.runCommand(e.provider.sudoExecutable(), resolvedCommand...)
	if runErr != nil {
		return nil, fmt.Errorf("privileged command failed: %w\nstdout:\n%s\nstderr:\n%s", runErr, result.Stdout, result.Stderr)
	}

	return &localCommandExecution{
		Result:          result,
		ResolvedCommand: resolvedCommand,
	}, nil
}

func (e *sudoersEnforcer) Revoke(meta models.LocalSudoAuthorizationMetadata) error {
	if len(meta.LeasePath) > 0 {
		e.stopTimer(meta.LeasePath)
		if err := e.provider.removeFile(meta.LeasePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove local lease metadata: %w", err)
		}
	}

	if len(meta.SudoersPath) == 0 {
		return nil
	}

	if err := e.provider.removeFile(meta.SudoersPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to revoke local sudo access: %w", err)
	}

	return nil
}

func (e *sudoersEnforcer) Reconcile() error {
	leaseDir := e.provider.leaseDir()
	if err := e.provider.mkdirAll(leaseDir, 0700); err != nil {
		return fmt.Errorf("failed to create local elevation lease directory: %w", err)
	}

	entries, err := e.provider.readDir(leaseDir)
	if err != nil {
		return fmt.Errorf("failed to list local elevation leases: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		leasePath := filepath.Join(leaseDir, entry.Name())
		record, err := e.readLeaseRecord(leasePath)
		if err != nil {
			logrus.WithError(err).WithField("lease_path", leasePath).Warn("failed to parse local elevation lease")
			continue
		}

		lease := localElevationLease{
			GrantID:     record.GrantID,
			DeviceID:    record.DeviceID,
			Username:    record.Username,
			SudoersPath: record.SudoersPath,
			LeasePath:   leasePath,
			ExpiresAt:   record.ExpiresAt,
		}

		if !lease.ExpiresAt.IsZero() && !lease.ExpiresAt.After(e.provider.now()) {
			if err := e.Revoke(models.LocalSudoAuthorizationMetadata{
				GrantID:     lease.GrantID,
				DeviceID:    lease.DeviceID,
				Username:    lease.Username,
				SudoersPath: lease.SudoersPath,
				LeasePath:   lease.LeasePath,
			}); err != nil {
				logrus.WithError(err).WithField("lease_path", leasePath).Warn("failed to revoke expired local elevation lease")
			}
			continue
		}

		e.scheduleLease(lease)
	}

	return nil
}

func (e *sudoersEnforcer) scheduleLease(lease localElevationLease) {
	if lease.LeasePath == "" || lease.ExpiresAt.IsZero() {
		return
	}

	delay := time.Until(lease.ExpiresAt)
	if delay < 0 {
		delay = 0
	}

	e.timerMu.Lock()
	defer e.timerMu.Unlock()

	if existing := e.timers[lease.LeasePath]; existing != nil {
		existing.Stop()
	}

	e.timers[lease.LeasePath] = e.provider.afterFunc(delay, func() {
		if err := e.Revoke(models.LocalSudoAuthorizationMetadata{
			GrantID:     lease.GrantID,
			DeviceID:    lease.DeviceID,
			Username:    lease.Username,
			SudoersPath: lease.SudoersPath,
			LeasePath:   lease.LeasePath,
			Mode:        string(models.LocalSudoModeTimed),
		}); err != nil {
			logrus.WithError(err).WithField("lease_path", lease.LeasePath).Warn("failed to revoke expired local elevation lease")
		}
		e.stopTimer(lease.LeasePath)
	})
}

func (e *sudoersEnforcer) stopTimer(leasePath string) {
	e.timerMu.Lock()
	defer e.timerMu.Unlock()

	if timer := e.timers[leasePath]; timer != nil {
		timer.Stop()
		delete(e.timers, leasePath)
	}
}

func (e *sudoersEnforcer) writeLease(lease localElevationLease) (string, error) {
	leaseDir := e.provider.leaseDir()
	if err := e.provider.mkdirAll(leaseDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create local elevation lease directory: %w", err)
	}

	fileName, err := sudoersFragmentName("lease", nil, lease.GrantID)
	if err != nil {
		return "", err
	}
	leasePath := filepath.Join(leaseDir, fileName+".json")

	record := leaseRecord{
		GrantID:     lease.GrantID,
		DeviceID:    lease.DeviceID,
		Username:    lease.Username,
		SudoersPath: lease.SudoersPath,
		ExpiresAt:   lease.ExpiresAt,
	}

	data, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("failed to encode local elevation lease: %w", err)
	}

	if err := e.provider.writeFile(leasePath, data, 0600); err != nil {
		return "", fmt.Errorf("failed to write local elevation lease: %w", err)
	}

	return leasePath, nil
}

func (e *sudoersEnforcer) readLeaseRecord(path string) (*leaseRecord, error) {
	data, err := e.provider.readFile(path)
	if err != nil {
		return nil, err
	}

	record := &leaseRecord{}
	if err := json.Unmarshal(data, record); err != nil {
		return nil, err
	}
	return record, nil
}

func (p *localProvider) leaseDir() string {
	if leaseDir := strings.TrimSpace(p.GetConfig().GetStringWithDefault("lease_dir", "")); leaseDir != "" {
		return leaseDir
	}
	return filepath.Join(os.TempDir(), "thand-local-elevation")
}

func (p *localProvider) installSudoersGrant(username string, command []string, roleName string, grantID string) (string, error) {
	sudoersDir := p.GetConfig().GetStringWithDefault("sudoers_dir", "/etc/sudoers.d")
	if len(sudoersDir) == 0 {
		return "", fmt.Errorf("sudoers directory is not configured")
	}

	if err := p.mkdirAll(sudoersDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create sudoers directory: %w", err)
	}

	fileName, err := sudoersFragmentName(roleName, command, grantID)
	if err != nil {
		return "", err
	}

	targetPath := filepath.Join(sudoersDir, fileName)
	content, err := p.buildSudoersContent(username, command)
	if err != nil {
		return "", err
	}

	tempFile, err := p.createTempFile(sudoersDir, ".thand-sudo-*")
	if err != nil {
		return "", fmt.Errorf("failed to create sudoers temp file: %w", err)
	}
	tempPath := tempFile.Name()
	_ = tempFile.Close()

	cleanupTemp := func() {
		if err := p.removeFile(tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			logrus.WithError(err).Warn("failed to clean up temporary sudoers file")
		}
	}

	if err := p.writeFile(tempPath, []byte(content), 0440); err != nil {
		cleanupTemp()
		return "", fmt.Errorf("failed to write sudoers temp file: %w", err)
	}

	if err := p.chmodFile(tempPath, 0440); err != nil {
		cleanupTemp()
		return "", fmt.Errorf("failed to set sudoers permissions: %w", err)
	}

	if err := p.validateSudoersFile(tempPath); err != nil {
		cleanupTemp()
		return "", err
	}

	if err := p.renameFile(tempPath, targetPath); err != nil {
		cleanupTemp()
		return "", fmt.Errorf("failed to install sudoers grant: %w", err)
	}

	if err := p.chmodFile(targetPath, 0440); err != nil {
		return "", fmt.Errorf("failed to set installed sudoers permissions: %w", err)
	}

	return targetPath, nil
}

func (p *localProvider) buildSudoersContent(username string, command []string) (string, error) {
	if strings.ContainsAny(username, " \t\r\n") {
		return "", fmt.Errorf("local username %q is not supported in sudoers grant", username)
	}

	spec := "ALL"
	if len(command) > 0 {
		resolved, err := sudoersCommandSpec(command)
		if err != nil {
			return "", err
		}
		spec = resolved
	}

	return fmt.Sprintf("# Managed by Thand\n%s ALL=(ALL:ALL) NOPASSWD: %s\n", username, spec), nil
}

func (p *localProvider) validateSudoersFile(path string) error {
	visudoPath, err := p.resolveExecutable(
		p.GetConfig().GetStringWithDefault("visudo_path", ""),
		[]string{"visudo", "/usr/sbin/visudo"},
	)
	if err != nil {
		return fmt.Errorf("failed to locate visudo for sudoers validation: %w", err)
	}

	result, runErr := p.runCommand(visudoPath, "-c", "-f", path)
	if runErr != nil {
		return fmt.Errorf("sudoers validation failed: %w\nstdout:\n%s\nstderr:\n%s", runErr, result.Stdout, result.Stderr)
	}

	return nil
}

func (p *localProvider) sudoExecutable() string {
	return p.GetConfig().GetStringWithDefault("sudo_path", "sudo")
}

func (p *localProvider) resolveCommand(command []string) ([]string, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("command is required")
	}

	executable, err := p.resolveExecutable(command[0], []string{command[0]})
	if err != nil {
		return nil, fmt.Errorf("failed to locate command %q: %w", command[0], err)
	}

	resolved := append([]string{executable}, command[1:]...)
	return resolved, nil
}

func (p *localProvider) resolveExecutable(value string, fallbacks []string) (string, error) {
	candidates := []string{}
	if len(value) > 0 {
		candidates = append(candidates, value)
	}
	candidates = append(candidates, fallbacks...)

	var lastErr error
	for _, candidate := range candidates {
		if len(candidate) == 0 {
			continue
		}
		if filepath.IsAbs(candidate) {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			} else {
				lastErr = err
				continue
			}
		}
		resolved, err := p.lookPath(candidate)
		if err == nil {
			return resolved, nil
		}
		lastErr = err
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no executable candidates provided")
	}
	return "", lastErr
}

func sudoersFragmentName(roleName string, command []string, grantID string) (string, error) {
	base := sanitizeFragmentComponent(roleName)
	if len(base) == 0 {
		base = "local-sudo"
	}
	grant := sanitizeFragmentComponent(grantID)
	if len(grant) == 0 {
		return "", fmt.Errorf("grant id is required")
	}

	if len(command) == 0 {
		return fmt.Sprintf("thand-%s-%s", base, grant), nil
	}

	hash := sha256.Sum256([]byte(strings.Join(command, "\x00")))
	return fmt.Sprintf("thand-%s-%s-%s", base, grant, hex.EncodeToString(hash[:])[:10]), nil
}

func sanitizeFragmentComponent(value string) string {
	value = strings.ToLower(value)
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	return strings.Trim(builder.String(), "-")
}

func sudoersCommandSpec(command []string) (string, error) {
	if len(command) == 0 {
		return "", fmt.Errorf("command is required")
	}

	escaped := make([]string, 0, len(command))
	for _, part := range command {
		if len(part) == 0 {
			return "", fmt.Errorf("command arguments must not be empty")
		}
		replacer := strings.NewReplacer(
			`\\`, `\\\\`,
			" ", `\ `,
			",", `\,`,
			":", `\:`,
			"=", `\=`,
		)
		escaped = append(escaped, replacer.Replace(part))
	}

	return strings.Join(escaped, " "), nil
}

func decodeLocalSudoRequestMetadata(value *models.CompositeRole) (models.LocalSudoRequestMetadata, error) {
	return models.DecodeLocalSudoRequest(value)
}

func decodeLocalSudoAuthorizationMetadata(value *models.CompositeRole) (models.LocalSudoAuthorizationMetadata, error) {
	return models.DecodeLocalSudoAuthorization(value)
}

func runSystemCommand(name string, args ...string) (commandResult, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()

	result := commandResult{
		Stdout: string(output),
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		result.Stderr = string(exitErr.Stderr)
		return result, err
	}

	if err != nil {
		return result, err
	}

	return result, nil
}

func init() {
	providers.Register(LocalProviderName, &localProvider{}, LocalCapabilities, &ConfigSchema{})
}
