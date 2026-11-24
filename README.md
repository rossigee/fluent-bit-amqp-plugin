# Fluent Bit AMQP Output Plugin

[![CI](https://github.com/rossigee/fluent-bit-amqp-plugin/actions/workflows/ci.yml/badge.svg)](https://github.com/rossigee/fluent-bit-amqp-plugin/actions/workflows/ci.yml)
[![Release](https://github.com/rossigee/fluent-bit-amqp-plugin/actions/workflows/release.yml/badge.svg)](https://github.com/rossigee/fluent-bit-amqp-plugin/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/rossigee/fluent-bit-amqp-plugin)](https://goreportcard.com/report/github.com/rossigee/fluent-bit-amqp-plugin)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

A production-ready Fluent Bit output plugin that sends log records to AMQP queues (RabbitMQ) as either plain JSON or [CloudEvents](https://cloudevents.io/), selectable per output block.

## ✨ Features

- **Plain JSON or CloudEvents**: Choose the format that suits your consumers — defaults to CloudEvents
- **AMQP 0.9.1 Support**: Direct publishing to RabbitMQ/AMQP brokers
- **Automatic Reconnection**: Handles connection failures gracefully with retry logic
- **Kubernetes Ready**: Custom container image with plugin pre-installed
- **Multi-Architecture**: AMD64 and ARM64 container images available
- **Production Hardened**: Comprehensive testing, security scanning, and monitoring

## 🚀 Quick Start

### Using Custom Image (Recommended for Kubernetes)

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: fluent-bit
spec:
  template:
    spec:
      containers:
      - name: fluent-bit
        image: ghcr.io/rossigee/fluent-bit-amqp-plugin:latest
        volumeMounts:
        - name: config
          mountPath: /fluent-bit/etc

      volumes:
      - name: config
        configMap:
          name: fluent-bit-config
```

### Configuration

```ini
# CloudEvents format (default)
[OUTPUT]
    Name                amqp
    Match               *
    url                 amqp://user:pass@rabbitmq:5672/
    routing_key         application-logs
    event_source        my-application
    event_type          application.log

# Plain JSON format
[OUTPUT]
    Name                amqp
    Match               alerts.*
    url                 amqp://user:pass@rabbitmq:5672/
    routing_key         application-alerts
    cloudevents         false
```

## 📦 Installation

### Container Image

- **Custom Image**: `ghcr.io/rossigee/fluent-bit-amqp-plugin:latest`
  - Fluent Bit with the AMQP plugin pre-installed
  - Drop-in replacement for official Fluent Bit images

### Manual Installation

```bash
# Download latest release
wget https://github.com/rossigee/fluent-bit-amqp-plugin/releases/latest/download/out_amqp-linux-amd64.so

# Install to Fluent Bit plugins directory
sudo mkdir -p /usr/local/lib/fluent-bit
sudo cp out_amqp-linux-amd64.so /usr/local/lib/fluent-bit/

# Load plugin in Fluent Bit
fluent-bit -e /usr/local/lib/fluent-bit/out_amqp-linux-amd64.so -c fluent-bit.conf
```

### Kubernetes

A DaemonSet manifest is provided in `deploy/kubernetes/daemonset.yaml`. It runs the
official `fluent/fluent-bit` image and uses the
`ghcr.io/rossigee/fluent-bit-amqp-plugin-init` init container to inject the plugin
`.so` into a shared volume. Both images are published with each release
(`:latest` and `:<version>`).

### Building from Source

```bash
# Clone repository
git clone https://github.com/rossigee/fluent-bit-amqp-plugin.git
cd fluent-bit-amqp-plugin

# Build plugin
make build

# Build container images
make docker-build
```

> The plugin is a `glibc` shared object (`out_amqp.so`). The release build compiles it
> inside the Fluent Bit base image so its `glibc` matches the runtime and it loads
> correctly when Fluent Bit `dlopen`s it. A standalone static binary is not applicable
> to a Fluent Bit plugin.

## ⚙️ Configuration Reference

| Parameter | Description | Default | Required |
|-----------|-------------|---------|----------|
| `url` | AMQP connection URL | `amqp://localhost:5672/` | No |
| `exchange` | AMQP exchange name | `""` (default exchange) | No |
| `routing_key` | Routing key for messages | `fluent-bit-events` | No |
| `queue` | Queue name to declare/use | `fluent-bit-events` | No |
| `cloudevents` | Wrap records as CloudEvents | `true` | No |
| `event_source` | CloudEvent source field (CloudEvents only) | `fluent-bit` | No |
| `event_type` | CloudEvent type field (CloudEvents only) | `fluent-bit.log` | No |
| `durable` | Declare queue as durable | `true` | No |
| `queue_type` | Queue type: `classic`, `quorum` or `stream` | `classic` | No |
| `queue_mode` | Classic queue mode: `default` or `lazy` | `default` | No |
| `persistent` | Publish messages with delivery mode 2 (survive broker restart) | `true` | No |
| `expiration` | Per-message TTL in milliseconds (`0` disables) | `0` | No |
| `content_type` | AMQP `content_type` property | `application/json` | No |
| `message_type` | AMQP `type` property | `""` | No |
| `priority` | AMQP message priority, `0`–`255` | `0` | No |
| `app_id` | AMQP `app_id` property | `fluent-bit` | No |
| `tls` | Enable TLS/AMQPS | `false` | No |
| `tls_insecure_skip_verify` | Skip TLS certificate verification | `false` | No |
| `tls_ca_file` | Path to CA certificate file | `""` | No |
| `tls_cert_file` | Path to client certificate file | `""` | No |
| `tls_key_file` | Path to client key file | `""` | No |

### Queue types and message properties

- `queue_type classic` (default) and `queue_mode default` send no extra queue arguments, so existing queues are declared exactly as before.
- `quorum` and `stream` queues require `durable true`, `exclusive false` and `auto_delete false`. The plugin refuses to start with an invalid combination and logs the reason.
- `queue_mode lazy` applies to classic queues only.
- **Behaviour change in 5.3.0:** `persistent` defaults to `true`. Earlier versions published without a delivery mode, which RabbitMQ stores as transient. Set `persistent false` to restore the previous behaviour.
- `priority` only takes effect on priority queues. The plugin does not set `x-max-priority`, so declare the queue with that argument by other means if you need priorities.

### Advanced Configuration Example

```ini
[SERVICE]
    Plugins_File    /fluent-bit/etc/plugins.conf

[INPUT]
    Name            tail
    Path            /var/log/app/*.log
    Tag             app.*

[FILTER]
    Name            kubernetes
    Match           kube.*
    Kube_URL        https://kubernetes.default.svc:443

# Structured CloudEvents for event-driven consumers
[OUTPUT]
    Name                amqp
    Match               app.*
    url                 amqp://logger:secret@rabbitmq.infra.svc.cluster.local:5672/logs
    exchange            application-events
    routing_key         ${HOSTNAME}.app.logs
    queue               app-logs-${ENV}
    event_source        kubernetes.${CLUSTER_NAME}
    event_type          application.container.log
    durable             true

# Plain JSON for alert pipelines or simple consumers
[OUTPUT]
    Name                amqp
    Match               alerts.*
    url                 amqp://logger:secret@rabbitmq.infra.svc.cluster.local:5672/logs
    routing_key         ${HOSTNAME}.alerts
    queue               alerts
    cloudevents         false
    durable             true
```

### TLS/AMQPS Configuration

```ini
# AMQPS with custom certificates
[OUTPUT]
    Name                amqp
    Match               secure.*
    url                 amqps://logger:secret@rabbitmq.secure.svc.cluster.local:5671/
    routing_key         secure-events
    tls                 true
    tls_ca_file         /etc/fluent-bit/certs/ca.pem
    tls_cert_file       /etc/fluent-bit/certs/client.pem
    tls_key_file        /etc/fluent-bit/certs/client.key

# AMQPS with insecure verification (testing only)
[OUTPUT]
    Name                amqp
    Match               dev.*
    url                 amqps://logger:secret@rabbitmq-dev:5671/
    routing_key         dev-events
    tls                 true
    tls_insecure_skip_verify true
```

## 📥 Input Plugin (AMQP Source)

The input plugin consumes messages from a RabbitMQ queue and injects them into the Fluent Bit pipeline. It is built as a separate library, `in_amqp.so`, from `cmd/input`, and is registered as `amqp` for `[INPUT]` sections.

```ini
[INPUT]
    Name                amqp
    Tag                 amqp.events
    url                 amqp://user:pass@rabbitmq:5672/
    queue               my-queue
    exchange            my-exchange
    routing_key         #
    durable             true
    prefetch_count      10
    cloudevents_decode  true
```

| Key | Description | Default | Required |
|-----|-------------|---------|----------|
| `url`, `tls*` | Connection and TLS settings, as for the output | `amqp://localhost:5672/` | No |
| `queue` | Queue to consume from (declared if missing) | `fluent-bit-events` | Yes |
| `exchange` | If set, the queue is bound to this exchange | `""` | No |
| `routing_key` | Binding key used when `exchange` is set | `fluent-bit-events` | No |
| `durable` | Declare the queue as durable | `true` | No |
| `prefetch_count` | Unacknowledged messages the broker may send at once (must be at least 1) | `10` | No |
| `cloudevents_decode` | Use the `data` of a CloudEvent as the record | `false` | No |

Records are produced as follows:

- A JSON object body becomes the record as-is.
- With `cloudevents_decode true`, a CloudEvent (an object with `specversion`) becomes its `data` (an object is used directly; any other value is wrapped as `{"data": ...}`).
- Any other body is wrapped as `{"message": "<body>"}`.

Consumption is at-least-once. Messages are acknowledged after they have been handed to Fluent Bit. If the broker connection drops, the consumer reconnects with exponential backoff (1 s up to 30 s), and unacknowledged messages are redelivered. A crash in the window between handing a batch to Fluent Bit and the engine accepting it can lose that batch.

To load the input alongside the output:

```bash
fluent-bit -e /usr/local/lib/fluent-bit/in_amqp.so -e /usr/local/lib/fluent-bit/out_amqp.so -c fluent-bit.conf
```

The Fluent Bit image shipped by this repository loads both libraries through `plugins.conf`.

## 📡 MQTT Backend

The output plugin can publish to an MQTT broker (Mosquitto, EMQX, HiveMQ) instead of RabbitMQ. Set `backend mqtt`; the AMQP keys are then ignored. The message bodies are the same as for AMQP: plain JSON, or CloudEvents JSON with the attributes in the body (MQTT 3.1.1 has no message headers).

```ini
[OUTPUT]
    Name          amqp
    Match         *
    backend       mqtt
    broker        tcp://localhost:1883
    topic         fluent-bit/logs
    qos           1
    retain        false
    client_id     fluent-bit-node-01
    cloudevents   false
```

| Key | Description | Default | Required |
|-----|-------------|---------|----------|
| `backend` | Transport: `amqp` or `mqtt` | `amqp` | No |
| `broker` | Broker URL: `tcp://`, `ssl://`, `ws://` or `wss://` | `tcp://localhost:1883` | No |
| `topic` | Topic to publish to | `fluent-bit/logs` | No |
| `qos` | Quality of service: `0` (at most once), `1` (at least once) or `2` (exactly once) | `1` | No |
| `retain` | Set the retain flag on published messages | `false` | No |
| `client_id` | Client identifier; must be unique per broker | `fluent-bit` | No |
| `username`, `password` | Broker credentials | `""` | No |
| `keepalive` | Keepalive interval in seconds | `30` | No |
| `tls_ca_cert` | CA certificate for the broker (`ssl://` or `wss://` only) | `""` | No |
| `tls_client_cert`, `tls_client_key` | Client certificate and key for mutual TLS (set together) | `""` | No |
| `cloudevents`, `event_source`, `event_type` | As for AMQP | see above | No |

Server certificates are always verified for `ssl://` and `wss://` brokers. The publisher reconnects automatically if the connection drops; a publish that fails returns a retry to Fluent Bit.

## 📊 Message Formats

### CloudEvents format (default, `cloudevents true`)

```json
{
  "specversion": "1.0",
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "source": "fluent-bit",
  "type": "fluent-bit.log",
  "time": "2024-01-15T10:30:00Z",
  "datacontenttype": "application/json",
  "fluentbittag": "app.container",
  "data": {
    "level": "info",
    "message": "Application started successfully",
    "service": "web-api"
  }
}
```

AMQP headers include `ce-specversion`, `ce-type`, `ce-source`, and `ce-id`.

### Plain JSON format (`cloudevents false`)

```json
{
  "@timestamp": "2024-01-15T10:30:00Z",
  "tag": "app.container",
  "level": "info",
  "message": "Application started successfully",
  "service": "web-api"
}
```

Record fields are merged at the top level with `@timestamp` and `tag` alongside them.

## 🎯 Use Cases

### Application Logging

```ini
[OUTPUT]
    Name                amqp
    Match               app.*
    url                 amqp://app:secret@rabbitmq:5672/
    routing_key         application.${SERVICE_NAME}
    event_source        application.${SERVICE_NAME}
    event_type          application.log
```

### Security Event Stream

```ini
[OUTPUT]
    Name                amqp
    Match               security.*
    url                 amqp://security:secret@security-rabbitmq:5672/security
    exchange            security-events
    routing_key         audit.${SEVERITY}
    event_source        security.${CLUSTER_NAME}
    event_type          security.audit
```

### Alert Pipeline (plain JSON)

```ini
[OUTPUT]
    Name                amqp
    Match               alert.*
    url                 amqp://alerts:secret@rabbitmq:5672/
    routing_key         alerts
    queue               alerts
    cloudevents         false
    durable             true
```

## 🔧 Development

### Prerequisites

- Go 1.27.4+
- Docker
- RabbitMQ (for testing)

### Development Setup

```bash
# Setup development environment
make setup-dev

# Start test environment
make dev-test

# Run tests
make test

# Build and test
make dev
```

### Testing

```bash
# Unit tests
make test-unit

# Integration tests (requires RabbitMQ; tests skip when a broker is unavailable)
make test-integration

# Optional integration checks:
#   RABBITMQ_URL=amqp://guest:guest@localhost:5672/  broker used by the tests
#   RABBITMQ_MGMT_URL=http://localhost:15672         verify queue type and arguments via the management API
#   RABBITMQ_TEST_CONTAINER=<name>                   restart this Docker container to test persistence
#   RABBITMQ_TLS_URL, RABBITMQ_TLS_CA_FILE, RABBITMQ_TLS_CERT_FILE, RABBITMQ_TLS_KEY_FILE   TLS broker

# All tests with coverage
make test-coverage
```

## 🐛 Troubleshooting

### Plugin Not Loading

```bash
# Check plugin file permissions
ls -la /fluent-bit/plugins/out_amqp.so

# Enable debug logging
[SERVICE]
    Log_Level debug
```

### Connection Issues

```bash
# Test AMQP connectivity
telnet rabbitmq 5672

# Check RabbitMQ logs
kubectl logs -n rabbitmq rabbitmq-0
```

### Performance Tuning

```ini
# Adjust flush interval
[SERVICE]
    Flush 5

# Enable buffering
[OUTPUT]
    Buffer_Max_Size     256k
```

## 🤝 Contributing

We welcome contributions!

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Make your changes and add tests
4. Run tests: `make test`
5. Commit your changes: `git commit -m 'Add amazing feature'`
6. Push to the branch: `git push origin feature/amazing-feature`
7. Open a Pull Request

## 📄 License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [Fluent Bit](https://fluentbit.io/) - Fast and lightweight log processor
- [CloudEvents](https://cloudevents.io/) - Event specification for interoperability
- [RabbitMQ](https://www.rabbitmq.com/) - Open source message broker
- [CloudEvents Go SDK](https://github.com/cloudevents/sdk-go) - Go implementation of CloudEvents

## 📈 Status

- ✅ **Stable**: Core plugin functionality
- ✅ **Production Ready**: Container images and CI/CD
- ✅ **Well Tested**: Comprehensive test suite
- ✅ **Documented**: Complete documentation
- 🔄 **Active Development**: Regular updates and improvements

---

**Made with ❤️ by [Ross Golder](https://github.com/rossigee)**
