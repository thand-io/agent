package grant

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/thand-io/agent/cmd/elevate/domain"
	"github.com/thand-io/agent/cmd/elevate/handler"
)

const defaultWindowsAdminGroup = "Administrators"

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

// WindowsEngineConfig contains Windows grant engine configuration.
type WindowsEngineConfig struct {
	AdminGroup string
}

// WindowsEngine implements local-admin grant/revoke via local Administrators group membership.
type WindowsEngine struct {
	adminGroup string
	now        func() time.Time
	runCommand func(context.Context, string, ...string) ([]byte, error)
	isMember   func(context.Context, string) (bool, error)
}

// NewWindowsEngine constructs a Windows GrantEngine backed by `net localgroup`.
func NewWindowsEngine(cfg WindowsEngineConfig, opts ...WindowsEngineOption) (handler.GrantEngine, error) {
	adminGroup := strings.TrimSpace(cfg.AdminGroup)
	if adminGroup == "" {
		adminGroup = defaultWindowsAdminGroup
	}

	e := &WindowsEngine{
		adminGroup: adminGroup,
		now:        func() time.Time { return time.Now().UTC() },
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
		if _, err := e.runCommand(ctx, "net", "localgroup", e.adminGroup, req.Username, "/add"); err != nil {
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

	if _, err := e.runCommand(ctx, "net", "localgroup", e.adminGroup, req.Username, "/delete"); err != nil {
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
	out, err := e.runCommand(ctx, "net", "localgroup", e.adminGroup)
	if err != nil {
		return false, fmt.Errorf("list windows admin group members: %w", err)
	}

	lines := bytes.Split(out, []byte{'\n'})
	for _, raw := range lines {
		line := strings.TrimSpace(strings.TrimRight(string(raw), "\r"))
		if line == "" {
			continue
		}
		if windowsUsernameMatches(line, username) {
			return true, nil
		}
	}
	return false, nil
}

func isValidWindowsUsername(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	if strings.ContainsAny(v, "\r\n") {
		return false
	}
	return true
}

func windowsUsernameMatches(line, username string) bool {
	lhs := strings.ToLower(strings.TrimSpace(line))
	rhs := strings.ToLower(strings.TrimSpace(username))
	if lhs == rhs {
		return true
	}
	if strings.HasSuffix(lhs, `\`+rhs) {
		return true
	}
	return false
}

var _ handler.GrantEngine = (*WindowsEngine)(nil)
