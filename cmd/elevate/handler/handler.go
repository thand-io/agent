package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/thand-io/agent/cmd/elevate/domain"
	"github.com/thand-io/agent/cmd/elevate/verify"
)

var ErrUnsupportedAction = errors.New("unsupported action")

const defaultRequestTimeout = 30 * time.Second

// Handler routes incoming request frames to action-specific handlers.
type Handler struct {
	grantEngine GrantEngine
	verifier    SignatureVerifier
	stateStore  StateStore
	clock       Clock
	logger      *slog.Logger

	requestTimeout time.Duration
	generateNonce  func() (string, error)
}

// Option configures handler behavior.
type Option func(*Handler)

// WithLogger sets the logger used by the handler.
func WithLogger(logger *slog.Logger) Option {
	return func(h *Handler) {
		if logger != nil {
			h.logger = logger
		}
	}
}

// WithRequestTimeout sets the per-connection request timeout.
func WithRequestTimeout(timeout time.Duration) Option {
	return func(h *Handler) {
		if timeout > 0 {
			h.requestTimeout = timeout
		}
	}
}

// New constructs a handler for elevate request processing.
func New(grantEngine GrantEngine, verifier SignatureVerifier, stateStore StateStore, clock Clock, opts ...Option) *Handler {
	h := &Handler{
		grantEngine:    grantEngine,
		verifier:       verifier,
		stateStore:     stateStore,
		clock:          clock,
		logger:         slog.Default(),
		requestTimeout: defaultRequestTimeout,
		generateNonce:  verify.GenerateNonce,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// HandleConnection is the per-connection router.
func (h *Handler) HandleConnection(ctx context.Context, conn IPCConn) error {
	reqCtx, cancel := context.WithTimeout(ctx, h.requestTimeout)
	defer cancel()

	frameBytes, err := conn.ReadFrame(reqCtx)
	if err != nil {
		return fmt.Errorf("read frame: %w", err)
	}

	var req domain.RequestFrame
	if err := json.Unmarshal(frameBytes, &req); err != nil {
		return fmt.Errorf("decode request frame: %w", err)
	}
	if req.Type != domain.FrameTypeRequest {
		return fmt.Errorf("invalid request frame type: %s", req.Type)
	}

	switch req.Action {
	case domain.ActionGrant:
		return h.handleGrant(reqCtx, conn, req)
	case domain.ActionRevoke:
		return h.handleRevoke(reqCtx, conn, req)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedAction, req.Action)
	}
}

func (h *Handler) logSuccess(action domain.Action, requestID string) {
	h.logger.Info("elevate request handled",
		"component", "elevate_handler",
		"action", string(action),
		"request_id", requestID,
		"status", "ok",
	)
}

func (h *Handler) logFailure(action domain.Action, requestID string, err *responseError) {
	level := slog.LevelWarn
	if err.Code == ErrorCodeInternal {
		level = slog.LevelError
	}

	h.logger.Log(context.Background(), level, "elevate request failed",
		"component", "elevate_handler",
		"action", string(action),
		"request_id", requestID,
		"status", "error",
		"code", string(err.Code),
	)

	if err.Cause != nil && h.logger.Enabled(context.Background(), slog.LevelDebug) {
		h.logger.Debug("elevate request failure detail",
			"component", "elevate_handler",
			"action", string(action),
			"request_id", requestID,
			"code", string(err.Code),
			"cause", err.Cause.Error(),
		)
	}
}
