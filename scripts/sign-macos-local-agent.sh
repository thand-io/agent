#!/bin/sh
set -eu

. "$(dirname "$0")/macos-privilege-services-common.sh"

require_darwin

BINARY_PATH=${1:-"$REPO_ROOT/bin/thand"}

if [ -z "${APPLE_TEAM_ID:-}" ]; then
    printf 'Skipping Apple Development signing for %s because APPLE_TEAM_ID is not set\n' "$BINARY_PATH"
    exit 0
fi

development_identity=$(find_apple_development_identity)
sign_local_agent_binary "$BINARY_PATH" "$development_identity"

printf 'Signed local agent binary at %s\n' "$BINARY_PATH"
