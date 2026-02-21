package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

// Handler routes incoming request frames to action-specific handlers.
type Handler struct {
	grantEngine GrantEngine
	verifier    SignatureVerifier
	stateStore  StateStore
	clock       Clock
}

func New(grantEngine GrantEngine, verifier SignatureVerifier, stateStore StateStore, clock Clock) *Handler {
	return &Handler{
		grantEngine: grantEngine,
		verifier:    verifier,
		stateStore:  stateStore,
		clock:       clock,
	}
}

// HandleConnection is the per-connection router.
func (h *Handler) HandleConnection(ctx context.Context, conn IPCConn) error {
	defer conn.Close()

	frameBytes, err := conn.ReadFrame(ctx)
	if err != nil {
		return fmt.Errorf("read frame: %w", err)
	}

	var req domain.RequestFrame
	if err := json.Unmarshal(frameBytes, &req); err != nil {
		return fmt.Errorf("decode request frame: %w", err)
	}

	switch req.Action {
	case "grant":
		return h.handleGrant(ctx, conn, req)
	case "revoke":
		return h.handleRevoke(ctx, conn, req)
	default:
		return fmt.Errorf("unsupported action: %s", req.Action)
	}
}
