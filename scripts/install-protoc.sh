#!/usr/bin/env bash

set -e

PROTOC_VERSION=3.6.1

sudo apt-get update
sudo apt-get install -y autoconf automake libtool curl make g++ unzip

curl -LO https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protobuf-all-${PROTOC_VERSION}.tar.gz

tar -xzvf protobuf-all-${PROTOC_VERSION}.tar.gz

cd protobuf-${PROTOC_VERSION}

./configure
make
sudo make install
sudo ldconfig