#!/usr/bin/env bash

make test
exitcode=$?

if [ "$exitcode" -gt "0" ]; then
  exit $exitcode
fi

WASM_SDK_VERSION=master COMMUNITY_VERSION=master docker-compose -f docker-compose.integration.yml up --force-recreate --abort-on-container-exit
exitcode=$?
docker-compose -f docker-compose.integration.yml down --remove-orphans -v
exit $exitcode