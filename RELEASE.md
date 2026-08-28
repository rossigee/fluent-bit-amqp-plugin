# Release Process

## Versioning

The release version lives in the `VERSION` file. Bump it (e.g. `make version-patch`),
commit, then tag and push:

```bash
git tag v$(cat VERSION)
git push origin --tags
```

Pushing a `v*` tag triggers `.github/workflows/release.yml`.

## What the release produces

- **Plugin binaries** (`out_amqp-linux-amd64.so`, and `out_amqp-linux-arm64.so` when
  the ARM64 build succeeds) attached to the GitHub release, with a `checksums.txt`.
- **Main image** `ghcr.io/rossigee/fluent-bit-amqp-plugin:<version>` and `:latest` —
  the official Fluent Bit image with the plugin pre-installed.
- **Init image** `ghcr.io/rossigee/fluent-bit-amqp-plugin-init:<version>` and `:latest` —
  copies the plugin `.so` into a shared volume for the Kubernetes DaemonSet.

## Plugin build notes

The plugin is a `glibc` shared object, not a standalone binary. It is compiled inside a
Debian bookworm builder image (matching `fluent/fluent-bit`'s glibc) so it loads cleanly
when Fluent Bit `dlopen`s it. A musl/fully-static `.so` segfaults inside the glibc
Fluent Bit process, so it is intentionally not used.

## Docker Image

The plugin is published to GHCR as:
- `ghcr.io/rossigee/fluent-bit-amqp-plugin:latest`
- `ghcr.io/rossigee/fluent-bit-amqp-plugin:<version>`
- `ghcr.io/rossigee/fluent-bit-amqp-plugin-init:latest`
- `ghcr.io/rossigee/fluent-bit-amqp-plugin-init:<version>`

## Upgrading

Update your Dockerfile:
```dockerfile
FROM ghcr.io/rossigee/fluent-bit-amqp-plugin:latest
```
