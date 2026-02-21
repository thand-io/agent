package handler

import (
	"context"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

// handleRevoke is split into its own file so revoke flow can evolve independently.
func (h *Handler) handleRevoke(ctx context.Context, conn IPCConn, req domain.RequestFrame) error {
	_ = ctx
	_ = conn
	_ = req
	// Chunk 1 placeholder: revoke lifecycle is implemented in later chunks.
	return nil
}
