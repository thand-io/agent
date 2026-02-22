package handler

import (
	"context"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

// handleRevoke is split into its own file so revoke flow can evolve independently.
func (h *Handler) handleRevoke(ctx context.Context, conn IPCConn, req domain.RequestFrame) error {
	if err := h.authenticateRequest(ctx, conn, req); err != nil {
		_ = h.writeResult(ctx, conn, req, resultStatusError, err.Code)
		h.logFailure(req.Action, req.RequestID, err)
		return nil
	}

	if err := h.grantEngine.Revoke(ctx, domain.RevokeRequest{
		RequestID:  req.RequestID,
		WorkflowID: req.WorkflowID,
		Username:   req.Username,
	}); err != nil {
		resErr := wrapInternal("revoke", err)
		_ = h.writeResult(ctx, conn, req, resultStatusError, resErr.Code)
		h.logFailure(req.Action, req.RequestID, resErr)
		return nil
	}

	if err := h.stateStore.Delete(ctx, req.RequestID); err != nil {
		resErr := wrapInternal("delete grant state", err)
		writeErr := h.writeResult(ctx, conn, req, resultStatusError, resErr.Code)
		if writeErr != nil {
			return writeErr
		}
		h.logFailure(req.Action, req.RequestID, resErr)
		return nil
	}

	if err := h.writeResult(ctx, conn, req, resultStatusOK, ""); err != nil {
		return err
	}
	h.logSuccess(req.Action, req.RequestID)
	return nil
}
