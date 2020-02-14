#!/bin/bash

set -v

docker-compose up -d --force-recreate --build

sleep 120

curl http://localhost:8080/debug/pprof/heap > debugging/$(date +%s)-before-run.heap
curl http://localhost:8080/debug/pprof/trace\?seconds\=30 > debugging/$(date +%s)-before-run.trace

doRun() {
  prefix=$1

  sleep 30

  # run is 20s delay, 30s timeout, 300s run
  nohup docker-compose -f docker-compose.yml -f docker-compose-benchmark.yml run benchmark > debugging/${prefix}-results.json 2>&1 &

  sleep 150
  curl http://localhost:8080/debug/pprof/heap > debugging/$(date +%s)-during-${prefix}.heap
  curl http://localhost:8080/debug/pprof/trace\?seconds\=30 > debugging/$(date +%s)-during-${prefix}.trace

  while [ ! -f debugging/${prefix}-results.json ]; do
    sleep 2
  done

  sleep 5

  mv debugging/${prefix}-results.json debugging/$(date +%s)-${prefix}-results.json

  sleep 2

  curl http://localhost:8080/debug/pprof/heap > debugging/$(date +%s)-right-after-${prefix}.heap
  curl http://localhost:8080/debug/pprof/trace\?seconds\=30 > debugging/$(date +%s)-right-after-${prefix}.trace

  sleep 120

  curl http://localhost:8080/debug/pprof/heap > debugging/$(date +%s)-120-after-${prefix}.heap
  curl http://localhost:8080/debug/pprof/trace\?seconds\=30 > debugging/$(date +%s)-120-after-${prefix}.trace
}


doRun "run1"
doRun "run2"
doRun "run3"
doRun "run4"

mkdir -p debugging/logs/

docker-compose logs bootstrap > debugging/logs/bootstrap.log 2>&1
docker-compose logs node0 > debugging/logs/node0.log 2>&1
docker-compose logs node1 > debugging/logs/node1.log 2>&1
docker-compose logs node2 > debugging/logs/node2.log 2>&1
docker-compose logs node3 > debugging/logs/node3.log 2>&1
docker-compose logs node4 > debugging/logs/node4.log 2>&1
docker-compose logs node5 > debugging/logs/node5.log 2>&1
docker-compose logs node6 > debugging/logs/node6.log 2>&1
docker-compose logs node7 > debugging/logs/node7.log 2>&1
docker-compose logs node8 > debugging/logs/node8.log 2>&1