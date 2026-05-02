#!/bin/sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "uninstall-macos-privilege-services-dev.sh must run as root" >&2
    exit 1
fi

. "$(dirname "$0")/macos-privilege-services-common.sh"

if [ -d "$INSTALL_APP_PATH" ] && [ -n "${SUDO_USER:-}" ] && [ "$SUDO_USER" != "root" ]; then
    sudo -u "$SUDO_USER" "$INSTALL_APP_PATH/Contents/MacOS/ThandPrivilegeServices" unregister >/dev/null 2>&1 || true
fi

rm -rf "$INSTALL_APP_PATH"
rm -f "$BROKER_CTL_INSTALL_DIR/$BROKER_CTL_BINARY_NAME"
rmdir "$BROKER_CTL_INSTALL_DIR" 2>/dev/null || true

printf 'Removed macOS privilege services app from %s\n' "$INSTALL_APP_PATH"
