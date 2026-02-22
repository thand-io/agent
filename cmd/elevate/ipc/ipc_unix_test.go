//go:build linux || darwin

package ipc

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thand-io/agent/cmd/elevate/handler"
)

type fakeListener struct {
	acceptConn *net.UnixConn
	acceptErr  error
	closed     bool
	acceptFn   func() (*net.UnixConn, error)
	deadlineFn func(time.Time) error
}

// fakeListener is a stub for unixListener so tests can drive
// accept/deadline paths without creating a real filesystem-backed Unix socket.
func (f *fakeListener) AcceptUnix() (*net.UnixConn, error) {
	if f.acceptFn != nil {
		return f.acceptFn()
	}
	return f.acceptConn, f.acceptErr
}

func (f *fakeListener) SetDeadline(t time.Time) error {
	if f.deadlineFn != nil {
		return f.deadlineFn(t)
	}
	_ = t
	return nil
}

func (f *fakeListener) Close() error {
	f.closed = true
	return nil
}

type fakeIPCConn struct{}

func (f *fakeIPCConn) ReadFrame(ctx context.Context) ([]byte, error) {
	_ = ctx
	return nil, nil
}

func (f *fakeIPCConn) WriteFrame(ctx context.Context, data []byte) error {
	_ = ctx
	_ = data
	return nil
}

func (f *fakeIPCConn) Close() error { return nil }

func mustUnixServer(t *testing.T, path string, opts ...Option) *UnixServer {
	t.Helper()
	srvAny, err := NewUnixServer(path, opts...)
	if err != nil {
		t.Fatalf("NewUnixServer failed: %v", err)
	}
	return srvAny.(*UnixServer)
}

func TestNewUnixServer(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{name: "empty path", path: "", wantErr: ErrSocketPathRequired},
		{name: "valid path", path: "/tmp/elevate.sock", wantErr: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewUnixServer(tt.path)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got == nil {
					t.Fatal("expected non-nil server")
				}
				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("unexpected error: got %v want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewUnixServerRejectsInvalidMaxFrame(t *testing.T) {
	_, err := NewUnixServer("/tmp/elevate.sock", WithMaxFrameBytes(0))
	if err == nil {
		t.Fatal("expected error for non-positive max frame size")
	}
}

func TestRemoveStaleSocket(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, srv *UnixServer) string
		wantErr bool
	}{
		{
			name: "missing path",
			setup: func(t *testing.T, srv *UnixServer) string {
				_ = srv
				return filepath.Join(t.TempDir(), "does-not-exist.sock")
			},
			wantErr: false,
		},
		{
			name: "reject non-socket file",
			setup: func(t *testing.T, srv *UnixServer) string {
				_ = srv
				path := filepath.Join(t.TempDir(), "not-a-socket")
				if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
					t.Fatalf("write file failed: %v", err)
				}
				return path
			},
			wantErr: true,
		},
	}

	srv := mustUnixServer(t, "/tmp/elevate.sock")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t, srv)
			err := srv.removeStaleSocket(path)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestNormalizeReadFrame(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trim CRLF", in: "hello\r\n", want: "hello"},
		{name: "trim LF", in: "hello\n", want: "hello"},
		{name: "no newline", in: "hello", want: "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeReadFrame([]byte(tt.in))
			if string(got) != tt.want {
				t.Fatalf("unexpected normalized frame: got %q want %q", string(got), tt.want)
			}
		})
	}
}

func TestNormalizeWriteFrame(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "append newline", in: "hello", want: "hello\n"},
		{name: "keep newline", in: "hello\n", want: "hello\n"},
		{name: "empty frame", in: "", want: "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeWriteFrame([]byte(tt.in))
			if string(got) != tt.want {
				t.Fatalf("unexpected normalized frame: got %q want %q", string(got), tt.want)
			}
		})
	}
}

