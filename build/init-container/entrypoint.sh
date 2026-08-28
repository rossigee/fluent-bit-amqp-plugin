#!/bin/sh
set -e

PLUGINS_DIR="${PLUGINS_DIR:-/plugins}"
mkdir -p "$PLUGINS_DIR"
cp /out_amqp.so "$PLUGINS_DIR/out_amqp.so"
echo "installed plugin to $PLUGINS_DIR/out_amqp.so"
