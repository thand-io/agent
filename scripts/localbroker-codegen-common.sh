#!/bin/sh
set -eu

REPO_ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)

LOCALBROKER_GO_GENERATED_DIR=${LOCALBROKER_GO_GENERATED_DIR:-"$REPO_ROOT/internal/localbroker/proto/localbroker/v1"}
LOCALBROKER_SWIFT_GENERATED_DIR=${LOCALBROKER_SWIFT_GENERATED_DIR:-"$REPO_ROOT/platform/macos/PrivilegeServices/Generated/LocalBroker"}
LOCALBROKER_CODEGEN_TOOLS_ROOT=${LOCALBROKER_CODEGEN_TOOLS_ROOT:-"$REPO_ROOT/.build/localbroker/CodegenTools"}
LOCALBROKER_CODEGEN_BIN_DIR=${LOCALBROKER_CODEGEN_BIN_DIR:-"$LOCALBROKER_CODEGEN_TOOLS_ROOT/bin"}
LOCALBROKER_CODEGEN_SRC_DIR=${LOCALBROKER_CODEGEN_SRC_DIR:-"$LOCALBROKER_CODEGEN_TOOLS_ROOT/src"}

LOCALBROKER_BUF_VERSION=${LOCALBROKER_BUF_VERSION:-v1.56.0}
LOCALBROKER_PROTOC_GEN_GO_VERSION=${LOCALBROKER_PROTOC_GEN_GO_VERSION:-v1.36.11}
LOCALBROKER_PROTOC_GEN_GO_GRPC_VERSION=${LOCALBROKER_PROTOC_GEN_GO_GRPC_VERSION:-v1.6.1}
LOCALBROKER_SWIFT_PROTOBUF_VERSION=${LOCALBROKER_SWIFT_PROTOBUF_VERSION:-1.37.0}
LOCALBROKER_GRPC_SWIFT_PROTOBUF_VERSION=${LOCALBROKER_GRPC_SWIFT_PROTOBUF_VERSION:-2.3.0}

ensure_command() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "missing required command: $1" >&2
        exit 1
    fi
}

require_darwin() {
    if [ "$(uname -s)" != "Darwin" ]; then
        echo "macOS privilege services can only be built on Darwin hosts" >&2
        exit 1
    fi
}

install_go_codegen_tool() {
    binary_name=$1
    package_name=$2
    version=$3
    version_marker="$LOCALBROKER_CODEGEN_BIN_DIR/.$binary_name.version"

    if [ -x "$LOCALBROKER_CODEGEN_BIN_DIR/$binary_name" ] &&
        [ -f "$version_marker" ] &&
        [ "$(cat "$version_marker")" = "$version" ]; then
        return
    fi

    GOBIN="$LOCALBROKER_CODEGEN_BIN_DIR" GOEXPERIMENT=jsonv2 \
        go install "$package_name@$version"
    printf '%s\n' "$version" >"$version_marker"
}

bootstrap_localbroker_go_codegen_tools() {
    ensure_command go
    mkdir -p "$LOCALBROKER_CODEGEN_BIN_DIR"

    install_go_codegen_tool buf github.com/bufbuild/buf/cmd/buf "$LOCALBROKER_BUF_VERSION"
    install_go_codegen_tool protoc-gen-go google.golang.org/protobuf/cmd/protoc-gen-go "$LOCALBROKER_PROTOC_GEN_GO_VERSION"
    install_go_codegen_tool protoc-gen-go-grpc google.golang.org/grpc/cmd/protoc-gen-go-grpc "$LOCALBROKER_PROTOC_GEN_GO_GRPC_VERSION"
}

ensure_codegen_source_checkout() {
    name=$1
    repo_url=$2
    version=$3
    checkout_path="$LOCALBROKER_CODEGEN_SRC_DIR/$name-$version"

    mkdir -p "$LOCALBROKER_CODEGEN_SRC_DIR"
    if [ -d "$checkout_path/.git" ]; then
        current_tag=$(git -C "$checkout_path" describe --tags --exact-match 2>/dev/null || true)
        if [ "$current_tag" = "$version" ]; then
            printf '%s\n' "$checkout_path"
            return
        fi
        rm -rf "$checkout_path"
    fi

    git clone --depth 1 --branch "$version" "$repo_url" "$checkout_path" >/dev/null 2>&1
    printf '%s\n' "$checkout_path"
}

