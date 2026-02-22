//go:build linux || darwin

package ipc

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/thand-io/agent/cmd/elevate/handler"
)

const (
	defaultReadPollInterval  = 250 * time.Millisecond
	defaultWritePollInterval = 250 * time.Millisecond
)

var (
	ErrSocketPathRequired = errors.New("unix socket path is required")
	ErrServerNotStarted   = errors.New("unix server not started")
)

type UnixServer struct {
	path       string
	dirPerm    os.FileMode
	socketPerm os.FileMode

	listener unixListener

	mkdirAll   func(path string, perm os.FileMode) error
	lstat      func(name string) (os.FileInfo, error)
	remove     func(name string) error
	listenUnix func(network string, laddr *net.UnixAddr) (unixListener, error)
	chmod      func(name string, mode os.FileMode) error
	now        func() time.Time
	newConn    func(*net.UnixConn) handler.IPCConn
}

type acceptResult struct {
	conn *net.UnixConn
	err  error
}

type unixListener interface {
	AcceptUnix() (*net.UnixConn, error)
	SetDeadline(t time.Time) error
	Close() error
}

type Option func(*UnixServer)

// WithMkdirAll overrides directory creation for testability.
func WithMkdirAll(fn func(path string, perm os.FileMode) error) Option {
	return func(s *UnixServer) { s.mkdirAll = fn }
}

// WithLstat overrides filesystem stat for testability.
func WithLstat(fn func(name string) (os.FileInfo, error)) Option {
	return func(s *UnixServer) { s.lstat = fn }
}

// WithRemove overrides filesystem remove for testability.
func WithRemove(fn func(name string) error) Option {
	return func(s *UnixServer) { s.remove = fn }
}

// WithListenUnix overrides Unix listener creation for testability.
func WithListenUnix(fn func(network string, laddr *net.UnixAddr) (unixListener, error)) Option {
	return func(s *UnixServer) { s.listenUnix = fn }
}

// WithChmod overrides chmod for testability.
func WithChmod(fn func(name string, mode os.FileMode) error) Option {
	return func(s *UnixServer) { s.chmod = fn }
}

// WithNow overrides time source for testability.
func WithNow(fn func() time.Time) Option {
	return func(s *UnixServer) { s.now = fn }
}

// WithNewConn overrides IPC connection wrapper for testability.
func WithNewConn(fn func(*net.UnixConn) handler.IPCConn) Option {
	return func(s *UnixServer) { s.newConn = fn }
}

func NewUnixServer(path string, opts ...Option) (handler.IPCServer, error) {
	if path == "" {
		return nil, ErrSocketPathRequired
	}

	srv := &UnixServer{
		path:       path,
		dirPerm:    0o750,
		socketPerm: 0o660,
		mkdirAll:   os.MkdirAll,
		lstat:      os.Lstat,
		remove:     os.Remove,
		listenUnix: func(network string, laddr *net.UnixAddr) (unixListener, error) { return net.ListenUnix(network, laddr) },
		chmod:      os.Chmod,
		now:        time.Now,
		newConn:    newUnixConn,
	}

	for _, opt := range opts {
		opt(srv)
	}

	return srv, nil
}

// Start initializes the Unix socket listener at the configured path.
func (s *UnixServer) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if s.listener != nil {
		return nil
	}

	dir := filepath.Dir(s.path)
	if err := s.mkdirAll(dir, s.dirPerm); err != nil {
		return fmt.Errorf("create unix socket directory: %w", err)
	}

	if err := s.removeStaleSocket(s.path); err != nil {
		return err
	}

	var err error
	s.listener, err = s.listenUnix("unix", &net.UnixAddr{Name: s.path, Net: "unix"})
	if err != nil {
		return fmt.Errorf("listen on unix socket: %w", err)
	}

	if err := s.chmod(s.path, s.socketPerm); err != nil {
		_ = s.listener.Close()
		s.listener = nil
		return fmt.Errorf("set unix socket permissions: %w", err)
	}

	return nil
}

// Accept waits for a single client connection and returns it as an IPCConn.
func (s *UnixServer) Accept(ctx context.Context) (handler.IPCConn, error) {
	if s.listener == nil {
		return nil, ErrServerNotStarted
	}

	resultCh := acceptAsync(s.listener)

	select {
	case <-ctx.Done():
		return nil, s.handleAcceptCancel(ctx, s.listener, resultCh)
	case res := <-resultCh:
		return s.handleAcceptResult(ctx, res)
	}
}

