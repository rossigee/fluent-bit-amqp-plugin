# Init container — carries the built plugin .so and drops it into the shared
# volume (PLUGINS_DIR) that the Fluent Bit DaemonSet pod mounts. The .so is just
# copied as a file here; this image never dlopen()s it, so its own libc is
# irrelevant (kept tiny with Alpine).
FROM alpine:3.21

COPY out_amqp.so /out_amqp.so
COPY build/init-container/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
