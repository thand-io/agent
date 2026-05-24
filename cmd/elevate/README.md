# `cmd/elevate`

Local privileged helper for temporary local-admin elevation.

## Overview

`elevate` is a standalone daemon intended to run as root (Linux target today).  
It accepts local IPC requests, verifies a challenge-response signature using pinned public keys, performs OS-level grant/revoke operations, and persists grant state for cleanup/recovery.

### Core Components

- `main.go`
  - Loads config from env.
  - Builds dependencies.
  - Starts server + cleanup runner.
- `ipc/`
  - Unix socket transport (Linux/macOS/Windows).
  - Newline-delimited JSON framing.
  - Socket permissions + optional socket owner/group (`THAND_ELEVATE_SOCKET_USER`, `THAND_ELEVATE_SOCKET_GROUP`).
- `handler/`
  - Request router (`grant`/`revoke`).
  - Challenge/response signature verification flow.
  - Sanitized error responses and structured `slog` logging.
- `verify/`
  - Nonce handling + signed payload validation.
  - Ed25519 verification against compile-time pinned keys (`verify/keys/*.pem`).
- `grant/`
  - Linux grant engine (`/etc/sudoers.d/thand-<request_id>`).
  - `visudo -cf` validation.
  - Revoke removes request-specific sudoers entry.
- `state/`
  - Atomic state persistence (`tmp + fsync + rename + dir fsync`).
- `cleanup.go`
  - Startup and periodic sweep of expired grants.
  - Revokes expired entries and removes state records.
- `tools/sign_request/`
  - Local test utility to generate `request` + `signed_response` frames from a private key.

## Configuration

Environment variables:

- `THAND_ELEVATE_SOCKET_PATH` (default: `/var/run/thand/elevate.sock`)
- `THAND_ELEVATE_SOCKET_USER` (default: unset; set socket owner user by name)
- `THAND_ELEVATE_SOCKET_GROUP` (default: unset; set socket group by name)
- `THAND_ELEVATE_SUDOERS_DIR` (default: `/etc/sudoers.d`)
- `THAND_ELEVATE_SUDOERS_FILE` (default: `/etc/sudoers`)
- `THAND_ELEVATE_VISUDO_BIN` (default: `visudo`)
- `THAND_ELEVATE_STATE_PATH` (default: `/var/lib/thand/elevate/state.json`)
- `THAND_ELEVATE_CLEANUP_INTERVAL` (default: `1m`)
- `THAND_ELEVATE_REQUEST_TIMEOUT` (default: `30s`)
- `THAND_ELEVATE_LOG_LEVEL` (default: `info`; `debug|info|warn|error`)

## Testing

### Unit and race tests

```bash
cd cmd/elevate
go test ./...
go test -race ./...
```

### Build

```bash
cd cmd/elevate
go build -o /home/tom/dev/agent/elevate ./
```

### Run with real system paths (root)

```bash
sudo mkdir -p /var/run/thand /var/lib/thand/elevate
sudo chown root:root /var/run/thand /var/lib/thand/elevate
sudo chmod 755 /var/run/thand /var/lib/thand/elevate
```

```bash
sudo env \
  THAND_ELEVATE_SOCKET_PATH=/var/run/thand/elevate.sock \
  THAND_ELEVATE_SOCKET_USER="tom" \
  THAND_ELEVATE_SOCKET_GROUP="tom" \
  THAND_ELEVATE_SUDOERS_DIR=/etc/sudoers.d \
  THAND_ELEVATE_SUDOERS_FILE=/etc/sudoers \
  THAND_ELEVATE_VISUDO_BIN=visudo \
  THAND_ELEVATE_STATE_PATH=/var/lib/thand/elevate/state.json \
  THAND_ELEVATE_CLEANUP_INTERVAL=1m \
  THAND_ELEVATE_REQUEST_TIMEOUT=15m \
  THAND_ELEVATE_LOG_LEVEL=debug \
  /home/tom/dev/agent/elevate
```

## Manual Protocol Smoke Test

1. Open socket session (same connection for both frames):

```bash
socat - UNIX-CONNECT:/var/run/thand/elevate.sock
```

2. Send a request frame:

```json
{"type":"request","action":"grant","workflow_id":"wf-manual-1","request_id":"req-manual-1","username":"alice","duration_seconds":600}
```

3. Copy nonce from challenge response.

4. (Success path) Generate a local test keypair and pin the public key:

```bash
KEYDIR="$(mktemp -d /tmp/elevate-keys-XXXXXX)"
cd /home/tom/dev/agent/cmd/elevate
go run ./tools/generate_test_key \
  -out-dir "$KEYDIR" \
  -key-id local-test-key
```

Copy the generated public key into pinned keys, rebuild, restart:

```bash
KEY_ID="local-test-key"
cp "$KEYDIR/${KEY_ID}.pem" "/home/tom/dev/agent/cmd/elevate/verify/keys/${KEY_ID}.pem"
cd /home/tom/dev/agent/cmd/elevate
go build -o /home/tom/dev/agent/elevate ./
# restart your running elevate process
```

5. Generate frames with signer tool using matching private key + key id:

```bash
cd /home/tom/dev/agent/cmd/elevate
GOCACHE=/tmp/gocache go run ./tools/sign_request \
  -private-key "$KEYDIR/${KEY_ID}.private.pem" \
  -key-id "$KEY_ID" \
  -nonce "<CHALLENGE_NONCE>" \
  -action grant \
  -workflow-id wf-manual-1 \
  -request-id req-manual-1 \
  -username alice \
  -duration-seconds 600
```

This outputs two lines:
- `request` JSON
- `signed_response` JSON

Paste the `signed_response` line into the open socket session as the second frame.

Expected:
- success: `{"type":"result","status":"ok",...}`

Negative-path tip:
- Use an unknown `key_id` or mismatched private key to get `{"status":"error","error":"unauthorized"}`.

## Notes

- Pinned key IDs come from filenames in `cmd/elevate/verify/keys/*.pem`.
- No production keys are committed by default. You must add at least one `.pem` key file before starting the daemon without override options.
- Changing pinned keys requires rebuilding/restarting the helper.
- Helper has no network code path; signature authority is external to this binary.
