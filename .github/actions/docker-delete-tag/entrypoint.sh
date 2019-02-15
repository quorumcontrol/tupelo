#!/usr/bin/env bash

REPO=$1

if [[ -z "$REPO" ]]; then
  echo "Must provide repo as command"
  exit 1
fi

if [[ -z "$GITHUB_EVENT_PATH" ]]; then
  echo "\$GITHUB_EVENT_PATH" not found
  exit 1
fi

TAG_FULL=$(jq --raw-output ".ref" "$GITHUB_EVENT_PATH")
# mimic https://github.com/actions/docker/blob/b12ae68bebbb2781edb562c0260881a3f86963b4/tag/tag.rb#L39
TAG=`echo $TAG_FULL | rev | cut -d / -f 1 | rev`

if [[ -z "$TAG" ]]; then
  echo "Tag could not be parsed from $GITHUB_EVENT_PATH"
  exit 1
fi

TOKEN=`curl -s -H "Content-Type: application/json" -X POST -d '{"username": "'$DOCKER_USERNAME'", "password": "'$DOCKER_PASSWORD'"}' https://hub.docker.com/v2/users/login/ | jq -r .token`

URL="https://hub.docker.com/v2/repositories/${REPO}/tags/${TAG}/"

STATUS_CODE=`curl -s -o request.out \
-w "%{http_code}" \
-X DELETE \
-H "Authorization: JWT ${TOKEN}" \
"${URL}"`

if [[ $STATUS_CODE == "204" ]]; then
  exit 0
else
  echo $URL
  echo $STATUS_CODE
  cat request.out
  exit 1
fi