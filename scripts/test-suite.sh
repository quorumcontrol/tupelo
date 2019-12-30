#!/usr/bin/env bash

make test
exitcode=$?

if [ "$exitcode" -gt "0" ]; then
  exit $exitcode
fi

# TODO: Uncomment the following when we bring WASM back in gossip4

#WASM_SDK_VERSION=master COMMUNITY_VERSION=master docker-compose -f docker-compose.integration.yml up --force-recreate --abort-on-container-exit
#exitcode=$?
#docker-compose -f docker-compose.integration.yml down --remove-orphans -v
exit $exitcode
