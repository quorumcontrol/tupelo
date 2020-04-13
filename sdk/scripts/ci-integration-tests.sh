#!/usr/bin/env bash

set -eo pipefail

mkdir -p ~/.ssh
echo "$SSH_PRIVATE_KEY" > ~/.ssh/id_rsa
chmod 600 ~/.ssh/id_rsa
eval "$(ssh-agent -s)" > /dev/null 2>&1
ssh-add ~/.ssh/id_rsa > /dev/null 2>&1

export GOPATH=${HOME}/go

go mod download

mkdir -p ${GOPATH}/bin

export PATH="${GOPATH}/bin:${PATH}"

echo -e "\n\nRunning integration tests against tupelo master"
make integration-test TUPELO=master

# echo -e "\n\nRunning integration tests against tupelo latest release"
# make integration-test TUPELO=latest
