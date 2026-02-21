package handler

import (
	"context"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

// handleGrant is split into its own file so grant flow can evolve independently.
func (h *Handler) handleGrant(ctx context.Context, conn IPCConn, req domain.RequestFrame) error {
	_ = ctx
	_ = conn
	_ = req
	// Chunk 1 placeholder: grant lifecycle is implemented in later chunks.
	return nil
}
