#!/usr/bin/env bash

trap "kill 0" EXIT

run_dir=$(pwd)/local-run/
keys_dir=$run_dir/keys
num_nodes=${1:-3}

rm -rf $run_dir && mkdir -p $run_dir && mkdir -p $keys_dir
rm -rf "~/Library/Application Support/tupelo/default/distributed-network"

local_ip=$(ifconfig | grep -Eo 'inet (addr:)?([0-9]*\.){3}[0-9]*' | grep -Eo '([0-9]*\.){3}[0-9]*' | grep -v '127.0.0.1' | head -n 1)

export TUPELO_NODE_BLS_KEY_HEX="0x1ef1cf13d52b2dbf3134d7fff6aa617c5cdac42ed89bd20007bfc93d00f0d0c6"
export TUPELO_NODE_ECDSA_KEY_HEX="0xe2c0b170c56ff0bf08c7376f1ca4bd5cbe481a85f6cdb3de609863e25dce613a"
export TUPELO_BOOTSTRAP_NODES="/ip4/${local_ip}/tcp/34001/ipfs/16Uiu2HAm3TGSEKEjagcCojSJeaT5rypaeJMKejijvYSnAjviWwV5"
go run main.go bootstrap-node -p 34001 &

go run main.go generate-node-keys -c $num_nodes -o json-file -p $keys_dir

for i in $(seq 1 $num_nodes); do
  node_index=$(expr $i - 1)
  export TUPELO_NODE_BLS_KEY_HEX=$(cat $keys_dir/private-keys.json | jq -r ".[${node_index}].blsHexPrivateKey")
  export TUPELO_NODE_ECDSA_KEY_HEX=$(cat $keys_dir/private-keys.json | jq -r ".[${node_index}].ecdsaHexPrivateKey")
  go run main.go test-node -k $keys_dir 2>&1 | tee $run_dir/node$i.log &
done

sleep 60

go run main.go benchmark -k $keys_dir -c 2 -i 60 -s tps -t 5

wait
