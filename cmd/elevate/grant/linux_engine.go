package grant

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/thand-io/agent/cmd/elevate/domain"
	"github.com/thand-io/agent/cmd/elevate/handler"
)

const (
	// DefaultSudoersDir is the default directory where grant files are written.
	DefaultSudoersDir  = "/etc/sudoers.d"
	// DefaultSudoersFile is the default sudoers file checked for #includedir.
	DefaultSudoersFile = "/etc/sudoers"
	// DefaultVisudoBin is the default visudo binary used for syntax validation.
	DefaultVisudoBin   = "visudo"
	sudoersFileMode    = 0o440
)

var (
	// ErrInvalidGrantRequest indicates malformed or unsafe grant input.
	ErrInvalidGrantRequest  = errors.New("invalid grant request")
	// ErrInvalidRevokeRequest indicates malformed or unsafe revoke input.
	ErrInvalidRevokeRequest = errors.New("invalid revoke request")

	requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	usernamePattern  = regexp.MustCompile(`^[a-z_][a-z0-9_-]*[$]?$`)
)

// EngineOption configures a LinuxEngine instance.
type EngineOption func(*LinuxEngine)

// WithSudoersDir overrides where temporary sudoers entries are written.
func WithSudoersDir(path string) EngineOption {
	return func(e *LinuxEngine) {
		e.sudoersDir = path
	}
}

// WithVisudoBinary overrides the validation command binary name/path.
func WithVisudoBinary(path string) EngineOption {
	return func(e *LinuxEngine) {
		e.visudoBin = path
	}
}

// WithNow overrides the wall clock source.
func WithNow(fn func() time.Time) EngineOption {
	return func(e *LinuxEngine) {
		e.now = fn
	}
}

// WithValidateFile overrides sudoers validation for tests.
func WithValidateFile(fn func(context.Context, string) error) EngineOption {
	return func(e *LinuxEngine) {
		e.validateFile = fn
	}
}

// WithWriteFile overrides file write behavior for tests.
func WithWriteFile(fn func(name string, data []byte, perm os.FileMode) error) EngineOption {
	return func(e *LinuxEngine) {
		e.writeFile = fn
	}
}

// WithSudoersFile overrides the base sudoers file path for include checks.
func WithSudoersFile(path string) EngineOption {
	return func(e *LinuxEngine) {
		e.sudoersFile = path
	}
}

// WithCheckAlreadyPrivileged provides baseline membership hook; persistence is in state layer.
func WithCheckAlreadyPrivileged(fn func(context.Context, string) (bool, error)) EngineOption {
	return func(e *LinuxEngine) {
		e.checkAlreadyPrivileged = fn
	}
}

// LinuxEngine implements local-admin grant/revoke via sudoers files.
type LinuxEngine struct {
	sudoersDir  string
	sudoersFile string
	visudoBin   string
	now         func() time.Time

	validateFile           func(context.Context, string) error
	writeFile              func(name string, data []byte, perm os.FileMode) error
	checkAlreadyPrivileged func(context.Context, string) (bool, error)
}

// NewLinuxEngine constructs a Linux GrantEngine backed by sudoers.d files.
func NewLinuxEngine(opts ...EngineOption) handler.GrantEngine {
	e := &LinuxEngine{
		sudoersDir:  DefaultSudoersDir,
		sudoersFile: DefaultSudoersFile,
		visudoBin:   DefaultVisudoBin,
		now:         func() time.Time { return time.Now().UTC() },
		writeFile:   os.WriteFile,
		checkAlreadyPrivileged: func(ctx context.Context, username string) (bool, error) {
			_ = ctx
			_ = username
			// Hook only in this chunk; actual baseline persistence happens in state layer.
			return false, nil
		},
	}

	for _, opt := range opts {
		opt(e)
	}

	// Ensure validateFile uses configured visudo binary unless explicitly overridden.
	if e.validateFile == nil {
		e.validateFile = func(ctx context.Context, path string) error {
			cmd := exec.CommandContext(ctx, e.visudoBin, "-cf", path)
			output, err := cmd.CombinedOutput()
			if err != nil {
				trimmed := strings.TrimSpace(string(output))
				if trimmed == "" {
					return fmt.Errorf("visudo validation failed: %w", err)
				}
				return fmt.Errorf("visudo validation failed: %s: %w", trimmed, err)
			}
			return nil
		}
	}

	return e
}