bootstrap_localbroker_swift_codegen_tools() {
    require_darwin
    ensure_command swift
    ensure_command git
    mkdir -p "$LOCALBROKER_CODEGEN_BIN_DIR"

    if [ ! -x "$LOCALBROKER_CODEGEN_BIN_DIR/protoc-gen-swift" ]; then
        swift_protobuf_checkout=$(ensure_codegen_source_checkout "swift-protobuf" "https://github.com/apple/swift-protobuf.git" "$LOCALBROKER_SWIFT_PROTOBUF_VERSION")
        swift build \
            --package-path "$swift_protobuf_checkout" \
            --product protoc-gen-swift \
            --configuration release \
            --scratch-path "$LOCALBROKER_CODEGEN_TOOLS_ROOT/build/swift-protobuf" \
            >/dev/null
        cp \
            "$LOCALBROKER_CODEGEN_TOOLS_ROOT/build/swift-protobuf/release/protoc-gen-swift" \
            "$LOCALBROKER_CODEGEN_BIN_DIR/protoc-gen-swift"
    fi

    if [ ! -x "$LOCALBROKER_CODEGEN_BIN_DIR/protoc-gen-grpc-swift-2" ]; then
        grpc_swift_protobuf_checkout=$(ensure_codegen_source_checkout "grpc-swift-protobuf" "https://github.com/grpc/grpc-swift-protobuf.git" "$LOCALBROKER_GRPC_SWIFT_PROTOBUF_VERSION")
        swift build \
            --package-path "$grpc_swift_protobuf_checkout" \
            --product protoc-gen-grpc-swift-2 \
            --configuration release \
            --scratch-path "$LOCALBROKER_CODEGEN_TOOLS_ROOT/build/grpc-swift-protobuf" \
            >/dev/null
        cp \
            "$LOCALBROKER_CODEGEN_TOOLS_ROOT/build/grpc-swift-protobuf/release/protoc-gen-grpc-swift-2" \
            "$LOCALBROKER_CODEGEN_BIN_DIR/protoc-gen-grpc-swift-2"
    fi
}

generate_localbroker_go_grpc_sources() {
    bootstrap_localbroker_go_codegen_tools

    rm -f "$LOCALBROKER_GO_GENERATED_DIR"/*.pb.go
    mkdir -p "$LOCALBROKER_GO_GENERATED_DIR"

    PATH="$LOCALBROKER_CODEGEN_BIN_DIR:$PATH" \
        "$LOCALBROKER_CODEGEN_BIN_DIR/buf" generate \
        --template "$REPO_ROOT/buf.gen.go.yaml" \
        "$REPO_ROOT"
}

generate_localbroker_swift_grpc_sources() {
    require_darwin
    bootstrap_localbroker_go_codegen_tools
    bootstrap_localbroker_swift_codegen_tools

    rm -rf "$LOCALBROKER_SWIFT_GENERATED_DIR"
    mkdir -p "$LOCALBROKER_SWIFT_GENERATED_DIR"

    PATH="$LOCALBROKER_CODEGEN_BIN_DIR:$PATH" \
        "$LOCALBROKER_CODEGEN_BIN_DIR/buf" generate \
        --template "$REPO_ROOT/buf.gen.swift.yaml" \
        "$REPO_ROOT"
}

generate_localbroker_grpc_sources() {
    generate_localbroker_go_grpc_sources
    if [ "$(uname -s)" = "Darwin" ]; then
        generate_localbroker_swift_grpc_sources
    fi
}

verify_localbroker_grpc_sources() {
    temp_root=$(mktemp -d)
    trap 'rm -rf "$temp_root"' EXIT INT TERM

    bootstrap_localbroker_go_codegen_tools
    mkdir -p "$temp_root/internal/localbroker/proto"
    temp_go_template="$temp_root/buf.gen.go.yaml"
    sed "s#out: internal/localbroker/proto#out: $temp_root/internal/localbroker/proto#g" \
        "$REPO_ROOT/buf.gen.go.yaml" >"$temp_go_template"
    PATH="$LOCALBROKER_CODEGEN_BIN_DIR:$PATH" \
        "$LOCALBROKER_CODEGEN_BIN_DIR/buf" generate \
        --template "$temp_go_template" \
        "$REPO_ROOT"
    diff -ru "$LOCALBROKER_GO_GENERATED_DIR" "$temp_root/internal/localbroker/proto/localbroker/v1" >/dev/null

    if [ "$(uname -s)" = "Darwin" ]; then
        bootstrap_localbroker_swift_codegen_tools
        mkdir -p "$temp_root/platform/macos/PrivilegeServices/Generated"
        temp_swift_template="$temp_root/buf.gen.swift.yaml"
        sed "s#out: platform/macos/PrivilegeServices/Generated/LocalBroker#out: $temp_root/platform/macos/PrivilegeServices/Generated/LocalBroker#g" \
            "$REPO_ROOT/buf.gen.swift.yaml" >"$temp_swift_template"
        PATH="$LOCALBROKER_CODEGEN_BIN_DIR:$PATH" \
            "$LOCALBROKER_CODEGEN_BIN_DIR/buf" generate \
            --template "$temp_swift_template" \
            "$REPO_ROOT"
        diff -ru "$LOCALBROKER_SWIFT_GENERATED_DIR" "$temp_root/platform/macos/PrivilegeServices/Generated/LocalBroker" >/dev/null
    fi
}
