#!/bin/bash

if [[ ! -z "$SSH_PRIVATE_KEY" ]]; then
  mkdir -p /ssh
  echo "$SSH_PRIVATE_KEY" > /ssh/id_rsa
  chmod 600 /ssh/id_rsa
  eval "$(ssh-agent -s)" > /dev/null 2>&1
  ssh-add /ssh/id_rsa > /dev/null 2>&1
fi

build_dir=$GOPATH/src/github.com/$GITHUB_REPOSITORY

mkdir -p $(dirname $build_dir)

ln -s $GITHUB_WORKSPACE $build_dir

cd $build_dir

exec /go/bin/dep "$@"
