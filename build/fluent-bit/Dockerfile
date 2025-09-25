# Custom Fluent Bit image with AMQP CloudEvents plugin
FROM fluent/fluent-bit:4.1.0

# Build argument for target architecture
ARG TARGETARCH

# Copy the architecture-specific plugin file with correct permissions
COPY --chmod=755 out_amqp_cloudevents-linux-${TARGETARCH}.so /usr/lib/fluent-bit/out_amqp_cloudevents.so

# Labels for better image management
LABEL org.opencontainers.image.title="Fluent Bit with AMQP CloudEvents Plugin"
LABEL org.opencontainers.image.description="Custom Fluent Bit image that includes the AMQP CloudEvents output plugin"
LABEL org.opencontainers.image.vendor="Ross Golder"
LABEL org.opencontainers.image.source="https://github.com/rossigee/fluent-bit-amqp-plugin"