#!/bin/sh
set -eu

. "$(dirname "$0")/localbroker-codegen-common.sh"

PRIVILEGE_SERVICES_ROOT=${PRIVILEGE_SERVICES_ROOT:-"$REPO_ROOT/platform/macos/PrivilegeServices"}
PRIVILEGE_SERVICES_PROJECT_SPEC=${PRIVILEGE_SERVICES_PROJECT_SPEC:-"$PRIVILEGE_SERVICES_ROOT/project.yml"}
PRIVILEGE_SERVICES_PROJECT_FILE=${PRIVILEGE_SERVICES_PROJECT_FILE:-"$PRIVILEGE_SERVICES_ROOT/ThandPrivilegeServices.xcodeproj"}
PRIVILEGE_SERVICES_SCHEME=${PRIVILEGE_SERVICES_SCHEME:-ThandPrivilegeServices}
DERIVED_DATA_PATH=${DERIVED_DATA_PATH:-"$REPO_ROOT/.build/macos/DerivedData"}
PRIVILEGE_SERVICES_BUILD_ROOT=${PRIVILEGE_SERVICES_BUILD_ROOT:-"$REPO_ROOT/.build/macos/PrivilegeServices"}
PRIVILEGE_SERVICES_SOURCE_PACKAGES_DIR=${PRIVILEGE_SERVICES_SOURCE_PACKAGES_DIR:-"$REPO_ROOT/.build/macos/SourcePackages"}
BUILD_CONFIGURATION=${BUILD_CONFIGURATION:-Debug}
SERVICE_LABEL=${SERVICE_LABEL:-io.thand.agent.privilege-broker}
APP_BUNDLE_ID=${APP_BUNDLE_ID:-io.thand.agent.privilege-services}
BROKER_SIGNING_IDENTIFIER=${BROKER_SIGNING_IDENTIFIER:-io.thand.agent.privilege-broker}
BROKER_CTL_SIGNING_IDENTIFIER=${BROKER_CTL_SIGNING_IDENTIFIER:-io.thand.agent}
AGENT_SIGNING_IDENTIFIER=${AGENT_SIGNING_IDENTIFIER:-io.thand.agent}
STATE_DIR=${STATE_DIR:-/var/db/thand/local-privilege-broker}
INSTALL_APP_PATH=${INSTALL_APP_PATH:-/Applications/ThandPrivilegeServices.app}
BROKER_CTL_INSTALL_DIR=${BROKER_CTL_INSTALL_DIR:-/Library/Application Support/Thand/PrivilegeBroker/bin}
STAGED_APP_NAME=ThandPrivilegeServices.app
LOGIN_ITEM_NAME=ThandPrivilegeNotifier.app
DAEMON_BINARY_NAME=ThandPrivilegeBrokerDaemon
BROKER_CTL_BINARY_NAME=thand-macos-privilege-brokerctl
HOST_ARCH=$(uname -m)
XCODEBUILD_BUILD_DESTINATION=${XCODEBUILD_BUILD_DESTINATION:-"platform=macOS,arch=$HOST_ARCH"}
XCODEBUILD_TEST_DESTINATION=${XCODEBUILD_TEST_DESTINATION:-"platform=macOS,arch=$HOST_ARCH"}
APP_GROUPS_ENTITLEMENT_KEY=com.apple.security.application-groups
APP_GROUP_CLIENT_SUFFIX=io.thand.agent.privileged-broker-client
APP_GROUP_NOTIFIER_SUFFIX=io.thand.agent.privileged-broker-notifier
APP_GROUP_SERVER_SUFFIX=io.thand.agent.privileged-broker-server

generate_privilege_services_project() {
    require_darwin
    ensure_command xcodegen
    xcodegen generate \
        --use-cache \
        --spec "$PRIVILEGE_SERVICES_PROJECT_SPEC" \
        --project "$PRIVILEGE_SERVICES_ROOT" \
        >/dev/null
}

