#!/bin/sh
set -eu

. "$(dirname "$0")/macos-privilege-services-common.sh"

require_darwin
BUILD_CONFIGURATION=Release
ARCHIVE_PATH=${ARCHIVE_PATH:-"$PRIVILEGE_SERVICES_BUILD_ROOT/release/ThandPrivilegeServices.xcarchive"}
STAGE_ROOT=${STAGE_ROOT:-"$PRIVILEGE_SERVICES_BUILD_ROOT/release/stage"}
PKG_ROOT=${PKG_ROOT:-"$PRIVILEGE_SERVICES_BUILD_ROOT/release/pkgroot"}
UNSIGNED_PKG=${UNSIGNED_PKG:-"$PRIVILEGE_SERVICES_BUILD_ROOT/release/ThandPrivilegeServices-unsigned.pkg"}
SIGNED_PKG=${SIGNED_PKG:-"$PRIVILEGE_SERVICES_BUILD_ROOT/release/ThandPrivilegeServices.pkg"}
PACKAGE_VERSION=${PACKAGE_VERSION:-0.0.0}
NOTARYTOOL_PROFILE=${NOTARYTOOL_PROFILE:-}

generate_localbroker_grpc_sources
generate_privilege_services_project
run_unsigned_xcodebuild archive -archivePath "$ARCHIVE_PATH"
stage_unsigned_payload "$STAGE_ROOT"

application_identity=$(find_developer_id_application_identity)
sign_staged_payload "$STAGE_ROOT" "$application_identity" 1 1
verify_staged_layout "$STAGE_ROOT"

rm -rf "$PKG_ROOT"
mkdir -p "$PKG_ROOT/Applications" "$PKG_ROOT/Library/Application Support/Thand/PrivilegeBroker/bin"
ditto "$STAGE_ROOT/Applications/$STAGED_APP_NAME" "$PKG_ROOT/Applications/$STAGED_APP_NAME"
install -m 0755 \
    "$STAGE_ROOT/Library/Application Support/Thand/PrivilegeBroker/bin/$BROKER_CTL_BINARY_NAME" \
    "$PKG_ROOT/Library/Application Support/Thand/PrivilegeBroker/bin/$BROKER_CTL_BINARY_NAME"
normalize_stage_root_modes "$PKG_ROOT"
verify_signed_payload \
    "$PKG_ROOT/Applications/$STAGED_APP_NAME" \
    "$PKG_ROOT/Library/Application Support/Thand/PrivilegeBroker/bin/$BROKER_CTL_BINARY_NAME"

pkgbuild \
    --root "$PKG_ROOT" \
    --ownership recommended \
    --identifier "io.thand.agent.privilege-services" \
    --version "$PACKAGE_VERSION" \
    "$UNSIGNED_PKG"

installer_identity=$(find_developer_id_installer_identity)
productsign --sign "$installer_identity" "$UNSIGNED_PKG" "$SIGNED_PKG"

if [ -n "$NOTARYTOOL_PROFILE" ]; then
    xcrun notarytool submit "$SIGNED_PKG" --keychain-profile "$NOTARYTOOL_PROFILE" --wait
    xcrun stapler staple "$SIGNED_PKG"
fi

printf 'Packaged signed macOS privilege services installer at %s\n' "$SIGNED_PKG"
