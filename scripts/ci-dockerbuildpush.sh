#!/usr/bin/env bash

set -eo pipefail

IMAGE_REF=$(echo "$GITHUB_REF" | rev | cut -d / -f 1 | rev)

docker build -t quorumcontrol/tupelo-go-sdk:${IMAGE_REF} .
docker push quorumcontrol/tupelo-go-sdk:${IMAGE_REF}
