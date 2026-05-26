# Custom Fluent Bit image with AMQP output plugin
FROM fluent/fluent-bit:5.0.6

# Copy the plugin file with correct permissions
COPY --chmod=755 out_amqp.so /fluent-bit/plugins/out_amqp.so

# Copy plugins configuration file
COPY build/fluent-bit/plugins.conf /fluent-bit/plugins/plugins.conf

# Labels for better image management
LABEL org.opencontainers.image.title="Fluent Bit with AMQP Plugin"
LABEL org.opencontainers.image.description="Custom Fluent Bit image that includes the AMQP output plugin (plain JSON or CloudEvents)"
LABEL org.opencontainers.image.vendor="Ross Golder"
LABEL org.opencontainers.image.source="https://github.com/rossigee/fluent-bit-amqp-plugin"