func TestNormalizeWriteFrameDoesNotMutateInput(t *testing.T) {
	src := make([]byte, 5, 8)
	copy(src, []byte("hello"))

	got := normalizeWriteFrame(src)
	if string(got) != "hello\n" {
		t.Fatalf("unexpected normalized frame: got %q want %q", string(got), "hello\\n")
	}
	if string(src) != "hello" {
		t.Fatalf("input slice was mutated: got %q want %q", string(src), "hello")
	}
}

func TestStartReturnsMkdirError(t *testing.T) {
	srv := mustUnixServer(t, filepath.Join(t.TempDir(), "elevate.sock"),
		WithMkdirAll(func(path string, perm os.FileMode) error {
			_ = path
			_ = perm
			return errors.New("mkdir failed")
		}),
	)

	if err := srv.Start(context.Background()); err == nil {
		t.Fatal("expected Start to fail")
	}
}

func TestStartReturnsListenError(t *testing.T) {
	srv := mustUnixServer(t, filepath.Join(t.TempDir(), "elevate.sock"),
		WithListenUnix(func(network string, laddr *net.UnixAddr) (unixListener, error) {
			_ = network
			_ = laddr
			return nil, errors.New("listen failed")
		}),
	)

	if err := srv.Start(context.Background()); err == nil {
		t.Fatal("expected Start to fail")
	}
}

func TestStartChmodFailureClosesListener(t *testing.T) {
	fl := &fakeListener{}
	srv := mustUnixServer(t, filepath.Join(t.TempDir(), "elevate.sock"),
		WithListenUnix(func(network string, laddr *net.UnixAddr) (unixListener, error) {
			_ = network
			_ = laddr
			return fl, nil
		}),
		WithChmod(func(path string, mode os.FileMode) error {
			_ = path
			_ = mode
			return errors.New("chmod failed")
		}),
	)

	if err := srv.Start(context.Background()); err == nil {
		t.Fatal("expected Start to fail")
	}
	if !fl.closed {
		t.Fatal("expected listener to be closed on chmod failure")
	}
	if srv.listener != nil {
		t.Fatal("expected listener to be cleared on chmod failure")
	}
}

func TestStartAppliesSocketGIDOwnership(t *testing.T) {
	fl := &fakeListener{}
	var chownCalls int
	var sawDir bool
	var sawSocket bool
	socketPath := filepath.Join(t.TempDir(), "elevate.sock")

	srv := mustUnixServer(t, socketPath,
		WithSocketGID(1234),
		WithListenUnix(func(network string, laddr *net.UnixAddr) (unixListener, error) {
			_ = network
			_ = laddr
			return fl, nil
		}),
		WithChmod(func(path string, mode os.FileMode) error {
			_ = path
			_ = mode
			return nil
		}),
		WithChown(func(name string, uid, gid int) error {
			chownCalls++
			if uid != 0 || gid != 1234 {
				t.Fatalf("unexpected chown ownership uid=%d gid=%d", uid, gid)
			}
			if name == filepath.Dir(socketPath) {
				sawDir = true
			}
			if name == socketPath {
				sawSocket = true
			}
			return nil
		}),
	)

	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if chownCalls != 2 || !sawDir || !sawSocket {
		t.Fatalf("unexpected chown calls: count=%d dir=%v socket=%v", chownCalls, sawDir, sawSocket)
	}
}

func TestStartSocketDirChownFailure(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "elevate.sock")
	srv := mustUnixServer(t, socketPath,
		WithSocketGID(1234),
		WithChown(func(name string, uid, gid int) error {
			_ = name
			_ = uid
			_ = gid
			return errors.New("chown dir failed")
		}),
	)
	if err := srv.Start(context.Background()); err == nil {
		t.Fatal("expected Start to fail")
	}
}

