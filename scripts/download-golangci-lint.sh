#!/bin/bash
set -eo pipefail

uname_os() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    msys_nt) os="windows" ;;
  esac
  echo "$os"
}
uname_arch() {
  arch=$(uname -m)
  case $arch in
    x86_64) arch="amd64" ;;
    x86) arch="386" ;;
    i686) arch="386" ;;
    i386) arch="386" ;;
    aarch64) arch="arm64" ;;
    armv5*) arch="armv5" ;;
    armv6*) arch="armv6" ;;
    armv7*) arch="armv7" ;;
  esac
  echo ${arch}
}

CHECKSUMS="checksums.txt"
TARGET="golangci-lint.tar.gz"
VERSION=1.15.0
OS=$(uname_os)
ARCH=$(uname_arch)
PLATFORM="${OS}/${ARCH}"

tmpdir=$(mktemp -d)
pushd $tmpdir

curl -fL https://github.com/golangci/golangci-lint/releases/download/v${VERSION}/golangci-lint-${VERSION}-checksums.txt -o ${CHECKSUMS}
curl -fL https://github.com/golangci/golangci-lint/releases/download/v${VERSION}/golangci-lint-${VERSION}-${OS}-${ARCH}.tar.gz -o $TARGET
WANT=$(grep golangci-lint-${VERSION}-${OS}-${ARCH}.tar.gz ${CHECKSUMS}|cut -d ' ' -f1)
GOT=$(sha256sum ${TARGET}|cut -d ' ' -f1)
if [ "$WANT" != "$GOT" ]; then
  log_err "Couldn't verify sha256 checksum for '$TARGET'"
  exit 1
fi

tar x -C $(go env GOPATH|cut -f1 -d:)/bin --strip-components=1 -f ${TARGET} \
golangci-lint-${VERSION}-${OS}-${ARCH}/golangci-lint

popd
rm -rf $tmpdir