// Grant validates input, checks sudoers preconditions, writes a temporary sudoers
// rule, validates it with visudo, and returns the resulting expiry metadata.
func (e *LinuxEngine) Grant(ctx context.Context, req domain.GrantRequest) (domain.GrantResult, error) {
	if !isValidRequestID(req.RequestID) || !isValidUsername(req.Username) {
		return domain.GrantResult{}, ErrInvalidGrantRequest
	}
	if req.DurationSeconds <= 0 {
		return domain.GrantResult{}, ErrInvalidGrantRequest
	}

	if err := e.ensureSudoersDirExists(); err != nil {
		return domain.GrantResult{}, err
	}
	if err := e.ensureSudoersIncludeDir(); err != nil {
		return domain.GrantResult{}, err
	}

	alreadyPrivileged, err := e.checkAlreadyPrivileged(ctx, req.Username)
	if err != nil {
		return domain.GrantResult{}, fmt.Errorf("check baseline privilege: %w", err)
	}

	sudoersPath := e.sudoersPath(req.RequestID)
	tmpPath := sudoersPath + ".tmp"
	if err := e.writeFile(tmpPath, []byte(e.sudoersContent(req)), 0o600); err != nil {
		return domain.GrantResult{}, fmt.Errorf("write sudoers temp file: %w", err)
	}

	if err := os.Rename(tmpPath, sudoersPath); err != nil {
		_ = os.Remove(tmpPath)
		return domain.GrantResult{}, fmt.Errorf("activate sudoers file: %w", err)
	}

	if err := os.Chmod(sudoersPath, sudoersFileMode); err != nil {
		_ = os.Remove(sudoersPath)
		return domain.GrantResult{}, fmt.Errorf("set sudoers permissions: %w", err)
	}

	if err := e.validateFile(ctx, sudoersPath); err != nil {
		_ = os.Remove(sudoersPath)
		return domain.GrantResult{}, err
	}

	return domain.GrantResult{
		RequestID:            req.RequestID,
		Username:             req.Username,
		Expiry:               e.now().Add(time.Duration(req.DurationSeconds) * time.Second),
		WasAlreadyPrivileged: alreadyPrivileged,
	}, nil
}

// Revoke removes the sudoers rule for a request and is idempotent when the
// target file does not exist.
func (e *LinuxEngine) Revoke(ctx context.Context, req domain.RevokeRequest) error {
	_ = ctx
	if !isValidRequestID(req.RequestID) {
		return ErrInvalidRevokeRequest
	}

	sudoersPath := e.sudoersPath(req.RequestID)
	if err := os.Remove(sudoersPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("remove sudoers file: %w", err)
	}

	return nil
}

func (e *LinuxEngine) sudoersPath(requestID string) string {
	return filepath.Join(e.sudoersDir, "thand-"+requestID)
}

func (e *LinuxEngine) sudoersContent(req domain.GrantRequest) string {
	expires := e.now().Add(time.Duration(req.DurationSeconds) * time.Second).Format(time.RFC3339)
	return fmt.Sprintf("# thand-agent temporary elevation\n# request_id: %s\n# expires: %s\n%s ALL=(ALL:ALL) NOPASSWD: ALL\n",
		req.RequestID,
		expires,
		req.Username,
	)
}

func isValidRequestID(v string) bool {
	return requestIDPattern.MatchString(strings.TrimSpace(v))
}

func isValidUsername(v string) bool {
	return usernamePattern.MatchString(strings.TrimSpace(v))
}

func (e *LinuxEngine) ensureSudoersDirExists() error {
	info, err := os.Stat(e.sudoersDir)
	if err != nil {
		return fmt.Errorf("sudoers dir is not accessible %q: %w", e.sudoersDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("sudoers dir is not a directory: %q", e.sudoersDir)
	}
	return nil
}

func (e *LinuxEngine) ensureSudoersIncludeDir() error {
	content, err := os.ReadFile(e.sudoersFile)
	if err != nil {
		return fmt.Errorf("read sudoers file %q: %w", e.sudoersFile, err)
	}

	if !hasIncludedir(string(content), e.sudoersDir) {
		return fmt.Errorf("sudoers file %q missing #includedir for %q", e.sudoersFile, e.sudoersDir)
	}

	return nil
}

func hasIncludedir(content string, sudoersDir string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#includedir") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		included := strings.Trim(fields[1], `"'`)
		if included == sudoersDir {
			return true
		}
	}
	return false
}
