#!/bin/bash
set -eo pipefail

uname_os() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    darwin) os="osx" ;;
    msys_nt) os="windows" ;;
  esac
  echo "$os"
}
uname_arch() {
  arch=$(uname -m)
  case $arch in
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

TARGET="protoc.zip"
VERSION=3.7.1
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
    echo "* hash_sha256 unable to find command to compute sha-256 hash"
    return 1
  fi
}

verify_hash() {
  checksums="scripts/checksums/protoc-checksums.txt"
  WANT=$(grep protoc-${VERSION}-${OS}-${ARCH}.zip ${checksums} |cut -d ' ' -f1)
  GOT=$(hash_sha256)
  if [ "$WANT" != "$GOT" ]; then
    echo "* Couldn't verify SHA256 checksum for '$TARGET'"
    exit 1
  fi
}

curl -fL https://github.com/protocolbuffers/protobuf/releases/download/v${VERSION}/protoc-${VERSION}-${OS}-${ARCH}.zip -o ${tmpdir}/$TARGET
verify_hash

dest_dir=/usr/local
unzip -u -d ${dest_dir} ${tmpdir}/${TARGET} bin/protoc

rm -rf $tmpdir

echo "* Successfully installed protoc to ${dest_dir}"
