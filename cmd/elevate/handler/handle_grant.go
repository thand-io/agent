package handler

import (
	"context"
	"fmt"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

// handleGrant is split into its own file so grant flow can evolve independently.
func (h *Handler) handleGrant(ctx context.Context, conn IPCConn, req domain.RequestFrame) error {
	if err := h.authenticateRequest(ctx, conn, req); err != nil {
		return h.writeRequestError(ctx, conn, req, err)
	}

	grants, err := h.stateStore.List(ctx)
	if err != nil {
		return h.writeRequestError(ctx, conn, req, wrapInternal("list grant state", err))
	}
	stateErr := validateGrantRequestState(grants, req)
	switch {
	case stateErr.idempotent:
		return h.writeGrantSuccess(ctx, conn, req)
	case stateErr == (grantStateError{}):
	default:
		return h.writeRequestError(ctx, conn, req, stateErr.responseError())
	}

	result, err := h.grantEngine.Grant(ctx, domain.GrantRequest{
		RequestID:       req.RequestID,
		WorkflowID:      req.WorkflowID,
		Username:        req.Username,
		DurationSeconds: req.DurationSeconds,
	})
	if err != nil {
		return h.writeRequestError(ctx, conn, req, wrapInternal("grant", err))
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
		return h.writeRequestError(ctx, conn, req, wrapInternal("persist grant state", err))
	}

	return h.writeGrantSuccess(ctx, conn, req)
}

type grantStateError struct {
	code       ErrorCode
	err        error
	idempotent bool
}

func (e grantStateError) responseError() *responseError {
	switch e.code {
	case ErrorCodeRequestConflict:
		return requestConflictErr(e.err)
	case ErrorCodeActiveGrantExists:
		return activeGrantExistsErr(e.err)
	default:
		return invalidRequestErr(e.err)
	}
}

func validateGrantRequestState(grants []domain.GrantState, req domain.RequestFrame) grantStateError {
	for _, grant := range grants {
		if grant.RequestID == req.RequestID {
			if sameGrantRequest(grant, req) {
				return grantStateError{idempotent: true}
			}
			return grantStateError{
				code: ErrorCodeRequestConflict,
				err:  fmt.Errorf("request %q conflicts with existing state", req.RequestID),
			}
		}
		if grant.Username == req.Username && !isCompletedGrantState(grant) {
			return grantStateError{
				code: ErrorCodeActiveGrantExists,
				err:  fmt.Errorf("user %q already has an active grant", req.Username),
			}
		}
	}
	return grantStateError{}
}

func sameGrantRequest(grant domain.GrantState, req domain.RequestFrame) bool {
	return grant.WorkflowID == req.WorkflowID &&
		grant.Username == req.Username &&
		grant.DurationSeconds == req.DurationSeconds
}

func (h *Handler) writeGrantSuccess(ctx context.Context, conn IPCConn, req domain.RequestFrame) error {
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
