package grant

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

func TestGrantWritesAndRevokesSudoersFile(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	sudoersFile := writeTempSudoersFile(t, "#includedir "+dir+"\n")

	engine := mustNewEngine(t, LinuxEngineConfig{
		SudoersDir:  dir,
		SudoersFile: sudoersFile,
		VisudoBin:   "visudo",
	},
		WithNow(func() time.Time { return now }),
		WithValidateFile(func(ctx context.Context, path string) error {
			_ = ctx
			if _, err := os.Stat(path); err != nil {
				return err
			}
			return nil
		}),
	)

	res, err := engine.Grant(context.Background(), domain.GrantRequest{
		RequestID:       "abc-123",
		WorkflowID:      "wf-1",
		Username:        "alice",
		DurationSeconds: 600,
	})
	if err != nil {
		t.Fatalf("Grant failed: %v", err)
	}

	if res.RequestID != "abc-123" || res.Username != "alice" {
		t.Fatalf("unexpected grant result: %+v", res)
	}
	if !res.Expiry.Equal(now.Add(600 * time.Second)) {
		t.Fatalf("unexpected expiry: got %s want %s", res.Expiry, now.Add(600*time.Second))
	}

	path := filepath.Join(dir, "thand-abc-123")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sudoers file failed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat sudoers file failed: %v", err)
	}
	if got := info.Mode().Perm(); got != sudoersFileMode {
		t.Fatalf("unexpected sudoers file mode: got %o want %o", got, sudoersFileMode)
	}
	text := string(b)
	if !strings.Contains(text, "request_id: abc-123") || !strings.Contains(text, "alice ALL=(ALL:ALL) NOPASSWD: ALL") {
		t.Fatalf("unexpected sudoers content:\n%s", text)
	}

	if err := engine.Revoke(context.Background(), domain.RevokeRequest{RequestID: "abc-123"}); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected removed sudoers file, stat err=%v", err)
	}
}

func TestGrantValidationFailureRollsBackFile(t *testing.T) {
	dir := t.TempDir()
	sudoersFile := writeTempSudoersFile(t, "#includedir "+dir+"\n")
	engine := mustNewEngine(t, LinuxEngineConfig{
		SudoersDir:  dir,
		SudoersFile: sudoersFile,
		VisudoBin:   "visudo",
	},
		WithValidateFile(func(ctx context.Context, path string) error {
			_ = ctx
			_ = path
			return errors.New("invalid sudoers")
		}),
	)

	_, err := engine.Grant(context.Background(), domain.GrantRequest{
		RequestID:       "bad",
		Username:        "bob",
		DurationSeconds: 60,
	})
	if err == nil {
		t.Fatal("expected Grant to fail")
	}

	if _, statErr := os.Stat(filepath.Join(dir, "thand-bad")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected sudoers file rollback, got stat err=%v", statErr)
	}
}

func TestGrantInvalidRequest(t *testing.T) {
	engine := mustNewEngine(t, LinuxEngineConfig{
		SudoersDir:  t.TempDir(),
		SudoersFile: writeTempSudoersFile(t, "#includedir /tmp/unused\n"),
		VisudoBin:   "visudo",
	})

	cases := []domain.GrantRequest{
		{RequestID: "", Username: "alice", DurationSeconds: 10},
		{RequestID: "bad/id", Username: "alice", DurationSeconds: 10},
		{RequestID: "bad\nid", Username: "alice", DurationSeconds: 10},
		{RequestID: " req", Username: "alice", DurationSeconds: 10},
		{RequestID: "req ", Username: "alice", DurationSeconds: 10},
		{RequestID: "x", Username: "", DurationSeconds: 10},
		{RequestID: "x", Username: "bad user", DurationSeconds: 10},
		{RequestID: "x", Username: "bad\nuser", DurationSeconds: 10},
		{RequestID: "x", Username: " alice", DurationSeconds: 10},
		{RequestID: "x", Username: "alice ", DurationSeconds: 10},
		{RequestID: "x", Username: "alice", DurationSeconds: 0},
	}
	for _, c := range cases {
		if _, err := engine.Grant(context.Background(), c); !errors.Is(err, ErrInvalidGrantRequest) {
			t.Fatalf("expected ErrInvalidGrantRequest for %+v, got %v", c, err)
		}
	}
}

