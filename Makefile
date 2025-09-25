# Fluent Bit AMQP CloudEvents Output Plugin Makefile

PLUGIN_NAME = out_amqp_cloudevents
MODULE = github.com/rossigee/fluent-bit-amqp-plugin
VERSION ?= $(shell cat VERSION)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

GO_VERSION = 1.25.1
CGO_ENABLED = 1
GOOS ?= linux
GOARCH ?= amd64

# Build flags
LDFLAGS = -w -s -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
BUILD_TAGS = netgo

# Docker settings
REGISTRY ?= ghcr.io
IMAGE_NAME ?= rossigee/fluent-bit-amqp-plugin
INIT_IMAGE_NAME ?= rossigee/fluent-bit-amqp-plugin-init

.PHONY: help build build-all build-plugin build-init-container clean test test-unit test-integration \
        fmt vet lint security-scan deps mod-tidy install docker-build docker-push \
        release-local release-github setup-dev info validate

# Default target
all: build

## help: Show this help message
help:
	@echo "Available targets:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

## setup-dev: Setup development environment
setup-dev:
	@echo "Setting up development environment..."
	go install golang.org/x/tools/gopls@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/securecodewarrior/sast-scan/cmd/gosec@latest
	curl -sSfL https://raw.githubusercontent.com/securecodewarrior/sast-scan/main/install.sh | sh -s -- -b $(shell go env GOPATH)/bin
	@echo "Development tools installed"

## deps: Download and tidy dependencies
deps:
	go mod download
	go mod tidy

## mod-tidy: Tidy go modules
mod-tidy:
	go mod tidy

## fmt: Format Go code
fmt:
	go fmt ./...

## vet: Run go vet
vet:
	go vet ./...

## lint: Run golangci-lint
lint:
	golangci-lint run --timeout 5m

## security-scan: Run security scanning with gosec
security-scan:
	gosec -fmt json -out gosec-report.json -stdout ./...

## test: Run all tests
test: test-unit test-integration

## test-unit: Run unit tests
test-unit:
	@if ls ./pkg/**/*_test.go >/dev/null 2>&1; then \
		go test -v -race -cover ./pkg/...; \
	else \
		echo "No unit tests found in ./pkg/"; \
		echo "Unit test coverage: 0.0% (no tests)"; \
	fi

