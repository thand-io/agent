package handler

import (
	"context"
	"time"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

// IPCServer abstracts how the helper accepts local IPC connections.
type IPCServer interface {
	Start(ctx context.Context) error
	Accept(ctx context.Context) (IPCConn, error)
	Close() error
}

// IPCConn abstracts framed request/response exchange over IPC.
type IPCConn interface {
	ReadFrame(ctx context.Context) ([]byte, error)
	WriteFrame(ctx context.Context, data []byte) error
	Close() error
}

// GrantEngine abstracts OS-specific local admin grant/revoke behavior.
type GrantEngine interface {
	Grant(ctx context.Context, req domain.GrantRequest) (domain.GrantResult, error)
	Revoke(ctx context.Context, req domain.RevokeRequest) error
}

// SignatureVerifier validates server signatures for signed payloads.
type SignatureVerifier interface {
	Verify(keyID string, payload []byte, signature []byte) error
}

// StateStore manages active grant persistence for recovery/cleanup.
type StateStore interface {
	Put(ctx context.Context, grant domain.GrantState) error
	Delete(ctx context.Context, requestID string) error
	List(ctx context.Context) ([]domain.GrantState, error)
}

// Clock abstracts time for deterministic tests and monotonic expiry logic.
type Clock interface {
	NowMonoNS() int64
	NowWallUTC() time.Time
}