func TestRevokeMissingIsIdempotent(t *testing.T) {
	engine := mustNewEngine(t, LinuxEngineConfig{
		SudoersDir:  t.TempDir(),
		SudoersFile: writeTempSudoersFile(t, "#includedir /tmp/unused\n"),
		VisudoBin:   "visudo",
	})
	if err := engine.Revoke(context.Background(), domain.RevokeRequest{RequestID: "missing"}); err != nil {
		t.Fatalf("expected nil on missing revoke, got %v", err)
	}
}

func TestRevokeInvalidRequestID(t *testing.T) {
	engine := mustNewEngine(t, LinuxEngineConfig{
		SudoersDir:  t.TempDir(),
		SudoersFile: writeTempSudoersFile(t, "#includedir /tmp/unused\n"),
		VisudoBin:   "visudo",
	})
	if err := engine.Revoke(context.Background(), domain.RevokeRequest{RequestID: "bad/id"}); !errors.Is(err, ErrInvalidRevokeRequest) {
		t.Fatalf("expected ErrInvalidRevokeRequest, got %v", err)
	}
}

func TestBaselinePrivilegeHook(t *testing.T) {
	dir := t.TempDir()
	sudoersFile := writeTempSudoersFile(t, "#includedir "+dir+"\n")
	engine := mustNewEngine(t, LinuxEngineConfig{
		SudoersDir:  dir,
		SudoersFile: sudoersFile,
		VisudoBin:   "visudo",
	},
		WithValidateFile(func(ctx context.Context, path string) error { _ = ctx; _ = path; return nil }),
		WithCheckAlreadyPrivileged(func(ctx context.Context, username string) (bool, error) {
			_ = ctx
			_ = username
			return true, nil
		}),
	)

	res, err := engine.Grant(context.Background(), domain.GrantRequest{RequestID: "r1", Username: "alice", DurationSeconds: 30})
	if err != nil {
		t.Fatalf("Grant failed: %v", err)
	}
	if !res.WasAlreadyPrivileged {
		t.Fatal("expected WasAlreadyPrivileged=true from hook")
	}
}

func TestGrantFailsWhenSudoersDirMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	sudoersFile := writeTempSudoersFile(t, "#includedir "+dir+"\n")
	engine := mustNewEngine(t, LinuxEngineConfig{
		SudoersDir:  dir,
		SudoersFile: sudoersFile,
		VisudoBin:   "visudo",
	})

	_, err := engine.Grant(context.Background(), domain.GrantRequest{
		RequestID:       "r1",
		Username:        "alice",
		DurationSeconds: 60,
	})
	if err == nil || !strings.Contains(err.Error(), "sudoers dir is not accessible") {
		t.Fatalf("expected missing dir error, got %v", err)
	}
}

func TestGrantFailsWhenSudoersMissingIncludedir(t *testing.T) {
	dir := t.TempDir()
	sudoersFile := writeTempSudoersFile(t, "#includedir /some/other/dir\n")
	engine := mustNewEngine(t, LinuxEngineConfig{
		SudoersDir:  dir,
		SudoersFile: sudoersFile,
		VisudoBin:   "visudo",
	})

	_, err := engine.Grant(context.Background(), domain.GrantRequest{
		RequestID:       "r1",
		Username:        "alice",
		DurationSeconds: 60,
	})
	if err == nil || !strings.Contains(err.Error(), "missing #includedir") {
		t.Fatalf("expected missing include error, got %v", err)
	}
}

func writeTempSudoersFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sudoers")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp sudoers file: %v", err)
	}
	return path
}

func mustNewEngine(t *testing.T, cfg LinuxEngineConfig, opts ...EngineOption) *LinuxEngine {
	t.Helper()
	engine, err := NewLinuxEngine(cfg, opts...)
	if err != nil {
		t.Fatalf("NewLinuxEngine failed: %v", err)
	}
	return engine.(*LinuxEngine)
}

func TestNewLinuxEngineRequiresConfigFields(t *testing.T) {
	_, err := NewLinuxEngine(LinuxEngineConfig{})
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}