## test-integration: Run integration tests (requires RabbitMQ)
test-integration:
	@if ls ./test/integration/*_test.go >/dev/null 2>&1; then \
		go test -v -tags integration ./test/integration/...; \
	else \
		echo "No integration tests found in ./test/integration/"; \
	fi

## test-coverage: Run tests with coverage report
test-coverage:
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out

## build: Build all targets
build: build-plugin

## build-all: Build plugin for multiple architectures
build-all: build-plugin-linux-amd64 build-plugin-linux-arm64-optional

## build-plugin: Build the plugin shared library
build-plugin: deps fmt vet
	@echo "Building $(PLUGIN_NAME).so..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -buildmode=c-shared \
		-ldflags="$(LDFLAGS)" \
		-tags="$(BUILD_TAGS)" \
		-o $(PLUGIN_NAME).so \
		./cmd/plugin
	@echo "Plugin built successfully: $(PLUGIN_NAME).so"

## build-plugin-linux-amd64: Build plugin for Linux AMD64
build-plugin-linux-amd64:
	@$(MAKE) build-plugin GOOS=linux GOARCH=amd64
	@cp $(PLUGIN_NAME).so $(PLUGIN_NAME)-linux-amd64.so

## build-plugin-linux-arm64: Build plugin for Linux ARM64
build-plugin-linux-arm64:
	@$(MAKE) build-plugin GOOS=linux GOARCH=arm64
	@mv $(PLUGIN_NAME).so $(PLUGIN_NAME)-linux-arm64.so

## build-plugin-linux-arm64-optional: Build plugin for Linux ARM64 (optional)
build-plugin-linux-arm64-optional:
	@echo "Attempting ARM64 build..."
	@if $(MAKE) build-plugin GOOS=linux GOARCH=arm64 2>/dev/null; then \
		cp $(PLUGIN_NAME).so $(PLUGIN_NAME)-linux-arm64.so; \
		echo "ARM64 build successful"; \
	else \
		echo "ARM64 build failed (fluent-bit-go constraints) - skipping"; \
	fi

## build-static: Build statically linked plugin
build-static: deps
	@echo "Building static $(PLUGIN_NAME).so..."
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -buildmode=c-shared \
		-ldflags="$(LDFLAGS) -extldflags '-static'" \
		-tags="$(BUILD_TAGS) static_build" \
		-o $(PLUGIN_NAME).so \
		./cmd/plugin
	@echo "Static plugin built successfully: $(PLUGIN_NAME).so"

## build-init-container: Build init container image
build-init-container:
	@echo "Building init container image..."
	docker build -f build/init-container/Dockerfile -t $(REGISTRY)/$(INIT_IMAGE_NAME):$(VERSION) .
	docker tag $(REGISTRY)/$(INIT_IMAGE_NAME):$(VERSION) $(REGISTRY)/$(INIT_IMAGE_NAME):latest
	@echo "Init container built: $(REGISTRY)/$(INIT_IMAGE_NAME):$(VERSION)"

## docker-build: Build Docker images
docker-build: build-plugin
	@echo "Building Docker images..."
	docker build -f build/Dockerfile -t $(REGISTRY)/$(IMAGE_NAME):$(VERSION) .
	docker tag $(REGISTRY)/$(IMAGE_NAME):$(VERSION) $(REGISTRY)/$(IMAGE_NAME):latest
	@$(MAKE) build-init-container

## docker-push: Push Docker images to registry
docker-push:
	@echo "Pushing Docker images..."
	docker push $(REGISTRY)/$(IMAGE_NAME):$(VERSION)
	docker push $(REGISTRY)/$(IMAGE_NAME):latest
	docker push $(REGISTRY)/$(INIT_IMAGE_NAME):$(VERSION)
	docker push $(REGISTRY)/$(INIT_IMAGE_NAME):latest

## clean: Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f $(PLUGIN_NAME)*.so
	rm -f $(PLUGIN_NAME)*.h
	rm -f *.test
	rm -f coverage.out coverage.html
	rm -f gosec-report.json
	rm -rf dist/
	docker rmi -f $(REGISTRY)/$(IMAGE_NAME):$(VERSION) $(REGISTRY)/$(IMAGE_NAME):latest || true
	docker rmi -f $(REGISTRY)/$(INIT_IMAGE_NAME):$(VERSION) $(REGISTRY)/$(INIT_IMAGE_NAME):latest || true
	@echo "Clean complete"

## install: Install plugin to system directory (requires sudo)
install: build-plugin
	@echo "Installing plugin to /usr/local/lib/fluent-bit/..."
	sudo mkdir -p /usr/local/lib/fluent-bit
	sudo cp $(PLUGIN_NAME).so /usr/local/lib/fluent-bit/
	@echo "Plugin installed"

## validate: Validate code quality and security
validate: fmt vet lint security-scan test

## info: Display build information
info:
	@echo "Plugin: $(PLUGIN_NAME)"
	@echo "Module: $(MODULE)"
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Date: $(DATE)"
	@echo "Go Version: $(GO_VERSION)"
	@echo "CGO Enabled: $(CGO_ENABLED)"
	@echo "Target OS/Arch: $(GOOS)/$(GOARCH)"
	@if [ -f $(PLUGIN_NAME).so ]; then \
		echo "Plugin Size: $$(du -h $(PLUGIN_NAME).so | cut -f1)"; \
		echo "Plugin Built: $$(stat -c %y $(PLUGIN_NAME).so 2>/dev/null || stat -f %Sm $(PLUGIN_NAME).so 2>/dev/null)"; \
	else \
		echo "Plugin not built yet. Run 'make build'"; \
	fi

## release-local: Create local release artifacts
release-local: clean build-all
	@echo "Creating release artifacts..."
	mkdir -p dist/binaries
	cp $(PLUGIN_NAME)-*.so dist/binaries/
	cd dist/binaries && sha256sum *.so > checksums.txt
	@echo "Release artifacts created in dist/"

## release-github: Create GitHub release (requires gh CLI)
release-github: release-local
	@echo "Creating GitHub release..."
	gh release create v$(VERSION) \
		--title "Release v$(VERSION)" \
		--generate-notes \
		dist/binaries/*

# Development helpers
.PHONY: dev dev-test dev-rabbitmq dev-clean

## dev: Quick development build and test
dev: clean build test

## dev-test: Start test environment with RabbitMQ
dev-test:
	@echo "Starting development test environment..."
	docker-compose -f deploy/docker-compose.yml up -d rabbitmq
	@echo "RabbitMQ available at http://localhost:15672 (guest/guest)"

## dev-clean: Clean development environment
dev-clean:
	docker-compose -f deploy/docker-compose.yml down -v

# Version management
.PHONY: version-patch version-minor version-major

## version-patch: Increment patch version
version-patch:
	@current=$$(cat VERSION); \
	new=$$(echo $$current | awk -F. '{$$NF = $$NF + 1;} 1' | sed 's/ /./g'); \
	echo $$new > VERSION; \
	echo "Version updated: $$current -> $$new"

## version-minor: Increment minor version
version-minor:
	@current=$$(cat VERSION); \
	new=$$(echo $$current | awk -F. '{$$(NF-1) = $$(NF-1) + 1; $$NF = 0;} 1' | sed 's/ /./g'); \
	echo $$new > VERSION; \
	echo "Version updated: $$current -> $$new"

## version-major: Increment major version
version-major:
	@current=$$(cat VERSION); \
	new=$$(echo $$current | awk -F. '{$$1 = $$1 + 1; $$(NF-1) = 0; $$NF = 0;} 1' | sed 's/ /./g'); \
	echo $$new > VERSION; \
	echo "Version updated: $$current -> $$new"