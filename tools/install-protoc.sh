#/bin/bash

set -e

PB_ARCH=${PB_ARCH:-x86_64}
PB_OS=${PB_OS:-linux}
PB_VERSION=${PB_VERSION:-21.12}
PB_REL="https://github.com/protocolbuffers/protobuf/releases"
PB_INSTALL_DIR=${PB_INSTALL_DIR:-/usr/local}

curl -LO "${PB_REL}/download/v${PB_VERSION}/protoc-${PB_VERSION}-${PB_OS}-${PB_ARCH}.zip"
unzip "protoc-${PB_VERSION}-${PB_OS}-${PB_ARCH}.zip" -d "$PB_INSTALL_DIR"
