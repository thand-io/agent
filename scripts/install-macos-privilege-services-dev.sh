#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "install-macos-privilege-services-dev.sh must run as root" >&2
    exit 1
fi

. "$(dirname "$0")/macos-privilege-services-common.sh"

STAGE_ROOT=${STAGE_ROOT:-"$PRIVILEGE_SERVICES_BUILD_ROOT/dev"}
PACKAGE_SCRIPT="$REPO_ROOT/scripts/package-macos-privilege-services-dev.sh"

if [ -n "${SUDO_USER:-}" ] && [ "$SUDO_USER" != "root" ]; then
    sudo -u "$SUDO_USER" env \
        PATH="$PATH" \
        APPLE_TEAM_ID="${APPLE_TEAM_ID:-}" \
        THAND_MACOS_SKIP_SIGNING="${THAND_MACOS_SKIP_SIGNING:-0}" \
        BUILD_CONFIGURATION="${BUILD_CONFIGURATION:-Debug}" \
        STAGE_ROOT="$STAGE_ROOT" \
        DERIVED_DATA_PATH="${DERIVED_DATA_PATH:-$DERIVED_DATA_PATH}" \
        PRIVILEGE_SERVICES_BUILD_ROOT="${PRIVILEGE_SERVICES_BUILD_ROOT:-$PRIVILEGE_SERVICES_BUILD_ROOT}" \
        XCODEBUILD_BUILD_DESTINATION="${XCODEBUILD_BUILD_DESTINATION:-$XCODEBUILD_BUILD_DESTINATION}" \
        "$PACKAGE_SCRIPT"
else
    "$PACKAGE_SCRIPT"
fi

if [ -d "$INSTALL_APP_PATH" ] && [ -n "${SUDO_USER:-}" ] && [ "$SUDO_USER" != "root" ]; then
    sudo -u "$SUDO_USER" "$INSTALL_APP_PATH/Contents/MacOS/ThandPrivilegeServices" unregister >/dev/null 2>&1 || true
fi

copy_stage_into_system "$STAGE_ROOT"
normalize_installed_payload

if [ "${THAND_MACOS_SKIP_SIGNING:-0}" = "1" ]; then
    printf 'Installed unsigned macOS privilege services payload to %s\n' "$INSTALL_APP_PATH"
    exit 0
fi

verify_signed_payload "$INSTALL_APP_PATH" "$BROKER_CTL_INSTALL_DIR/$BROKER_CTL_BINARY_NAME"

if [ -n "${SUDO_USER:-}" ] && [ "$SUDO_USER" != "root" ]; then
    run_privilege_services_as_user register || true
fi

printf 'Installed macOS privilege services app to %s\n' "$INSTALL_APP_PATH"
