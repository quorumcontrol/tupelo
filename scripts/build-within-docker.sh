#!/bin/bash
set -eo pipefail

mkdir -p ~/.ssh
echo "$SSH_KEY" > ~/.ssh/id_rsa
chmod 600 ~/.ssh/id_rsa
eval `ssh-agent`
ssh-add ~/.ssh/id_rsa
git config --global url."ssh://git@github.com/".insteadOf "https://github.com/"
ssh-keyscan -t rsa github.com >> ~/.ssh/known_hosts
./scripts/download-golangci-lint.sh

go mod download
golangci-lint run
go test -mod=readonly ./... -tags=integration
