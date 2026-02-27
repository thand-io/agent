package grant

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

func TestWindowsGrantAddsUser(t *testing.T) {
	now := time.Date(2026, 2, 23, 20, 0, 0, 0, time.UTC)
	var addCalled bool

	engineAny, err := NewWindowsEngine(WindowsEngineConfig{},
		WithWindowsNow(func() time.Time { return now }),
		WithWindowsRunCommand(func(ctx context.Context, name string, args ...string) ([]byte, error) {
			_ = ctx
			if name != "net" {
				t.Fatalf("unexpected command name %q", name)
			}
			joined := strings.Join(args, " ")
			switch joined {
			case "localgroup Administrators":
				return []byte("Administrator\r\n"), nil
			case "localgroup Administrators alice /add":
				addCalled = true
				return []byte("ok"), nil
			default:
				t.Fatalf("unexpected command args: %q", joined)
				return nil, nil
			}
		}),
	)
	if err != nil {
		t.Fatalf("NewWindowsEngine failed: %v", err)
	}
	engine := engineAny.(*WindowsEngine)

	res, err := engine.Grant(context.Background(), domain.GrantRequest{
		RequestID:       "req-1",
		Username:        "alice",
		DurationSeconds: 600,
	})
	if err != nil {
		t.Fatalf("Grant failed: %v", err)
	}
	if !addCalled {
		t.Fatal("expected add command to be called")
	}
	if res.WasAlreadyPrivileged {
		t.Fatal("expected WasAlreadyPrivileged=false")
	}
	if !res.Expiry.Equal(now.Add(600 * time.Second)) {
		t.Fatalf("unexpected expiry: got %s", res.Expiry)
	}
}

func TestWindowsGrantAlreadyMember(t *testing.T) {
	engineAny, err := NewWindowsEngine(WindowsEngineConfig{},
		WithWindowsRunCommand(func(ctx context.Context, name string, args ...string) ([]byte, error) {
			_ = ctx
			_ = name
			_ = args
			return []byte("BUILTIN\\Administrators\r\nalice\r\n"), nil
		}),
	)
	if err != nil {
		t.Fatalf("NewWindowsEngine failed: %v", err)
	}
	engine := engineAny.(*WindowsEngine)

	res, err := engine.Grant(context.Background(), domain.GrantRequest{
		RequestID:       "req-1",
		Username:        "alice",
		DurationSeconds: 60,
	})
	if err != nil {
		t.Fatalf("Grant failed: %v", err)
	}
	if !res.WasAlreadyPrivileged {
		t.Fatal("expected WasAlreadyPrivileged=true")
	}
}

func TestWindowsRevokeIdempotentWhenMissing(t *testing.T) {
	engineAny, err := NewWindowsEngine(WindowsEngineConfig{},
		WithWindowsRunCommand(func(ctx context.Context, name string, args ...string) ([]byte, error) {
			_ = ctx
			_ = name
			_ = args
			return []byte("Administrator\r\n"), nil
		}),
	)
	if err != nil {
		t.Fatalf("NewWindowsEngine failed: %v", err)
	}
	engine := engineAny.(*WindowsEngine)

	if err := engine.Revoke(context.Background(), domain.RevokeRequest{
		RequestID: "req-1",
		Username:  "alice",
	}); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}
}

func TestWindowsRevokeRemovesUser(t *testing.T) {
	var deleted bool
	call := 0

	engineAny, err := NewWindowsEngine(WindowsEngineConfig{},
		WithWindowsRunCommand(func(ctx context.Context, name string, args ...string) ([]byte, error) {
			_ = ctx
			if name != "net" {
				t.Fatalf("unexpected command name: %q", name)
			}
			call++
			joined := strings.Join(args, " ")
			switch call {
			case 1:
				if joined != "localgroup Administrators alice /delete" {
					t.Fatalf("unexpected delete command args: %q", joined)
				}
				deleted = true
				return []byte("ok"), nil
			default:
				t.Fatalf("unexpected extra command: %q", joined)
				return nil, nil
			}
		}),
	)
	if err != nil {
		t.Fatalf("NewWindowsEngine failed: %v", err)
	}
	engine := engineAny.(*WindowsEngine)

	engine.isMember = func(context.Context, string) (bool, error) { return true, nil }

	if err := engine.Revoke(context.Background(), domain.RevokeRequest{
		RequestID: "req-1",
		Username:  "alice",
	}); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}
	if !deleted {
		t.Fatal("expected delete command to be called")
	}
}

func TestWindowsGrantInvalidRequest(t *testing.T) {
	engineAny, err := NewWindowsEngine(WindowsEngineConfig{})
	if err != nil {
		t.Fatalf("NewWindowsEngine failed: %v", err)
	}
	engine := engineAny.(*WindowsEngine)

	_, err = engine.Grant(context.Background(), domain.GrantRequest{
		RequestID:       "",
		Username:        "alice",
		DurationSeconds: 30,
	})
	if !errors.Is(err, ErrInvalidGrantRequest) {
		t.Fatalf("expected ErrInvalidGrantRequest, got %v", err)
	}
}
