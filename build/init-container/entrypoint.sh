#!/bin/sh

# Fluent Bit AMQP Plugin Init Container Entrypoint
# This script copies the plugin .so file to the shared volume

set -e

PLUGIN_NAME="${PLUGIN_NAME:-out_amqp_cloudevents.so}"
PLUGIN_SOURCE="/usr/lib/fluent-bit/${PLUGIN_NAME}"
PLUGIN_DEST="${PLUGINS_DIR:-/plugins}/${PLUGIN_NAME}"

echo "Fluent Bit AMQP Plugin Init Container"
echo "Plugin: ${PLUGIN_NAME}"
echo "Source: ${PLUGIN_SOURCE}"
echo "Destination: ${PLUGIN_DEST}"

# Check if source plugin exists
if [ ! -f "${PLUGIN_SOURCE}" ]; then
    echo "ERROR: Plugin file not found at ${PLUGIN_SOURCE}"
    exit 1
fi

# Create destination directory if it doesn't exist
mkdir -p "$(dirname "${PLUGIN_DEST}")"

# Copy plugin to shared volume
echo "Copying plugin to shared volume..."
cp "${PLUGIN_SOURCE}" "${PLUGIN_DEST}"

# Set permissions
chmod 755 "${PLUGIN_DEST}"

# Verify the copy was successful
if [ -f "${PLUGIN_DEST}" ]; then
    echo "Plugin successfully copied to ${PLUGIN_DEST}"
    echo "File size: $(du -h "${PLUGIN_DEST}" | cut -f1)"
    echo "File permissions: $(ls -la "${PLUGIN_DEST}")"
else
    echo "ERROR: Failed to copy plugin to ${PLUGIN_DEST}"
    exit 1
fi

echo "Init container completed successfully"