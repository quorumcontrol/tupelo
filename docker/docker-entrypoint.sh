#!/bin/sh
set -e

export TUPELO_PORT=${TUPELO_PORT:-"0"}
export TUPELO_WEB_SOCKET_PORT=${TUPELO_WEB_SOCKET_PORT:-"0"}
export TUPELO_BOOTSTRAP_ONLY=${TUPELO_BOOTSTRAP_ONLY:-"false"}
export TUPELO_CONFIG_PATH=${TUPELO_CONFIG_PATH:-"/tupelo"}
export TUPELO_CONFIG_FILE=${TUPELO_CONFIG_FILE:-"$TUPELO_CONFIG_PATH/config.toml"}
export TUPELO_NOTARY_GROUP_CONFIG=${TUPELO_NOTARY_GROUP_CONFIG:-"$TUPELO_CONFIG_PATH/notarygroup.toml"}
export TUPELO_GOSSIP3_NOTARY_GROUP_CONFIG=${TUPELO_GOSSIP3_NOTARY_GROUP_CONFIG:-"$TUPELO_CONFIG_PATH/gossip3_notarygroup.toml"}
export TUPELO_STORAGE_PATH=${TUPELO_STORAGE_PATH:-"$TUPELO_CONFIG_PATH/data"}
export TUPELO_CERTIFICATE_CACHE=${TUPELO_CERTIFICATE_CACHE:-"$TUPELO_CONFIG_PATH/certs"}
export TUPELO_TRACING_SYSTEM=${TUPELO_TRACING_SYSTEM}

mkdir -p $TUPELO_CONFIG_PATH

if [ -n "$TUPELO_NOTARY_GROUP_URL" ]; then
  wget -q -O "$TUPELO_NOTARY_GROUP_CONFIG" "$TUPELO_NOTARY_GROUP_URL"
fi

if [ -n "$TUPELO_GOSSIP3_NOTARY_GROUP_URL" ]; then
  wget -q -O "$TUPELO_GOSSIP3_NOTARY_GROUP_CONFIG" "$TUPELO_GOSSIP3_NOTARY_GROUP_URL"
fi

if [ -n "$TUPELO_ADVERTISE_EC2_PUBLIC_IP" ]; then
  TUPELO_PUBLIC_IP=$(curl -s http://169.254.169.254/latest/meta-data/public-ipv4)
  export TUPELO_PUBLIC_IP
fi

# If no config file exists, symlink in a generated one
if [ ! -e "$TUPELO_CONFIG_FILE" ]; then
  touch "$TUPELO_CONFIG_FILE.generated"
  ln -s "$TUPELO_CONFIG_FILE.generated" "$TUPELO_CONFIG_FILE"
fi
# Then, if generated file exists, regenerate on each boot to pickup any changes
# This is critical when a named volume is mounted into the `/tupelo` directory
# else config is static from first boot
if [ -f "$TUPELO_CONFIG_FILE.generated" ]; then
  envsubst < /tupelo/config.toml.tpl  > "$TUPELO_CONFIG_FILE.generated"
fi

exec /usr/bin/tupelo --config "$TUPELO_CONFIG_FILE" "$@"
