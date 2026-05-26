# Release Process

## Docker Image

The plugin is published to GHCR as:
- `ghcr.io/rossigee/fluent-bit-amqp-plugin:latest`
- `ghcr.io/rossigee/fluent-bit-amqp-plugin:<version>`

## Binaries

See Assets section for platform-specific binaries.

## Upgrading

Update your Dockerfile:
```dockerfile
FROM ghcr.io/rossigee/fluent-bit-amqp-plugin:latest
```