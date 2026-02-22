package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/thand-io/agent/cmd/elevate/domain"
	"github.com/thand-io/agent/cmd/elevate/verify"
)

const (
	resultStatusOK    = "ok"
	resultStatusError = "error"
)

// authenticateRequest performs the challenge-response signature flow:
// emit challenge nonce, read signed response, then verify payload+signature.
func (h *Handler) authenticateRequest(ctx context.Context, conn IPCConn, req domain.RequestFrame) *responseError {
	nonce, err := h.generateNonce()
	if err != nil {
		return wrapInternal("generate challenge nonce", err)
	}

	if err := h.writeFrame(ctx, conn, domain.ChallengeFrame{
		Type:  domain.FrameTypeChallenge,
		Nonce: nonce,
	}); err != nil {
		return wrapInternal("write challenge", err)
	}

	signedBytes, err := conn.ReadFrame(ctx)
	if err != nil {
		return invalidRequestErr(fmt.Errorf("read signed response: %w", err))
	}

	var signed domain.SignedResponseFrame
	if err := json.Unmarshal(signedBytes, &signed); err != nil {
		return invalidRequestErr(fmt.Errorf("decode signed response: %w", err))
	}
	if signed.Type != domain.FrameTypeSignedResponse {
		return invalidRequestErr(fmt.Errorf("invalid signed response type: %s", signed.Type))
	}

	// Decode and validate signed payload fields against the original request+nonce.
	payload, err := verify.DecodeSignedPayload(signed.SignedPayload)
	if err != nil {
		return invalidRequestErr(fmt.Errorf("decode signed payload: %w", err))
	}
	if err := verify.MatchSignedPayload(req, payload, nonce); err != nil {
		return unauthorizedErr(fmt.Errorf("match signed payload: %w", err))
	}

	canonical, err := verify.CanonicalPayload(payload)
	if err != nil {
		return invalidRequestErr(fmt.Errorf("canonical payload: %w", err))
	}
	sig, err := verify.DecodeSignature(signed.Signature)
	if err != nil {
		return invalidRequestErr(fmt.Errorf("decode signature: %w", err))
	}
	// Verify detached signature over canonical payload using pinned key ID.
	if err := h.verifier.Verify(signed.KeyID, canonical, sig); err != nil {
		return unauthorizedErr(fmt.Errorf("verify signature: %w", err))
	}

	return nil
}

// writeResult sends the terminal result frame for the current request.
func (h *Handler) writeResult(ctx context.Context, conn IPCConn, req domain.RequestFrame, status string, code ErrorCode) error {
	result := domain.ResultFrame{
		Type:      domain.FrameTypeResult,
		Status:    status,
		RequestID: req.RequestID,
	}
	if code != "" {
		result.Error = string(code)
	}

	return h.writeFrame(ctx, conn, result)
}

// writeFrame marshals and writes one protocol frame to the connection.
func (h *Handler) writeFrame(ctx context.Context, conn IPCConn, frame any) error {
	b, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("encode frame: %w", err)
	}
	if err := conn.WriteFrame(ctx, b); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}
	return nil
}
