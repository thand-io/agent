package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"os"

	"github.com/thand-io/agent/cmd/elevate/domain"
	"github.com/thand-io/agent/cmd/elevate/verify"
)

func main() {
	var (
		privateKeyPath   string
		keyID            string
		nonce            string
		action           string
		workflowID       string
		requestID        string
		username         string
		durationSeconds  int64
	)

	flag.StringVar(&privateKeyPath, "private-key", "", "path to Ed25519 private key PEM (PKCS8)")
	flag.StringVar(&keyID, "key-id", "", "key ID to put in signed_response")
	flag.StringVar(&nonce, "nonce", "", "challenge nonce from elevate helper")
	flag.StringVar(&action, "action", "grant", "request action: grant|revoke")
	flag.StringVar(&workflowID, "workflow-id", "", "workflow_id")
	flag.StringVar(&requestID, "request-id", "", "request_id")
	flag.StringVar(&username, "username", "", "username")
	flag.Int64Var(&durationSeconds, "duration-seconds", 0, "duration_seconds (required for grant)")
	flag.Parse()

	if privateKeyPath == "" || keyID == "" || nonce == "" || workflowID == "" || requestID == "" || username == "" {
		exitf("missing required flags")
	}
	if action != string(domain.ActionGrant) && action != string(domain.ActionRevoke) {
		exitf("invalid action %q", action)
	}
	if action == string(domain.ActionGrant) && durationSeconds <= 0 {
		exitf("duration-seconds must be > 0 for grant")
	}

	privateKey, err := loadEd25519PrivateKey(privateKeyPath)
	if err != nil {
		exitf("load private key: %v", err)
	}

	payload := verify.SignedPayload{
		Nonce:           nonce,
		Action:          action,
		WorkflowID:      workflowID,
		RequestID:       requestID,
		Username:        username,
		DurationSeconds: durationSeconds,
	}
	canonical, err := verify.CanonicalPayload(payload)
	if err != nil {
		exitf("canonical payload: %v", err)
	}

	sig := ed25519.Sign(privateKey, canonical)
	request := domain.RequestFrame{
		Type:            domain.FrameTypeRequest,
		Action:          domain.Action(action),
		WorkflowID:      workflowID,
		RequestID:       requestID,
		Username:        username,
		DurationSeconds: durationSeconds,
	}
	frame := domain.SignedResponseFrame{
		Type:          domain.FrameTypeSignedResponse,
		KeyID:         keyID,
		Signature:     base64.StdEncoding.EncodeToString(sig),
		SignedPayload: base64.StdEncoding.EncodeToString(canonical),
	}

	reqJSON, err := json.Marshal(request)
	if err != nil {
		exitf("marshal request: %v", err)
	}
	respJSON, err := json.Marshal(frame)
	if err != nil {
		exitf("marshal signed response: %v", err)
	}

	// Print wire-ready frames in protocol order: request then signed_response.
	fmt.Println(string(reqJSON))
	fmt.Println(string(respJSON))
}

func loadEd25519PrivateKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("invalid PEM data")
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not Ed25519")
	}
	return key, nil
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
