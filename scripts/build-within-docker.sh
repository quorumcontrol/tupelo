#!/bin/bash
set -eo pipefail

mkdir -p ~/.ssh
git config --global url."ssh://git@github.com/".insteadOf "https://github.com/"
ssh-keyscan -t rsa github.com >> ~/.ssh/known_hosts

make lint
go test -mod=readonly ./... -tags=integration
