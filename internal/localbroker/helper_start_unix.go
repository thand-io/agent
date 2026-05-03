//go:build unix

package localbroker

import (
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

func newDefaultHelperStarter() helperStarter {
	return func(ctx context.Context, executable string, args []string) (*helperSession, error) {
		// A socketpair would be a tighter private channel here, but the currently used
		// grpc-swift transport stack (grpc-swift-nio-transport 2.7.x) doesn't support
		// serving helper RPCs directly over an inherited socketpair endpoint, so we use
		// a one-shot Unix domain socket in a private temp directory instead.
		socketDir, err := os.MkdirTemp("", "thand-localbroker-helper-*")
		if err != nil {
			return nil, err
		}
		socketPath := filepath.Join(socketDir, "helper.sock")
		cmd := exec.CommandContext(ctx, executable, append(args, "--grpc-socket-path", socketPath)...)

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Start(); err != nil {
			_ = os.RemoveAll(socketDir)
			return nil, err
		}

		var waitOnce sync.Once
		var waitErr error
		var closeOnce sync.Once
		cleanup := func() {
			closeOnce.Do(func() {
				_ = os.RemoveAll(socketDir)
			})
		}

		return &helperSession{
			dial: func(ctx context.Context, _ string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, "unix", socketPath)
			},
			wait: func() error {
				waitOnce.Do(func() {
					waitErr = cmd.Wait()
					cleanup()
				})
				return waitErr
			},
			close: func() error {
				cleanup()
				return nil
			},
			diagnostics: func() string {
				combined := bytes.TrimSpace(append(stdout.Bytes(), append([]byte("\n"), stderr.Bytes()...)...))
				return string(bytes.TrimSpace(combined))
			},
		}, nil
	}
}
