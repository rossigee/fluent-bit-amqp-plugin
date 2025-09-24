# Multi-stage Dockerfile for Fluent Bit AMQP CloudEvents Plugin

# Build stage
FROM golang:1.25.1-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev git

# Set working directory
WORKDIR /build

# Copy Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the plugin
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

RUN CGO_ENABLED=1 go build -buildmode=c-shared \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o out_amqp_cloudevents.so \
    ./cmd/plugin

# Runtime stage - based on Fluent Bit official image
FROM fluent/fluent-bit:3.2

# Switch to root to install plugin
USER root

# Install plugin
COPY --from=builder /build/out_amqp_cloudevents.so /fluent-bit/bin/

# Create plugins configuration
RUN echo "[PLUGINS]" > /fluent-bit/etc/plugins.conf && \
    echo "    Path /fluent-bit/bin/out_amqp_cloudevents.so" >> /fluent-bit/etc/plugins.conf

# Ensure proper permissions
RUN chmod 755 /fluent-bit/bin/out_amqp_cloudevents.so

# Switch back to fluent-bit user
USER fluent-bit

# Expose Fluent Bit port
EXPOSE 24224

# Labels
LABEL org.opencontainers.image.title="Fluent Bit with AMQP CloudEvents Plugin"
LABEL org.opencontainers.image.description="Fluent Bit container with AMQP CloudEvents output plugin pre-installed"
LABEL org.opencontainers.image.vendor="Ross Golder"
LABEL org.opencontainers.image.source="https://github.com/rossigee/fluent-bit-amqp-plugin"

# Default command
CMD ["/fluent-bit/bin/fluent-bit", "--config=/fluent-bit/etc/fluent-bit.conf"]