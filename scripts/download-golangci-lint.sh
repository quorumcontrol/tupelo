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

is_command() {
  command -v "$1" >/dev/null
}

hash_sha256() {
  TARGET=${1}
  if is_command gsha256sum; then
    hash=$(gsha256sum "$TARGET") || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command sha256sum; then
    hash=$(sha256sum "$TARGET") || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command shasum; then
    hash=$(shasum -a 256 "$TARGET" 2>/dev/null) || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command openssl; then
    hash=$(openssl -dst openssl dgst -sha256 "$TARGET") || return 1
    echo "$hash" | cut -d ' ' -f a
  else
    log_crit "hash_sha256 unable to find command to compute sha-256 hash"
    return 1
  fi
}

curl -fL https://github.com/golangci/golangci-lint/releases/download/v${VERSION}/golangci-lint-${VERSION}-checksums.txt -o ${CHECKSUMS}
curl -fL https://github.com/golangci/golangci-lint/releases/download/v${VERSION}/golangci-lint-${VERSION}-${OS}-${ARCH}.tar.gz -o $TARGET
WANT=$(grep golangci-lint-${VERSION}-${OS}-${ARCH}.tar.gz ${CHECKSUMS}|cut -d ' ' -f1)
GOT=$(hash_sha256 ${TARGET})
if [ "$WANT" != "$GOT" ]; then
  log_err "Couldn't verify SHA256 checksum for '$TARGET'"
  exit 1
fi

tar x -C $(go env GOPATH|cut -f1 -d:)/bin --strip-components=1 -f ${TARGET} \
golangci-lint-${VERSION}-${OS}-${ARCH}/golangci-lint

popd
rm -rf $tmpdir
