# Plugin builder — compiles the .so against Debian bookworm glibc so it loads
# correctly when Fluent Bit (a glibc binary) dlopen()s it. A musl/fully-static
# .so segfaults inside the glibc Fluent Bit process, so we deliberately build a
# glibc-dynamic shared object here.
FROM debian:bookworm

ARG GO_VERSION=1.27.1
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT=""
ARG DATE=""

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates curl gcc libc6-dev && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${TARGETARCH}.tar.gz" | \
        tar -C /usr/local -xz && \
    rm -rf /var/lib/apt/lists/*

ENV PATH=/usr/local/go/bin:$PATH
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOARCH=${TARGETARCH} go build -buildmode=c-shared -tags netgo \
        -ldflags="-w -s -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
        -o /out_amqp.so ./cmd/plugin
