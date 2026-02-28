package handler

import (
	"context"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

// handleRevoke is split into its own file so revoke flow can evolve independently.
func (h *Handler) handleRevoke(ctx context.Context, conn IPCConn, req domain.RequestFrame) error {
	if err := h.authenticateRequest(ctx, conn, req); err != nil {
		_ = h.writeRequestError(ctx, conn, req, err)
		return nil
	}
	if err := validateRequestUsername(req.Username); err != nil {
		return h.writeRequestError(ctx, conn, req, err)
	}

	grants, err := h.stateStore.List(ctx)
	if err != nil {
		return h.writeRequestError(ctx, conn, req, wrapInternal("list grant state", err))
	}

	stored, found := findGrantState(grants, req.RequestID)
	username := req.Username
	if found {
		username = stored.Username
		if isCompletedGrantState(stored) {
			return h.writeRevokeSuccess(ctx, conn, req, username)
		}
		if stored.WasAlreadyPrivileged {
			if err := h.completeGrantState(ctx, req, conn, stored); err != nil {
				return err
			}
			return h.writeRevokeSuccess(ctx, conn, req, username)
		}
	}

	if err := h.grantEngine.Revoke(ctx, domain.RevokeRequest{
		RequestID:  req.RequestID,
		WorkflowID: req.WorkflowID,
		Username:   username,
	}); err != nil {
		return h.writeRequestError(ctx, conn, req, wrapInternal("revoke", err))
	}

	if found {
		if err := h.completeGrantState(ctx, req, conn, stored); err != nil {
			return err
		}
	}

	return h.writeRevokeSuccess(ctx, conn, req, username)
}

func (h *Handler) completeGrantState(ctx context.Context, req domain.RequestFrame, conn IPCConn, grant domain.GrantState) error {
	if isCompletedGrantState(grant) {
		return nil
	}

	grant.CompletedAtWallUTC = h.clock.NowWallUTC()
	if err := h.stateStore.Put(ctx, grant); err != nil {
		return h.writeRequestError(ctx, conn, req, wrapInternal("persist completed grant state", err))
	}

	return nil
}

func isCompletedGrantState(grant domain.GrantState) bool {
	return !grant.CompletedAtWallUTC.IsZero()
}

func (h *Handler) writeRequestError(ctx context.Context, conn IPCConn, req domain.RequestFrame, err *responseError) error {
	if writeErr := h.writeResult(ctx, conn, req, resultStatusError, err.Code); writeErr != nil {
		return writeErr
	}
	h.logFailure(req.Action, req.RequestID, err)
	return nil
}

func (h *Handler) writeRevokeSuccess(ctx context.Context, conn IPCConn, req domain.RequestFrame, username string) error {
	if err := h.writeResult(ctx, conn, req, resultStatusOK, ""); err != nil {
		return err
	}
	h.logger.Info("access revoked for "+username,
		"component", "elevate_handler",
		"action", string(req.Action),
		"request_id", req.RequestID,
		"workflow_id", req.WorkflowID,
		"username", username,
		"duration_seconds", req.DurationSeconds,
		"status", "ok",
	)
	return nil
}

func findGrantState(grants []domain.GrantState, requestID string) (domain.GrantState, bool) {
	for _, grant := range grants {
		if grant.RequestID == requestID {
			return grant, true
		}
	}
	return domain.GrantState{}, false
}
