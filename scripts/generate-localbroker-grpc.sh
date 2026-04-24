#!/bin/sh
set -eu

. "$(dirname "$0")/localbroker-codegen-common.sh"

generate_localbroker_grpc_sources
