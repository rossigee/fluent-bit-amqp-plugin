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

## ⚙️ Configuration Reference

| Parameter | Description | Default | Required |
|-----------|-------------|---------|----------|
| `url` | AMQP connection URL | `amqp://guest:guest@localhost:5672/` | No |
| `exchange` | AMQP exchange name | `""` (default exchange) | No |
| `routing_key` | Routing key for messages | `fluent-bit-events` | No |
| `queue` | Queue name to declare/use | `fluent-bit-events` | No |
| `cloudevents` | Wrap records as CloudEvents | `true` | No |
| `event_source` | CloudEvent source field (CloudEvents only) | `fluent-bit` | No |
| `event_type` | CloudEvent type field (CloudEvents only) | `fluent-bit.log` | No |
| `durable` | Declare queue as durable | `true` | No |
| `tls` | Enable TLS/AMQPS | `false` | No |
| `tls_insecure_skip_verify` | Skip TLS certificate verification | `false` | No |
| `tls_ca_file` | Path to CA certificate file | `""` | No |
| `tls_cert_file` | Path to client certificate file | `""` | No |
| `tls_key_file` | Path to client key file | `""` | No |

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

- Go 1.25.1+
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

# Integration tests (requires RabbitMQ)
make test-integration

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
