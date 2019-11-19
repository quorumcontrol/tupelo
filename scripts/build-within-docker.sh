#!/bin/bash
set -eo pipefail

mkdir -p ~/.ssh
git config --global url."ssh://git@github.com/".insteadOf "https://github.com/"
ssh-keyscan -t rsa github.com >> ~/.ssh/known_hosts

if [[ "${CI}" == "true" ]]; then
  sudo ./scripts/download-protoc.sh
fi

go mod download
make lint
if [[ "${CI}" == "true" ]]; then
  make ci-test
else
  make test
fi
