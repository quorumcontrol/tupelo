#!/usr/bin/env bash

# sources contains both stretch/updates and stretch-updates, the former causes a 404
sed -i '/stretch\/updates/s/^/#/g' /etc/apt/sources.list

apt-get update

apt-get install -y \
  apt-transport-https \
  ca-certificates \
  curl \
  gnupg2 \
  software-properties-common

curl -fsSL https://download.docker.com/linux/debian/gpg | apt-key add -

if !(APT_KEY_DONT_WARN_ON_DANGEROUS_USAGE=1 apt-key fingerprint 0EBFCD88 | grep -q "9DC8 5822 9FC7 DD38 854A  E2D8 8D81 803C 0EBF CD88"); then
  echo "docker keys did not match"
  exit 1
fi

add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/debian $(lsb_release -cs) stable"

apt-get update

apt-get install -y docker-ce docker-compose
