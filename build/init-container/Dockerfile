# Fluent Bit AMQP Plugin Init Container
# This container copies the plugin .so file to a shared emptyDir volume

FROM alpine:3.19

# Install minimal required tools
RUN apk add --no-cache coreutils

# Copy the plugin file and entrypoint script
COPY out_amqp_cloudevents.so /usr/lib/fluent-bit/out_amqp_cloudevents.so
COPY build/init-container/entrypoint.sh /entrypoint.sh

# Set up environment
ENV PLUGIN_NAME=out_amqp_cloudevents.so
ENV PLUGINS_DIR=/plugins

# Make entrypoint executable
RUN chmod +x /entrypoint.sh

# Create non-root user for security
RUN addgroup -g 1001 fluent && \
    adduser -D -u 1001 -G fluent fluent

# Change ownership of plugin file
RUN chown -R fluent:fluent /usr/lib/fluent-bit/

# Switch to non-root user
USER fluent

# Set entrypoint
ENTRYPOINT ["/entrypoint.sh"]

# Labels for better image management
LABEL org.opencontainers.image.title="Fluent Bit AMQP Plugin Init Container"
LABEL org.opencontainers.image.description="Init container that copies the Fluent Bit AMQP CloudEvents plugin to a shared volume"
LABEL org.opencontainers.image.vendor="Ross Golder"
LABEL org.opencontainers.image.source="https://github.com/rossigee/fluent-bit-amqp-plugin"