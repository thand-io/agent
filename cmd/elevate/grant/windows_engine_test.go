package grant

import (
	"context"
	"errors"
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
			if name != "powershell" {
				t.Fatalf("unexpected command name %q", name)
			}
			if len(args) < 5 {
				t.Fatalf("unexpected command args: %+v", args)
			}
			switch args[3] {
			case windowsMembershipScript:
				if args[4] != "Administrators" {
					t.Fatalf("unexpected membership group arg: %q", args[4])
				}
				return []byte("[]"), nil
			case windowsAddMemberScript:
				if args[4] != "Administrators" || args[5] != "alice" {
					t.Fatalf("unexpected add args: %+v", args)
				}
				addCalled = true
				return []byte("ok"), nil
			default:
				t.Fatalf("unexpected script: %q", args[3])
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
			if name != "powershell" {
				t.Fatalf("unexpected command name %q", name)
			}
			if args[3] != windowsMembershipScript {
				t.Fatalf("unexpected script: %q", args[3])
			}
			return []byte(`[{"Name":"BUILTIN\\Administrators","ObjectClass":"Group","PrincipalSource":"Local"},{"Name":"alice","ObjectClass":"User","PrincipalSource":"Local"}]`), nil
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
			if name != "powershell" {
				t.Fatalf("unexpected command name %q", name)
			}
			if args[3] != windowsMembershipScript {
				t.Fatalf("unexpected script: %q", args[3])
			}
			return []byte(`[{"Name":"Administrator","ObjectClass":"User","PrincipalSource":"Local"}]`), nil
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
			if name != "powershell" {
				t.Fatalf("unexpected command name: %q", name)
			}
			call++
			switch call {
			case 1:
				if args[3] != windowsRemoveMemberScript {
					t.Fatalf("unexpected script: %q", args[3])
				}
				if args[4] != "Administrators" || args[5] != "alice" {
					t.Fatalf("unexpected delete args: %+v", args)
				}
				deleted = true
				return []byte("ok"), nil
			default:
				t.Fatalf("unexpected extra command: %+v", args)
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
