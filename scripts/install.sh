#!/bin/bash
# filepath: /Users/admin/Documents/agent/scripts/install.sh

set -e

# Configuration
REPO_OWNER="thand-io"
REPO_NAME="agent"
GITHUB_API_URL="https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="thand-agent"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to detect OS and architecture
detect_platform() {
    local os=""
    local arch=""
    
    # Detect OS
    case "$(uname -s)" in
        Darwin)
            os="darwin"
            ;;
        Linux)
            os="linux"
            ;;
        MINGW*|MSYS*|CYGWIN*)
            os="windows"
            ;;
        *)
            print_error "Unsupported operating system: $(uname -s)"
            exit 1
            ;;
    esac
    
    # Detect architecture
    case "$(uname -m)" in
        x86_64|amd64)
            arch="amd64"
            ;;
        arm64|aarch64)
            arch="arm64"
            ;;
        armv7l)
            arch="arm"
            ;;
        i386|i686)
            arch="386"
            ;;
        *)
            print_error "Unsupported architecture: $(uname -m)"
            exit 1
            ;;
    esac
    
    echo "${os}_${arch}"
}

# Function to get latest release info from GitHub
get_latest_release() {
    print_status "Fetching latest release information..."
    
    if command -v curl >/dev/null 2>&1; then
        curl -s "$GITHUB_API_URL"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO- "$GITHUB_API_URL"
    else
        print_error "Neither curl nor wget is available. Please install one of them."
        exit 1
    fi
}

# Function to extract download URL for platform
get_download_url() {
    local release_json="$1"
    local platform="$2"
    local extension=""
    
    # Set file extension based on OS
    if [[ "$platform" == *"windows"* ]]; then
        extension=".exe"
    fi
    
    # Extract download URL using grep and sed (more portable than jq)
    echo "$release_json" | grep -o "\"browser_download_url\":[^,]*${platform}[^,]*${extension}\"" | sed 's/.*"browser_download_url":"\([^"]*\)".*/\1/' | head -1
}

# Function to get version from release
get_version() {
    local release_json="$1"
    echo "$release_json" | grep -o '"tag_name":"[^"]*"' | sed 's/.*"tag_name":"\([^"]*\)".*/\1/'
}

# Function to download and install binary
install_binary() {
    local download_url="$1"
    local version="$2"
    local platform="$3"
    
    if [[ -z "$download_url" ]]; then
        print_error "No download URL found for platform: $platform"
        print_error "Please check if a release exists for your platform at: https://github.com/${REPO_OWNER}/${REPO_NAME}/releases"
        exit 1
    fi
    
    print_status "Downloading ${BINARY_NAME} ${version} for ${platform}..."
    print_status "Download URL: $download_url"
    
    # Create temporary directory
    local temp_dir=$(mktemp -d)
    local filename=$(basename "$download_url")
    local temp_file="${temp_dir}/${filename}"
    
    # Download the file
    if command -v curl >/dev/null 2>&1; then
        curl -L -o "$temp_file" "$download_url"
    elif command -v wget >/dev/null 2>&1; then
        wget -O "$temp_file" "$download_url"
    fi
    
    if [[ ! -f "$temp_file" ]]; then
        print_error "Failed to download the binary"
        exit 1
    fi
    
    # Handle different archive formats
    local binary_path=""
    if [[ "$filename" == *.tar.gz ]]; then
        print_status "Extracting tar.gz archive..."
        tar -xzf "$temp_file" -C "$temp_dir"
        binary_path=$(find "$temp_dir" -name "$BINARY_NAME*" -type f -executable | head -1)
    elif [[ "$filename" == *.zip ]]; then
        print_status "Extracting zip archive..."
        if command -v unzip >/dev/null 2>&1; then
            unzip -q "$temp_file" -d "$temp_dir"
            binary_path=$(find "$temp_dir" -name "$BINARY_NAME*" -type f | head -1)
        else
            print_error "unzip command not found. Please install unzip to extract .zip files."
            exit 1
        fi
    else
        # Assume it's a direct binary
        binary_path="$temp_file"
    fi
    
    if [[ -z "$binary_path" || ! -f "$binary_path" ]]; then
        print_error "Could not find binary in downloaded archive"
        exit 1
    fi
    
    # Make binary executable
    chmod +x "$binary_path"
    
    # Install binary
    print_status "Installing to ${INSTALL_DIR}/${BINARY_NAME}..."
    
    # Check if we need sudo
    if [[ ! -w "$INSTALL_DIR" ]]; then
        print_warning "Installing to ${INSTALL_DIR} requires sudo privileges"
        sudo mv "$binary_path" "${INSTALL_DIR}/${BINARY_NAME}"
    else
        mv "$binary_path" "${INSTALL_DIR}/${BINARY_NAME}"
    fi
    
    # Cleanup
    rm -rf "$temp_dir"
    
    print_status "Installation completed successfully!"
    print_status "Binary installed at: ${INSTALL_DIR}/${BINARY_NAME}"
    
    # Verify installation
    if command -v "$BINARY_NAME" >/dev/null 2>&1; then
        print_status "Verification: $(which $BINARY_NAME)"
        print_status "Version: $($BINARY_NAME --version 2>/dev/null || echo "Version command not available")"
    else
        print_warning "${INSTALL_DIR} may not be in your PATH"
        print_warning "Add ${INSTALL_DIR} to your PATH or use the full path: ${INSTALL_DIR}/${BINARY_NAME}"
    fi
}

# Main installation function
main() {
    print_status "Starting thand-agent installation..."
    
    # Detect platform
    local platform=$(detect_platform)
    print_status "Detected platform: $platform"
    
    # Get latest release information
    local release_json=$(get_latest_release)
    
    if [[ -z "$release_json" ]]; then
        print_error "Failed to fetch release information from GitHub"
        exit 1
    fi
    
    # Extract version and download URL
    local version=$(get_version "$release_json")
    local download_url=$(get_download_url "$release_json" "$platform")
    
    if [[ -z "$version" ]]; then
        print_error "Could not determine latest version"
        exit 1
    fi
    
    print_status "Latest version: $version"
    
    # Check if already installed
    if command -v "$BINARY_NAME" >/dev/null 2>&1; then
        local current_version=$($BINARY_NAME --version 2>/dev/null | grep -o 'v[0-9.]*' || echo "unknown")
        if [[ "$current_version" == "$version" ]]; then
            print_status "thand-agent $version is already installed"
            exit 0
        else
            print_status "Upgrading from $current_version to $version"
        fi
    fi
    
    # Download and install
    install_binary "$download_url" "$version" "$platform"
}

# Check if running as script (not sourced)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
