# Custom Fluent Bit image with AMQP CloudEvents plugin
FROM ghcr.io/fluent/fluent-operator/fluent-bit:4.0.1

# Build argument for target architecture
ARG TARGETARCH

# Copy the architecture-specific plugin file
COPY out_amqp_cloudevents-linux-${TARGETARCH}.so /usr/lib/fluent-bit/out_amqp_cloudevents.so

# Set proper permissions
USER root
RUN chmod 755 /usr/lib/fluent-bit/out_amqp_cloudevents.so

# Switch back to fluent-bit user
USER fluent-bit

# Labels for better image management
LABEL org.opencontainers.image.title="Fluent Bit with AMQP CloudEvents Plugin"
LABEL org.opencontainers.image.description="Custom Fluent Bit image that includes the AMQP CloudEvents output plugin"
LABEL org.opencontainers.image.vendor="Ross Golder"
LABEL org.opencontainers.image.source="https://github.com/rossigee/fluent-bit-amqp-plugin"