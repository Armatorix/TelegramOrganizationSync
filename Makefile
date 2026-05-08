.DEFAULT_GOAL := help

GO              ?= go
BIN_DIR         ?= bin
DEVSERVER_BIN   := $(BIN_DIR)/devserver
TOS_BIN         := $(BIN_DIR)/tos

DEVSERVER_ADDR  ?= :8080
DEV_API_KEY     ?= dev-api-key
CLIENT_CONFIG   ?= config.yaml

.PHONY: help build setup dev devserver client tidy vet fmt lint test ci clean

help: ## list targets
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## compile both binaries to ./bin/
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(DEVSERVER_BIN) ./cmd/devserver
	$(GO) build -o $(TOS_BIN) ./cmd/tos

setup: config.yaml fake-telegram.json ## seed local config files from examples (idempotent)

config.yaml:
	cp config.example.yaml config.yaml
	@echo "wrote config.yaml"

fake-telegram.json:
	cp fake-telegram.example.json fake-telegram.json
	@echo "wrote fake-telegram.json"

dev: build setup ## run dev server + client side by side; ctrl-c stops both
	@echo "==> dev server: http://localhost$(DEVSERVER_ADDR)  api-key: $(DEV_API_KEY)"
	@trap 'kill 0' INT TERM EXIT; \
		( $(DEVSERVER_BIN) -addr $(DEVSERVER_ADDR) -api-key $(DEV_API_KEY) 2>&1 | sed -u 's/^/[srv] /' ) & \
		sleep 0.3; \
		( $(TOS_BIN) -config $(CLIENT_CONFIG) 2>&1 | sed -u 's/^/[cli] /' ) & \
		wait

devserver: build setup ## run only the dev server (foreground)
	$(DEVSERVER_BIN) -addr $(DEVSERVER_ADDR) -api-key $(DEV_API_KEY)

client: build setup ## run only the sync client (foreground)
	$(TOS_BIN) -config $(CLIENT_CONFIG)

tidy: ## go mod tidy
	$(GO) mod tidy

vet: ## go vet ./...
	$(GO) vet ./...

fmt: ## go fmt ./...
	$(GO) fmt ./...

test: ## go test -race ./...
	$(GO) test -race ./...

lint: ## golangci-lint run (requires golangci-lint installed)
	golangci-lint run

ci: vet test ## run the same checks CI runs (build, vet, test for both build-tag variants)
	$(GO) build ./...
	$(GO) build -tags tdlib ./...

clean: ## remove build artifacts (keeps your config.yaml / fake-telegram.json)
	rm -rf $(BIN_DIR)
