#!/usr/bin/env bash

set -eo pipefail

git config --global url."ssh://git@github.com/".insteadOf "https://github.com/"

mkdir -p ~/.ssh
ssh-keyscan -t rsa github.com > github.pub
diff <(ssh-keygen -lf github.pub) <(echo "2048 SHA256:nThbg6kXUpJWGl7E1IGOCspRomTxdCARLviKw6E5SY8 github.com (RSA)")
cat github.pub >> ~/.ssh/known_hosts
rm -f github.pub
