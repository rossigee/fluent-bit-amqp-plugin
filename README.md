# Fluent Bit AMQP CloudEvents Output Plugin

[![CI](https://github.com/rossigee/fluent-bit-amqp-plugin/actions/workflows/ci.yml/badge.svg)](https://github.com/rossigee/fluent-bit-amqp-plugin/actions/workflows/ci.yml)
[![Release](https://github.com/rossigee/fluent-bit-amqp-plugin/actions/workflows/release.yml/badge.svg)](https://github.com/rossigee/fluent-bit-amqp-plugin/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/rossigee/fluent-bit-amqp-plugin)](https://goreportcard.com/report/github.com/rossigee/fluent-bit-amqp-plugin)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

A production-ready Fluent Bit output plugin that sends log events to AMQP queues (RabbitMQ) wrapped as [CloudEvents](https://cloudevents.io/). Perfect for building event-driven architectures with standardized event formats.

## ✨ Features

- **CloudEvents v1.0 Compliance**: All events formatted according to CloudEvents specification
- **AMQP 0.9.1 Support**: Direct publishing to RabbitMQ/AMQP brokers
- **Automatic Reconnection**: Handles connection failures gracefully with retry logic
- **Kubernetes Ready**: Init container pattern for seamless plugin deployment
- **Multi-Architecture**: AMD64 and ARM64 container images available
- **Production Hardened**: Comprehensive testing, security scanning, and monitoring

## 🚀 Quick Start

### Using Init Container (Recommended for Kubernetes)

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: fluent-bit
spec:
  template:
    spec:
      initContainers:
      - name: plugin-installer
        image: ghcr.io/rossigee/fluent-bit-amqp-plugin-init:latest
        volumeMounts:
        - name: plugins
          mountPath: /plugins

      containers:
      - name: fluent-bit
        image: fluent/fluent-bit:3.2
        volumeMounts:
        - name: plugins
          mountPath: /fluent-bit/plugins
        - name: config
          mountPath: /fluent-bit/etc

      volumes:
      - name: plugins
        emptyDir: {}
      - name: config
        configMap:
          name: fluent-bit-config
```

### Configuration

```ini
[OUTPUT]
    Name                amqp_cloudevents
    Match               *
    url                 amqp://user:pass@rabbitmq:5672/
    routing_key         application-logs
    event_source        my-application
    event_type          application.log
```

## 📦 Installation

### Container Images

- **Init Container**: `ghcr.io/rossigee/fluent-bit-amqp-plugin-init:latest`
- **Full Image**: `ghcr.io/rossigee/fluent-bit-amqp-plugin:latest`

### Manual Installation

```bash
# Download latest release
wget https://github.com/rossigee/fluent-bit-amqp-plugin/releases/latest/download/out_amqp_cloudevents-linux-amd64.so

# Install to Fluent Bit plugins directory
sudo mkdir -p /usr/local/lib/fluent-bit
sudo cp out_amqp_cloudevents-linux-amd64.so /usr/local/lib/fluent-bit/

# Load plugin in Fluent Bit
fluent-bit -e /usr/local/lib/fluent-bit/out_amqp_cloudevents-linux-amd64.so -c fluent-bit.conf
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
| `event_source` | CloudEvent source field | `fluent-bit` | No |
| `event_type` | CloudEvent type field | `fluent-bit.log` | No |
| `durable` | Declare queue as durable | `true` | No |

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

[OUTPUT]
    Name                amqp_cloudevents
    Match               app.*
    url                 amqp://logger:secret@rabbitmq.infra.svc.cluster.local:5672/logs
    exchange            application-events
    routing_key         ${HOSTNAME}.app.logs
    queue               app-logs-${ENV}
    event_source        kubernetes.${CLUSTER_NAME}
    event_type          application.container.log
    durable             true
```

## 📊 CloudEvent Format

Events are published as CloudEvents with the following structure:

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
    "timestamp": "2024-01-15T10:30:00Z",
    "level": "info",
    "message": "Application started successfully",
    "service": "web-api",
    "kubernetes": {
      "namespace": "production",
      "pod_name": "web-api-7d4b8c9f5-x2j8k"
    }
  }
}
```

### AMQP Headers

CloudEvent metadata is also included as AMQP headers:

- `ce-specversion`: CloudEvent specification version
- `ce-type`: CloudEvent type
- `ce-source`: CloudEvent source
- `ce-id`: CloudEvent ID

## 🎯 Use Cases

### Application Logging

```ini
[OUTPUT]
    Name                amqp_cloudevents
    Match               app.*
    url                 amqp://app:secret@rabbitmq:5672/
    routing_key         application.${SERVICE_NAME}
    event_source        application.${SERVICE_NAME}
    event_type          application.log
```

### Security Event Stream

```ini
[OUTPUT]
    Name                amqp_cloudevents
    Match               security.*
    url                 amqp://security:secret@security-rabbitmq:5672/security
    exchange            security-events
    routing_key         audit.${SEVERITY}
    event_source        security.${CLUSTER_NAME}
    event_type          security.audit
```

### Multi-Tenant Logging

```ini
[OUTPUT]
    Name                amqp_cloudevents
    Match               tenant.*
    url                 amqp://tenant:secret@rabbitmq:5672/
    routing_key         tenant.${TENANT_ID}.logs
    queue               tenant-${TENANT_ID}-logs
    event_source        tenant.${TENANT_ID}
    event_type          tenant.application.log
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

# Install pre-commit hooks
pre-commit install

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

## 📚 Documentation

- [Installation Guide](docs/installation.md)
- [Configuration Reference](docs/configuration.md)
- [Kubernetes Integration](docs/kubernetes.md)
- [Development Guide](docs/development.md)
- [Contributing Guidelines](CONTRIBUTING.md)

## 🐛 Troubleshooting

### Plugin Not Loading

```bash
# Check plugin file permissions
ls -la /fluent-bit/plugins/out_amqp_cloudevents.so

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

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

### Quick Contribute

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