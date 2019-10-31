#!/usr/bin/env bash

set -eo pipefail

IMAGE_REF=$(echo "$GITHUB_REF" | rev | cut -d / -f 1 | rev)

mkdir -p ~/.ssh
echo "$SSH_PRIVATE_KEY" > ~/.ssh/id_rsa
chmod 600 ~/.ssh/id_rsa
eval "$(ssh-agent -s)" > /dev/null 2>&1
ssh-add ~/.ssh/id_rsa > /dev/null 2>&1

export GOPATH=${HOME}/go

go mod download

mkdir -p ${GOPATH}/bin

export PATH="${GOPATH}/bin:${PATH}"

make vendor

docker build -t quorumcontrol/tupelo-go-sdk:${IMAGE_REF} .
docker push quorumcontrol/tupelo-go-sdk:${IMAGE_REF}