// Close shuts down the listener and removes the socket file path.
func (s *UnixServer) Close() error {
	if s.listener != nil {
		if err := s.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}
		s.listener = nil
	}

	if err := s.remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// removeStaleSocket removes an existing socket file so a new listener can bind.
func (s *UnixServer) removeStaleSocket(path string) error {
	info, err := s.lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat unix socket path: %w", err)
	}

	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to remove non-socket path: %s", path)
	}

	if err := s.remove(path); err != nil {
		return fmt.Errorf("remove stale unix socket: %w", err)
	}

	return nil
}

type unixConn struct {
	conn   *net.UnixConn
	reader *bufio.Reader
}

func newUnixConn(conn *net.UnixConn) handler.IPCConn {
	return &unixConn{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

// ReadFrame reads one newline-delimited frame from the connection.
func (c *unixConn) ReadFrame(ctx context.Context) ([]byte, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Frame reads are newline-delimited; deadline polling keeps reads cancelable.
		if err := c.conn.SetReadDeadline(time.Now().Add(defaultReadPollInterval)); err != nil {
			return nil, fmt.Errorf("set read deadline: %w", err)
		}

		line, err := c.reader.ReadBytes('\n')
		if err == nil {
			return normalizeReadFrame(line), nil
		}

		if isTimeoutErr(err) {
			continue
		}

		return nil, fmt.Errorf("read frame: %w", err)
	}
}

// WriteFrame writes one newline-delimited frame to the connection.
func (c *unixConn) WriteFrame(ctx context.Context, data []byte) error {
	frame := normalizeWriteFrame(data)
	written := 0
	for written < len(frame) {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Use bounded writes so we can honor context cancellation/deadlines.
		deadline := time.Now().Add(defaultWritePollInterval)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		if err := c.conn.SetWriteDeadline(deadline); err != nil {
			return fmt.Errorf("set write deadline: %w", err)
		}

		n, err := c.conn.Write(frame[written:])
		written += n
		if err == nil {
			continue
		}
		if isTimeoutErr(err) {
			continue
		}
		return fmt.Errorf("write frame: %w", err)
	}

	return nil
}

// Close closes the underlying Unix domain socket connection.
func (c *unixConn) Close() error {
	return c.conn.Close()
}

func isTimeoutErr(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// acceptAsync performs a blocking AcceptUnix call in a goroutine.
func acceptAsync(l unixListener) <-chan acceptResult {
	resultCh := make(chan acceptResult, 1)
	go func() {
		conn, err := l.AcceptUnix()
		resultCh <- acceptResult{conn: conn, err: err}
	}()
	return resultCh
}

// handleAcceptCancel unblocks and drains a pending accept on context cancellation.
func (s *UnixServer) handleAcceptCancel(ctx context.Context, l unixListener, resultCh <-chan acceptResult) error {
	// AcceptUnix is blocking. Force it to return so the goroutine can exit.
	_ = l.SetDeadline(s.now())
	res := <-resultCh
	if res.conn != nil {
		_ = res.conn.Close()
	}
	return ctx.Err()
}

// handleAcceptResult converts a raw accept result into IPCConn or wrapped error.
func (s *UnixServer) handleAcceptResult(ctx context.Context, res acceptResult) (handler.IPCConn, error) {
	if res.err == nil {
		return s.newConn(res.conn), nil
	}

	if errors.Is(res.err, net.ErrClosed) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, res.err
	}

	return nil, fmt.Errorf("accept unix connection: %w", res.err)
}

// normalizeReadFrame trims CR/LF framing bytes from an incoming frame.
func normalizeReadFrame(line []byte) []byte {
	return bytes.TrimRight(line, "\r\n")
}

// normalizeWriteFrame ensures a frame ends with newline without mutating caller input.
func normalizeWriteFrame(data []byte) []byte {
	// Copy caller input to avoid mutating shared buffers when appending newline.
	frame := append([]byte(nil), data...)
	if len(frame) == 0 || frame[len(frame)-1] != '\n' {
		frame = append(frame, '\n')
	}
	return frame
}
