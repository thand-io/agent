#!/bin/sh
set -eu

. "$(dirname "$0")/macos-privilege-services-common.sh"

require_darwin
BUILD_CONFIGURATION=${BUILD_CONFIGURATION:-Debug}
STAGE_ROOT=${STAGE_ROOT:-"$PRIVILEGE_SERVICES_BUILD_ROOT/dev"}

generate_localbroker_grpc_sources
generate_privilege_services_project
run_unsigned_xcodebuild build -destination "$XCODEBUILD_BUILD_DESTINATION"
stage_unsigned_payload "$STAGE_ROOT"

if [ "${THAND_MACOS_SKIP_SIGNING:-0}" = "1" ]; then
    verify_staged_layout "$STAGE_ROOT"
    printf 'Packaged unsigned macOS privilege services payload in %s\n' "$STAGE_ROOT"
    exit 0
fi

development_identity=$(find_apple_development_identity)
sign_staged_payload "$STAGE_ROOT" "$development_identity" 0 1
verify_signed_payload \
    "$STAGE_ROOT/Applications/$STAGED_APP_NAME" \
    "$STAGE_ROOT/Library/Application Support/Thand/PrivilegeBroker/bin/$BROKER_CTL_BINARY_NAME"
printf 'Packaged Apple Development-signed macOS privilege services payload in %s\n' "$STAGE_ROOT"
