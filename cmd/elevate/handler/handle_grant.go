package handler

import (
	"context"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

// handleGrant is split into its own file so grant flow can evolve independently.
func (h *Handler) handleGrant(ctx context.Context, conn IPCConn, req domain.RequestFrame) error {
	if err := h.authenticateRequest(ctx, conn, req); err != nil {
		_ = h.writeResult(ctx, conn, req, resultStatusError, err.Code)
		h.logFailure(req.Action, req.RequestID, err)
		return nil
	}

	result, err := h.grantEngine.Grant(ctx, domain.GrantRequest{
		RequestID:       req.RequestID,
		WorkflowID:      req.WorkflowID,
		Username:        req.Username,
		DurationSeconds: req.DurationSeconds,
	})
	if err != nil {
		resErr := wrapInternal("grant", err)
		_ = h.writeResult(ctx, conn, req, resultStatusError, resErr.Code)
		h.logFailure(req.Action, req.RequestID, resErr)
		return nil
	}

	state := domain.GrantState{
		RequestID:            req.RequestID,
		WorkflowID:           req.WorkflowID,
		Username:             req.Username,
		GrantedAtWallUTC:     h.clock.NowWallUTC(),
		GrantedAtMonoNS:      h.clock.NowMonoNS(),
		DurationSeconds:      req.DurationSeconds,
		WasAlreadyPrivileged: result.WasAlreadyPrivileged,
	}
	if err := h.stateStore.Put(ctx, state); err != nil {
		resErr := wrapInternal("persist grant state", err)
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
	h.logger.Info("access elevated for "+req.Username,
		"component", "elevate_handler",
		"action", string(req.Action),
		"request_id", req.RequestID,
		"workflow_id", req.WorkflowID,
		"username", req.Username,
		"duration_seconds", req.DurationSeconds,
		"status", "ok",
	)
	return nil
}
