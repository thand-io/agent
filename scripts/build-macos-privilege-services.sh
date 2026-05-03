#!/bin/sh
set -eu

. "$(dirname "$0")/macos-privilege-services-common.sh"

require_darwin
generate_localbroker_grpc_sources
generate_privilege_services_project
run_unsigned_xcodebuild build -destination "$XCODEBUILD_BUILD_DESTINATION"