resolve_privilege_services_packages() {
    require_darwin
    xcodebuild \
        -project "$PRIVILEGE_SERVICES_PROJECT_FILE" \
        -scheme "$PRIVILEGE_SERVICES_SCHEME" \
        -resolvePackageDependencies \
        -clonedSourcePackagesDirPath "$PRIVILEGE_SERVICES_SOURCE_PACKAGES_DIR" \
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

privilege_services_products_dir() {
    printf '%s\n' "$DERIVED_DATA_PATH/Build/Products/$BUILD_CONFIGURATION"
}

find_apple_development_identity() {
    if [ -z "${APPLE_TEAM_ID:-}" ]; then
        echo "APPLE_TEAM_ID must be set for Apple Development signing" >&2
        exit 1
    fi

    identity=$(find_matching_codesigning_identity "Apple Development")
    if [ -z "$identity" ]; then
        echo "unable to locate an Apple Development signing identity for team ${APPLE_TEAM_ID}" >&2
        exit 1
    fi
    printf '%s\n' "$identity"
}

find_developer_id_application_identity() {
    if [ -z "${APPLE_TEAM_ID:-}" ]; then
        echo "APPLE_TEAM_ID must be set for Developer ID signing" >&2
        exit 1
    fi

    identity=$(find_matching_codesigning_identity "Developer ID Application")
    if [ -z "$identity" ]; then
        echo "unable to locate a Developer ID Application identity for team ${APPLE_TEAM_ID}" >&2
        exit 1
    fi
    printf '%s\n' "$identity"
}

find_developer_id_installer_identity() {
    if [ -z "${APPLE_TEAM_ID:-}" ]; then
        echo "APPLE_TEAM_ID must be set for Developer ID installer signing" >&2
        exit 1
    fi

    identity=$(find_matching_codesigning_identity "Developer ID Installer")
    if [ -z "$identity" ]; then
        echo "unable to locate a Developer ID Installer identity for team ${APPLE_TEAM_ID}" >&2
        exit 1
    fi
    printf '%s\n' "$identity"
}

find_matching_codesigning_identity() {
    label_prefix=$1

    security find-identity -v -p codesigning \
        | while IFS= read -r line; do
            if ! printf '%s\n' "$line" | grep -F "\"$label_prefix" >/dev/null 2>&1; then
                continue
            fi

            identity_hash=$(printf '%s\n' "$line" | sed -E 's/.* ([A-F0-9]{40}) \".*/\1/')
            identity_label=$(printf '%s\n' "$line" | sed -E 's/.* \"([^\"]+)\".*/\1/')

            if printf '%s\n' "$identity_label" | grep -F "(${APPLE_TEAM_ID})" >/dev/null 2>&1; then
                printf '%s\n' "$identity_hash"
                break
            fi

            cert_file=$(mktemp)
            security find-certificate -p -c "$identity_label" ~/Library/Keychains/login.keychain-db >"$cert_file" 2>/dev/null || true
            if [ -s "$cert_file" ]; then
                cert_subject=$(openssl x509 -in "$cert_file" -noout -subject -nameopt RFC2253 2>/dev/null || true)
                rm -f "$cert_file"
                if printf '%s\n' "$cert_subject" | grep -F "OU=${APPLE_TEAM_ID}" >/dev/null 2>&1; then
                    printf '%s\n' "$identity_hash"
                    break
                fi
            else
                rm -f "$cert_file"
            fi
        done
}

render_daemon_plist() {
    output_path=$1

    mkdir -p "$(dirname "$output_path")"
    sed \
        -e "s|__SERVICE_LABEL__|$SERVICE_LABEL|g" \
        -e "s|__STATE_DIR__|$STATE_DIR|g" \
        "$PRIVILEGE_SERVICES_ROOT/Packaging/LaunchDaemons/io.thand.agent.privilege-broker.plist.template" \
        >"$output_path"
}

prepare_stage_root() {
    stage_root=$1
    rm -rf "$stage_root"
    mkdir -p \
        "$stage_root/Applications" \
        "$stage_root/Library/Application Support/Thand/PrivilegeBroker/bin"
}

stage_unsigned_payload() {
    stage_root=$1
    products_dir=$(privilege_services_products_dir)
    app_path="$stage_root/Applications/$STAGED_APP_NAME"
    login_item_path="$app_path/Contents/Library/LoginItems/$LOGIN_ITEM_NAME"
    daemon_binary_path="$app_path/Contents/Resources/$DAEMON_BINARY_NAME"
    daemon_plist_path="$app_path/Contents/Library/LaunchDaemons/$SERVICE_LABEL.plist"

    if [ ! -d "$products_dir/$STAGED_APP_NAME" ]; then
        echo "missing app product at $products_dir/$STAGED_APP_NAME" >&2
        exit 1
    fi
    if [ ! -d "$products_dir/$LOGIN_ITEM_NAME" ]; then
        echo "missing login item product at $products_dir/$LOGIN_ITEM_NAME" >&2
        exit 1
    fi
    if [ ! -x "$products_dir/$DAEMON_BINARY_NAME" ]; then
        echo "missing daemon product at $products_dir/$DAEMON_BINARY_NAME" >&2
        exit 1
    fi
    if [ ! -x "$products_dir/$BROKER_CTL_BINARY_NAME" ]; then
        echo "missing brokerctl product at $products_dir/$BROKER_CTL_BINARY_NAME" >&2
        exit 1
    fi

    prepare_stage_root "$stage_root"

    ditto "$products_dir/$STAGED_APP_NAME" "$app_path"
    mkdir -p "$(dirname "$login_item_path")" "$(dirname "$daemon_binary_path")"
    ditto "$products_dir/$LOGIN_ITEM_NAME" "$login_item_path"
    install -m 0755 "$products_dir/$DAEMON_BINARY_NAME" "$daemon_binary_path"
    install -m 0755 "$products_dir/$BROKER_CTL_BINARY_NAME" \
        "$stage_root/Library/Application Support/Thand/PrivilegeBroker/bin/$BROKER_CTL_BINARY_NAME"
    render_daemon_plist "$daemon_plist_path"
}

codesign_component() {
    identity=$1
    entitlements=$2
    path=$3
    hardened_runtime=$4
    signing_identifier=$5

    if [ "$hardened_runtime" = "1" ]; then
        runtime_flags="--options runtime --timestamp"
    else
        runtime_flags=""
    fi

    if [ -n "$entitlements" ]; then
        # shellcheck disable=SC2086
        codesign --force --sign "$identity" $runtime_flags --identifier "$signing_identifier" --entitlements "$entitlements" "$path"
        return
    fi

    # shellcheck disable=SC2086
    codesign --force --sign "$identity" $runtime_flags --identifier "$signing_identifier" "$path"
}

sign_staged_payload() {
    stage_root=$1
    identity=$2
    hardened_runtime=$3
    include_peer_entitlements=$4

    app_path="$stage_root/Applications/$STAGED_APP_NAME"
    login_item_path="$app_path/Contents/Library/LoginItems/$LOGIN_ITEM_NAME"
    daemon_binary_path="$app_path/Contents/Resources/$DAEMON_BINARY_NAME"
    brokerctl_path="$stage_root/Library/Application Support/Thand/PrivilegeBroker/bin/$BROKER_CTL_BINARY_NAME"

    daemon_entitlements=""
    login_item_entitlements=""
    brokerctl_entitlements=""
    entitlements_root=""
    if [ "$include_peer_entitlements" = "1" ]; then
        if [ -z "${APPLE_TEAM_ID:-}" ]; then
            echo "APPLE_TEAM_ID must be set when peer entitlements are enabled" >&2
            exit 1
        fi

        entitlements_root=$(mktemp -d)
        trap 'if [ -n "$entitlements_root" ]; then rm -rf "$entitlements_root"; fi' EXIT INT TERM
        daemon_entitlements=$(render_entitlements_template \
            "$PRIVILEGE_SERVICES_ROOT/Resources/Daemon/ThandPrivilegeBrokerDaemon.entitlements.template" \
            "$entitlements_root/ThandPrivilegeBrokerDaemon.entitlements")
        login_item_entitlements=$(render_entitlements_template \
            "$PRIVILEGE_SERVICES_ROOT/Resources/LoginItem/ThandPrivilegeNotifier.entitlements.template" \
            "$entitlements_root/ThandPrivilegeNotifier.entitlements")
        brokerctl_entitlements=$(render_entitlements_template \
            "$PRIVILEGE_SERVICES_ROOT/Resources/Ctl/ThandPrivilegeBrokerCtl.entitlements.template" \
            "$entitlements_root/ThandPrivilegeBrokerCtl.entitlements")
    fi

    codesign_component "$identity" "$daemon_entitlements" "$daemon_binary_path" "$hardened_runtime" "$BROKER_SIGNING_IDENTIFIER"
    codesign_component "$identity" "$login_item_entitlements" "$login_item_path" "$hardened_runtime" "io.thand.agent.privilege-notifier"
    codesign_component "$identity" "$brokerctl_entitlements" "$brokerctl_path" "$hardened_runtime" "$BROKER_CTL_SIGNING_IDENTIFIER"
    codesign_component "$identity" "" "$app_path" "$hardened_runtime" "$APP_BUNDLE_ID"

    codesign --verify --deep --strict --verbose=2 "$app_path" >/dev/null
    codesign --verify --strict --verbose=2 "$brokerctl_path" >/dev/null

    if [ -n "$entitlements_root" ]; then
        rm -rf "$entitlements_root"
        trap - EXIT INT TERM
    fi
}

sign_local_agent_binary() {
    binary_path=$1
    identity=$2

    if [ ! -x "$binary_path" ]; then
        echo "missing local agent binary at $binary_path" >&2
        exit 1
    fi

    codesign_component "$identity" "" "$binary_path" 0 "$AGENT_SIGNING_IDENTIFIER"
    codesign --verify --strict --verbose=2 "$binary_path" >/dev/null
}

assert_plist_value() {
    plist_path=$1
    key_path=$2
    expected_value=$3
    description=$4

    actual_value=$(plutil -extract "$key_path" raw -o - "$plist_path" 2>/dev/null || true)
    if [ "$actual_value" != "$expected_value" ]; then
        echo "$description expected $key_path=$expected_value but found ${actual_value:-<missing>}" >&2
        exit 1
    fi
}

assert_plist_key_absent() {
    plist_path=$1
    key_path=$2
    description=$3

    if plutil -extract "$key_path" raw -o - "$plist_path" >/dev/null 2>&1; then
        echo "$description expected $key_path to be absent" >&2
        exit 1
    fi
}

assert_entitlement_present() {
    path=$1
    entitlement=$2
    description=$3

    entitlements_output=$(mktemp)
    if ! codesign -d --entitlements :- "$path" >"$entitlements_output" 2>/dev/null; then
        rm -f "$entitlements_output"
        echo "unable to inspect entitlements for $description at $path" >&2
        exit 1
    fi

    if ! grep -F "$entitlement" "$entitlements_output" >/dev/null 2>&1; then
        rm -f "$entitlements_output"
        echo "$description at $path is missing required entitlement $entitlement" >&2
        exit 1
    fi

    rm -f "$entitlements_output"
}

expected_app_group() {
    suffix=$1

    if [ -z "${APPLE_TEAM_ID:-}" ]; then
        echo "APPLE_TEAM_ID must be set to verify peer entitlements" >&2
        exit 1
    fi

    printf '%s.%s\n' "$APPLE_TEAM_ID" "$suffix"
}

render_entitlements_template() {
    template_path=$1
    output_path=$2

    mkdir -p "$(dirname "$output_path")"
    sed "s|__APPLE_TEAM_ID__|$APPLE_TEAM_ID|g" "$template_path" >"$output_path"
    printf '%s\n' "$output_path"
}

verify_daemon_launch_policy() {
    daemon_plist_path=$1

    assert_plist_value "$daemon_plist_path" "EnvironmentVariables.THAND_PRIVILEGE_BROKER_SERVICE_LABEL" \
        "$SERVICE_LABEL" "daemon launchd environment"
    assert_plist_value "$daemon_plist_path" "EnvironmentVariables.THAND_PRIVILEGE_BROKER_STATE_DIR" \
        "$STATE_DIR" "daemon launchd environment"

    environment_xml=$(plutil -extract EnvironmentVariables xml1 -o - "$daemon_plist_path" 2>/dev/null || true)
    environment_key_count=$(printf '%s\n' "$environment_xml" | grep -c '<key>')
    if [ "$environment_key_count" -ne 2 ]; then
        echo "daemon launchd environment expected exactly 2 environment keys but found $environment_key_count" >&2
        exit 1
    fi
}

verify_staged_layout() {
    stage_root=$1
    app_path="$stage_root/Applications/$STAGED_APP_NAME"
    login_item_path="$app_path/Contents/Library/LoginItems/$LOGIN_ITEM_NAME"
    daemon_binary_path="$app_path/Contents/Resources/$DAEMON_BINARY_NAME"
    daemon_plist_path="$app_path/Contents/Library/LaunchDaemons/$SERVICE_LABEL.plist"
    brokerctl_path="$stage_root/Library/Application Support/Thand/PrivilegeBroker/bin/$BROKER_CTL_BINARY_NAME"

    [ -d "$app_path" ]
    [ -d "$login_item_path" ]
    [ -x "$daemon_binary_path" ]
    [ -f "$daemon_plist_path" ]
    [ -x "$brokerctl_path" ]
    verify_daemon_launch_policy "$daemon_plist_path"
}

verify_signed_payload() {
    app_path=$1
    brokerctl_path=$2
    login_item_path="$app_path/Contents/Library/LoginItems/$LOGIN_ITEM_NAME"
    login_item_binary_path="$login_item_path/Contents/MacOS/ThandPrivilegeNotifier"
    daemon_binary_path="$app_path/Contents/Resources/$DAEMON_BINARY_NAME"
    daemon_plist_path="$app_path/Contents/Library/LaunchDaemons/$SERVICE_LABEL.plist"

    verify_daemon_launch_policy "$daemon_plist_path"
    codesign --verify --deep --strict --verbose=2 "$app_path" >/dev/null
    codesign --verify --strict --verbose=2 "$brokerctl_path" >/dev/null
    assert_entitlement_present "$daemon_binary_path" "$APP_GROUPS_ENTITLEMENT_KEY" "broker daemon"
    assert_entitlement_present "$daemon_binary_path" "$(expected_app_group "$APP_GROUP_SERVER_SUFFIX")" "broker daemon"
    assert_entitlement_present "$login_item_binary_path" "$APP_GROUPS_ENTITLEMENT_KEY" "notifier login item"
    assert_entitlement_present "$login_item_binary_path" "$(expected_app_group "$APP_GROUP_NOTIFIER_SUFFIX")" "notifier login item"
    assert_entitlement_present "$brokerctl_path" "$APP_GROUPS_ENTITLEMENT_KEY" "broker control helper"
    assert_entitlement_present "$brokerctl_path" "$(expected_app_group "$APP_GROUP_CLIENT_SUFFIX")" "broker control helper"
}

normalize_modes_tree() {
    target_path=$1

    find "$target_path" -type d -exec chmod 0755 {} +
    find "$target_path" -type f -perm -111 -exec chmod 0755 {} +
    find "$target_path" -type f ! -perm -111 -exec chmod 0644 {} +
}

normalize_stage_root_modes() {
    stage_root=$1

    normalize_modes_tree "$stage_root/Applications"
    normalize_modes_tree "$stage_root/Library"
}

normalize_installed_payload() {
    chown -R root:wheel "$INSTALL_APP_PATH" "$BROKER_CTL_INSTALL_DIR"
    normalize_modes_tree "$INSTALL_APP_PATH"
    normalize_modes_tree "$BROKER_CTL_INSTALL_DIR"
}

copy_stage_into_system() {
    stage_root=$1

    rm -rf "$INSTALL_APP_PATH"
    ditto "$stage_root/Applications/$STAGED_APP_NAME" "$INSTALL_APP_PATH"

    install -d "$BROKER_CTL_INSTALL_DIR"
    install -m 0755 \
        "$stage_root/Library/Application Support/Thand/PrivilegeBroker/bin/$BROKER_CTL_BINARY_NAME" \
        "$BROKER_CTL_INSTALL_DIR/$BROKER_CTL_BINARY_NAME"
}

run_privilege_services_as_user() {
    command=$1

    if [ -z "${SUDO_USER:-}" ] || [ "$SUDO_USER" = "root" ]; then
        echo "SUDO_USER is required to run ThandPrivilegeServices $command as the desktop user" >&2
        exit 1
    fi

    sudo -u "$SUDO_USER" "$INSTALL_APP_PATH/Contents/MacOS/ThandPrivilegeServices" "$command"
}