func TestStartSocketChownFailureClosesListener(t *testing.T) {
	fl := &fakeListener{}
	socketPath := filepath.Join(t.TempDir(), "elevate.sock")
	srv := mustUnixServer(t, socketPath,
		WithSocketGID(1234),
		WithListenUnix(func(network string, laddr *net.UnixAddr) (unixListener, error) {
			_ = network
			_ = laddr
			return fl, nil
		}),
		WithChown(func(name string, uid, gid int) error {
			_ = uid
			_ = gid
			if name == socketPath {
				return errors.New("chown socket failed")
			}
			return nil
		}),
	)
	if err := srv.Start(context.Background()); err == nil {
		t.Fatal("expected Start to fail")
	}
	if !fl.closed {
		t.Fatal("expected listener to close on socket chown failure")
	}
	if srv.listener != nil {
		t.Fatal("expected listener to be cleared on socket chown failure")
	}
}

func TestCloseReturnsRemoveError(t *testing.T) {
	srv := mustUnixServer(t, "/tmp/elevate.sock",
		WithRemove(func(path string) error {
			_ = path
			return errors.New("remove failed")
		}),
	)

	if err := srv.Close(); err == nil {
		t.Fatal("expected Close to fail on remove error")
	}
}

func TestHandleAcceptResult(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name       string
		ctx        context.Context
		res        acceptResult
		wantErr    bool
		errIs      error
		errContain string
	}{
		{
			name:       "generic error gets wrapped",
			ctx:        context.Background(),
			res:        acceptResult{err: errors.New("boom")},
			wantErr:    true,
			errContain: "accept unix connection",
		},
		{
			name:    "closed listener with canceled context returns canceled",
			ctx:     cancelledCtx,
			res:     acceptResult{err: net.ErrClosed},
			wantErr: true,
			errIs:   context.Canceled,
		},
		{
			name:    "closed listener without canceled context returns net.ErrClosed",
			ctx:     context.Background(),
			res:     acceptResult{err: net.ErrClosed},
			wantErr: true,
			errIs:   net.ErrClosed,
		},
	}

	srv := mustUnixServer(t, "/tmp/elevate.sock")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := srv.handleAcceptResult(tt.ctx, tt.res)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.errIs != nil && !errors.Is(err, tt.errIs) {
				t.Fatalf("unexpected error: got %v want errors.Is(...,%v)=true", err, tt.errIs)
			}
			if tt.errContain != "" && (err == nil || !strings.Contains(err.Error(), tt.errContain)) {
				t.Fatalf("expected error containing %q, got %v", tt.errContain, err)
			}
		})
	}
}

func TestAcceptReturnsErrServerNotStarted(t *testing.T) {
	srv := mustUnixServer(t, "/tmp/elevate.sock")

	_, err := srv.Accept(context.Background())
	if !errors.Is(err, ErrServerNotStarted) {
		t.Fatalf("expected ErrServerNotStarted, got: %v", err)
	}
}

func TestAcceptSuccessReturnsConn(t *testing.T) {
	srv := mustUnixServer(t, "/tmp/elevate.sock",
		WithNewConn(func(conn *net.UnixConn) handler.IPCConn {
			_ = conn
			return &fakeIPCConn{}
		}),
	)

	fl := &fakeListener{acceptConn: nil, acceptErr: nil}
	srv.listener = fl

	conn, err := srv.Accept(context.Background())
	if err != nil {
		t.Fatalf("Accept failed: %v", err)
	}
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
}

func TestAcceptCanceledContextUnblocksAccept(t *testing.T) {
	// Keep this as a standalone scenario: it validates cancellation choreography
	// between SetDeadline-based unblock and the in-flight accept goroutine.
	release := make(chan struct{})
	fl := &fakeListener{
		acceptFn: func() (*net.UnixConn, error) {
			<-release
			return nil, net.ErrClosed
		},
		deadlineFn: func(t time.Time) error {
			_ = t
			close(release)
			return nil
		},
	}

	srv := mustUnixServer(t, "/tmp/elevate.sock")
	srv.listener = fl

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := srv.Accept(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}
