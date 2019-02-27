#!/usr/bin/env bash

set -e

PROTOC_VERSION=3.6.1

echo "Installing protoc build deps"
sudo apt-get update > /dev/null
sudo apt-get install -y autoconf automake libtool curl make g++ unzip > /dev/null

echo "Downloading protoc source code"
curl -LO https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protobuf-all-${PROTOC_VERSION}.tar.gz > /dev/null

echo "Extracting protoc source code"
tar -xzvf protobuf-all-${PROTOC_VERSION}.tar.gz > /dev/null

cd protobuf-${PROTOC_VERSION}

echo "Configuring protoc"
./configure > /dev/null
echo "Building protoc"
make > /dev/null
echo "Installing protoc"
sudo make install > /dev/null
sudo ldconfig > /dev/null
