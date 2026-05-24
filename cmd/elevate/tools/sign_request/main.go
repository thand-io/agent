package main

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/thand-io/agent/cmd/elevate/domain"
	"github.com/thand-io/agent/cmd/elevate/verify"
)

func main() {
	var (
		privateKeyPath  string
		keyID           string
		nonce           string
		socketPath      string
		action          string
		workflowID      string
		requestID       string
		username        string
		durationSeconds int64
		timeout         time.Duration
	)

	flag.StringVar(&privateKeyPath, "private-key", "", "path to Ed25519 private key PEM (PKCS8)")
	flag.StringVar(&keyID, "key-id", "", "key ID to put in signed_response")
	flag.StringVar(&nonce, "nonce", "", "challenge nonce from elevate helper")
	flag.StringVar(&socketPath, "socket", "", "optional unix socket path; when set, performs full challenge/response flow")
	flag.StringVar(&action, "action", "grant", "request action: grant|revoke")
	flag.StringVar(&workflowID, "workflow-id", "", "workflow_id")
	flag.StringVar(&requestID, "request-id", "", "request_id")
	flag.StringVar(&username, "username", "", "username")
	flag.Int64Var(&durationSeconds, "duration-seconds", 0, "duration_seconds (required for grant)")
	flag.DurationVar(&timeout, "timeout", 10*time.Second, "socket read/write timeout when -socket is set")
	flag.Parse()

	if privateKeyPath == "" || keyID == "" || workflowID == "" || requestID == "" || username == "" {
		exitf("missing required flags")
	}
	if action != string(domain.ActionGrant) && action != string(domain.ActionRevoke) {
		exitf("invalid action %q", action)
	}
	if action == string(domain.ActionGrant) && durationSeconds <= 0 {
		exitf("duration-seconds must be > 0 for grant")
	}
	if socketPath == "" && nonce == "" {
		exitf("nonce is required unless -socket is set")
	}

	privateKey, err := loadEd25519PrivateKey(privateKeyPath)
	if err != nil {
		exitf("load private key: %v", err)
	}

	request := domain.RequestFrame{
		Type:            domain.FrameTypeRequest,
		Action:          domain.Action(action),
		WorkflowID:      workflowID,
		RequestID:       requestID,
		Username:        username,
		DurationSeconds: durationSeconds,
	}
	if socketPath != "" {
		if err := runSocketFlow(socketPath, timeout, request, keyID, nonce, privateKey); err != nil {
			exitf("socket flow failed: %v", err)
		}
		return
	}

	frame, err := buildSignedResponseFrame(request, nonce, keyID, privateKey)
	if err != nil {
		exitf("build signed response: %v", err)
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

func runSocketFlow(socketPath string, timeout time.Duration, req domain.RequestFrame, keyID string, expectedNonce string, privateKey ed25519.PrivateKey) error {
	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return fmt.Errorf("dial unix socket %q: %w", socketPath, err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return fmt.Errorf("set deadline: %w", err)
	}

	if err := writeFrame(conn, req); err != nil {
		return fmt.Errorf("write request frame: %w", err)
	}

	reader := bufio.NewReader(conn)
	rawChallenge, err := readFrame(reader)
	if err != nil {
		return fmt.Errorf("read challenge frame: %w", err)
	}
	var challenge domain.ChallengeFrame
	if err := json.Unmarshal(rawChallenge, &challenge); err != nil {
		return fmt.Errorf("decode challenge frame: %w", err)
	}
	if challenge.Type != domain.FrameTypeChallenge {
		return fmt.Errorf("unexpected first frame type %q", challenge.Type)
	}
	if expectedNonce != "" && expectedNonce != challenge.Nonce {
		return fmt.Errorf("challenge nonce mismatch: expected %q got %q", expectedNonce, challenge.Nonce)
	}

	signedResp, err := buildSignedResponseFrame(req, challenge.Nonce, keyID, privateKey)
	if err != nil {
		return fmt.Errorf("build signed response frame: %w", err)
	}
	if err := writeFrame(conn, signedResp); err != nil {
		return fmt.Errorf("write signed response frame: %w", err)
	}

	rawResult, err := readFrame(reader)
	if err != nil {
		return fmt.Errorf("read result frame: %w", err)
	}

	// Print wire frames received from helper for easy manual inspection.
	fmt.Println(string(rawChallenge))
	fmt.Println(string(rawResult))
	return nil
}

func buildSignedResponseFrame(req domain.RequestFrame, nonce, keyID string, privateKey ed25519.PrivateKey) (domain.SignedResponseFrame, error) {
	payload := verify.SignedPayload{
		Nonce:           nonce,
		Action:          string(req.Action),
		WorkflowID:      req.WorkflowID,
		RequestID:       req.RequestID,
		Username:        req.Username,
		DurationSeconds: req.DurationSeconds,
	}
	canonical, err := verify.CanonicalPayload(payload)
	if err != nil {
		return domain.SignedResponseFrame{}, fmt.Errorf("canonical payload: %w", err)
	}
	sig := ed25519.Sign(privateKey, canonical)
	return domain.SignedResponseFrame{
		Type:          domain.FrameTypeSignedResponse,
		KeyID:         keyID,
		Signature:     base64.StdEncoding.EncodeToString(sig),
		SignedPayload: base64.StdEncoding.EncodeToString(canonical),
	}, nil
}

func writeFrame(conn net.Conn, frame any) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = conn.Write(data)
	return err
}

func readFrame(reader *bufio.Reader) ([]byte, error) {
	raw, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	return bytes.TrimSpace(raw), nil
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
