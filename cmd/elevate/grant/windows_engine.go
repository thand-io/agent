package grant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/thand-io/agent/cmd/elevate/domain"
	"github.com/thand-io/agent/cmd/elevate/handler"
	"github.com/thand-io/agent/cmd/elevate/identity"
)

const defaultWindowsAdminGroup = "Administrators"

const (
	windowsMembershipScript = `& {
param($group)
ConvertTo-Json -InputObject @(
Get-LocalGroupMember -Group $group |
	Select-Object Name,ObjectClass
) -Compress
} `
	windowsAddMemberScript = `& {
param($group, $member)
Add-LocalGroupMember -Group $group -Member $member
} `
	windowsRemoveMemberScript = `& {
param($group, $member)
Remove-LocalGroupMember -Group $group -Member $member
} `
)

type windowsLocalGroupMember struct {
	Name        string `json:"Name"`
	ObjectClass string `json:"ObjectClass"`
}

// WindowsEngineOption configures a WindowsEngine instance.
type WindowsEngineOption func(*WindowsEngine)

// WithWindowsNow overrides the wall clock source.
func WithWindowsNow(fn func() time.Time) WindowsEngineOption {
	return func(e *WindowsEngine) {
		e.now = fn
	}
}

// WithWindowsRunCommand overrides command execution for tests.
func WithWindowsRunCommand(fn func(context.Context, string, ...string) ([]byte, error)) WindowsEngineOption {
	return func(e *WindowsEngine) {
		e.runCommand = fn
	}
}

// WithWindowsComputerName overrides the local computer name used for exact local-principal matching.
func WithWindowsComputerName(name string) WindowsEngineOption {
	return func(e *WindowsEngine) {
		e.computerName = strings.TrimSpace(name)
	}
}

// WindowsEngineConfig contains Windows grant engine configuration.
type WindowsEngineConfig struct {
	AdminGroup string
}

// WindowsEngine implements local-admin grant/revoke via local Administrators group membership.
type WindowsEngine struct {
	adminGroup   string
	computerName string
	now          func() time.Time
	runCommand   func(context.Context, string, ...string) ([]byte, error)
	isMember     func(context.Context, string) (bool, error)
}

// NewWindowsEngine constructs a Windows GrantEngine backed by PowerShell local-group cmdlets.
func NewWindowsEngine(cfg WindowsEngineConfig, opts ...WindowsEngineOption) (handler.GrantEngine, error) {
	adminGroup := strings.TrimSpace(cfg.AdminGroup)
	if adminGroup == "" {
		adminGroup = defaultWindowsAdminGroup
	}
	if !identity.ValidWindowsAdminGroup(adminGroup) {
		return nil, errors.New("windows admin group is invalid")
	}

	computerName := strings.TrimSpace(os.Getenv("COMPUTERNAME"))
	if computerName == "" {
		var err error
		computerName, err = os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("resolve windows computer name: %w", err)
		}
		computerName = strings.TrimSpace(computerName)
	}
	if computerName == "" {
		return nil, errors.New("windows computer name is required")
	}

	e := &WindowsEngine{
		adminGroup:   adminGroup,
		computerName: computerName,
		now:          func() time.Time { return time.Now().UTC() },
		runCommand: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, name, args...)
			return cmd.CombinedOutput()
		},
	}
	e.isMember = e.checkMembership

	for _, opt := range opts {
		opt(e)
	}

	return e, nil
}

// Grant adds the user to the configured local admin group.
func (e *WindowsEngine) Grant(ctx context.Context, req domain.GrantRequest) (domain.GrantResult, error) {
	if !isValidRequestID(req.RequestID) || !isValidWindowsUsername(req.Username) || req.DurationSeconds <= 0 {
		return domain.GrantResult{}, ErrInvalidGrantRequest
	}

	alreadyMember, err := e.isMember(ctx, req.Username)
	if err != nil {
		return domain.GrantResult{}, err
	}
	if !alreadyMember {
		if _, err := e.runPowerShell(ctx, windowsAddMemberScript, e.adminGroup, req.Username); err != nil {
			// If membership changed between check and add, treat as already privileged.
			raceMember, raceErr := e.isMember(ctx, req.Username)
			if raceErr != nil {
				return domain.GrantResult{}, fmt.Errorf("add windows admin group member: %w", err)
			}
			if !raceMember {
				return domain.GrantResult{}, fmt.Errorf("add windows admin group member: %w", err)
			}
			alreadyMember = true
		}
	}

	return domain.GrantResult{
		RequestID:            req.RequestID,
		Username:             req.Username,
		Expiry:               e.now().Add(time.Duration(req.DurationSeconds) * time.Second),
		WasAlreadyPrivileged: alreadyMember,
	}, nil
}

// Revoke removes the user from the configured local admin group.
func (e *WindowsEngine) Revoke(ctx context.Context, req domain.RevokeRequest) error {
	if !isValidRequestID(req.RequestID) || !isValidWindowsUsername(req.Username) {
		return ErrInvalidRevokeRequest
	}

	member, err := e.isMember(ctx, req.Username)
	if err != nil {
		return err
	}
	if !member {
		return nil
	}

	if _, err := e.runPowerShell(ctx, windowsRemoveMemberScript, e.adminGroup, req.Username); err != nil {
		memberAfter, checkErr := e.isMember(ctx, req.Username)
		if checkErr != nil {
			return fmt.Errorf("remove windows admin group member: %w", err)
		}
		if memberAfter {
			return fmt.Errorf("remove windows admin group member: %w", err)
		}
	}
	return nil
}

func (e *WindowsEngine) checkMembership(ctx context.Context, username string) (bool, error) {
	out, err := e.runPowerShell(ctx, windowsMembershipScript, e.adminGroup)
	if err != nil {
		return false, fmt.Errorf("list windows admin group members: %w", err)
	}

	var members []windowsLocalGroupMember
	if len(strings.TrimSpace(string(out))) == 0 {
		return false, nil
	}
	if err := json.Unmarshal(out, &members); err != nil {
		return false, fmt.Errorf("decode windows admin group members: %w", err)
	}

	for _, member := range members {
		if windowsUsernameMatches(member.Name, username, e.computerName) {
			return true, nil
		}
	}
	return false, nil
}

func (e *WindowsEngine) runPowerShell(ctx context.Context, script string, args ...string) ([]byte, error) {
	psArgs := []string{"-NoProfile", "-NonInteractive", "-Command", script}
	psArgs = append(psArgs, args...)
	return e.runCommand(ctx, "powershell", psArgs...)
}

func isValidWindowsUsername(v string) bool {
	return identity.ValidAccountName(v)
}

func windowsUsernameMatches(line, username string, computerName string) bool {
	lhs := strings.ToLower(strings.TrimSpace(line))
	rhs := strings.ToLower(strings.TrimSpace(username))
	localHost := strings.ToLower(strings.TrimSpace(computerName))
	if lhs == rhs {
		return true
	}

	prefix, suffix, ok := strings.Cut(lhs, `\`)
	if ok {
		return prefix == localHost && suffix == rhs
	}
	return false
}

var _ handler.GrantEngine = (*WindowsEngine)(nil)
