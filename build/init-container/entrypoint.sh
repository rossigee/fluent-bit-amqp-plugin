#!/bin/sh
set -e

PLUGINS_DIR="${PLUGINS_DIR:-/plugins}"
mkdir -p "$PLUGINS_DIR"
cp /out_amqp.so "$PLUGINS_DIR/out_amqp.so"
cp /in_amqp.so "$PLUGINS_DIR/in_amqp.so"
echo "installed plugins to $PLUGINS_DIR: out_amqp.so, in_amqp.so"
