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

TARGET="golangci-lint.tar.gz"
VERSION=1.15.0
OS=$(uname_os)
ARCH=$(uname_arch)
PLATFORM="${OS}/${ARCH}"

tmpdir=$(mktemp -d)

is_command() {
  command -v "$1" >/dev/null
}

hash_sha256() {
  target_abs=${tmpdir}/${TARGET}
  if is_command gsha256sum; then
    hash=$(gsha256sum "$target_abs") || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command sha256sum; then
    hash=$(sha256sum "$target_abs") || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command shasum; then
    hash=$(shasum -a 256 "$target_abs" 2>/dev/null) || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command openssl; then
    hash=$(openssl -dst openssl dgst -sha256 "$target_abs") || return 1
    echo "$hash" | cut -d ' ' -f a
  else
    log_crit "hash_sha256 unable to find command to compute sha-256 hash"
    return 1
  fi
}

verify_hash() {
  checksums="scripts/checksums/golangci-lint-checksums.txt"
  WANT=$(grep golangci-lint-${VERSION}-${OS}-${ARCH}.tar.gz ${checksums} |cut -d ' ' -f1)
  GOT=$(hash_sha256)
  if [ "$WANT" != "$GOT" ]; then
    log_err "Couldn't verify SHA256 checksum for '$TARGET'"
    exit 1
  fi
}

curl -fL https://github.com/golangci/golangci-lint/releases/download/v${VERSION}/golangci-lint-${VERSION}-${OS}-${ARCH}.tar.gz -o ${tmpdir}/$TARGET
verify_hash

dest_dir=$(go env GOPATH|cut -f1 -d:)/bin
tar x -C ${dest_dir} --strip-components=1 -f ${tmpdir}/${TARGET} \
golangci-lint-${VERSION}-${OS}-${ARCH}/golangci-lint

rm -rf $tmpdir

echo "* Successfully installed golangci-lint to ${dest_dir}"
