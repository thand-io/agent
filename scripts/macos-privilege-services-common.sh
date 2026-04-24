#!/bin/sh
set -eu

. "$(dirname "$0")/localbroker-codegen-common.sh"

PRIVILEGE_SERVICES_ROOT=${PRIVILEGE_SERVICES_ROOT:-"$REPO_ROOT/platform/macos/PrivilegeServices"}
PRIVILEGE_SERVICES_PROJECT_SPEC=${PRIVILEGE_SERVICES_PROJECT_SPEC:-"$PRIVILEGE_SERVICES_ROOT/project.yml"}
PRIVILEGE_SERVICES_PROJECT_FILE=${PRIVILEGE_SERVICES_PROJECT_FILE:-"$PRIVILEGE_SERVICES_ROOT/ThandPrivilegeServices.xcodeproj"}
PRIVILEGE_SERVICES_SCHEME=${PRIVILEGE_SERVICES_SCHEME:-ThandPrivilegeServices}
DERIVED_DATA_PATH=${DERIVED_DATA_PATH:-"$REPO_ROOT/.build/macos/DerivedData"}
PRIVILEGE_SERVICES_SOURCE_PACKAGES_DIR=${PRIVILEGE_SERVICES_SOURCE_PACKAGES_DIR:-"$REPO_ROOT/.build/macos/SourcePackages"}
BUILD_CONFIGURATION=${BUILD_CONFIGURATION:-Debug}
HOST_ARCH=$(uname -m)
XCODEBUILD_BUILD_DESTINATION=${XCODEBUILD_BUILD_DESTINATION:-"platform=macOS,arch=$HOST_ARCH"}
XCODEBUILD_TEST_DESTINATION=${XCODEBUILD_TEST_DESTINATION:-"platform=macOS,arch=$HOST_ARCH"}

generate_privilege_services_project() {
    require_darwin
    ensure_command xcodegen
    xcodegen generate \
        --use-cache \
        --spec "$PRIVILEGE_SERVICES_PROJECT_SPEC" \
        --project "$PRIVILEGE_SERVICES_ROOT" \
        >/dev/null
}

run_unsigned_xcodebuild() {
    action=$1
    shift

    xcodebuild \
        -project "$PRIVILEGE_SERVICES_PROJECT_FILE" \
        -scheme "$PRIVILEGE_SERVICES_SCHEME" \
        -configuration "$BUILD_CONFIGURATION" \
        -derivedDataPath "$DERIVED_DATA_PATH" \
        CODE_SIGNING_ALLOWED=NO \
        CODE_SIGNING_REQUIRED=NO \
        "$action" \
        "$@"
}
