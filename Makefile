.PHONY: fmt vet test race lint build check test-integration

APP_NAME := owot-server
BUILD_DIR := bin
GOLANGCI_LINT_VERSION := v2.4.0
GOPATH_BIN := $(shell go env GOPATH)/bin
GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null || (test -x "$(GOPATH_BIN)/golangci-lint" && echo "$(GOPATH_BIN)/golangci-lint"))

fmt:
	gofmt -s -w .

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

lint:
	@if [ -z "$(GOLANGCI_LINT)" ]; then \
		echo "ERROR: golangci-lint ($(GOLANGCI_LINT_VERSION)) is required but not installed." >&2; \
		echo "Install with: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)" >&2; \
		exit 1; \
	fi
	$(GOLANGCI_LINT) run ./...

build:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) ./cmd/api

check: fmt vet test race lint build

test-integration:
	OWOT_RUN_INTEGRATION=1 go test -v -race ./tests/integration/...
