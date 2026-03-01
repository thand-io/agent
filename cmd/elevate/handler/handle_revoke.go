package handler

import (
	"context"
	"fmt"

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
		if stored.WorkflowID != req.WorkflowID || stored.Username != req.Username {
			return h.writeRequestError(ctx, conn, req, requestConflictErr(fmt.Errorf("request %q conflicts with existing state", req.RequestID)))
		}
		username = stored.Username
		if domain.IsCompletedGrantState(stored) {
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
		return h.writeRequestError(ctx, conn, req, fmt.Errorf("revoke: %w", err))
	}

	if found {
		if err := h.completeGrantState(ctx, req, conn, stored); err != nil {
			return err
		}
	}

	return h.writeRevokeSuccess(ctx, conn, req, username)
}

func (h *Handler) completeGrantState(ctx context.Context, req domain.RequestFrame, conn IPCConn, grant domain.GrantState) error {
	if domain.IsCompletedGrantState(grant) {
		return nil
	}

	grant.CompletedAtWallUTC = h.clock.NowWallUTC()
	if err := h.stateStore.Put(ctx, grant); err != nil {
		if deleteErr := h.stateStore.Delete(ctx, grant.RequestID); deleteErr != nil {
			return h.writeRequestError(ctx, conn, req, wrapInternal("persist completed grant state", fmt.Errorf("%w; delete fallback failed: %v", err, deleteErr)))
		}
		return nil
	}

	return nil
}

func (h *Handler) writeRequestError(ctx context.Context, conn IPCConn, req domain.RequestFrame, err error) error {
	resErr := classifyResponseError(err)
	if writeErr := h.writeResult(ctx, conn, req, resultStatusError, resErr.Code); writeErr != nil {
		return writeErr
	}
	h.logFailure(req.Action, req.RequestID, resErr)
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
