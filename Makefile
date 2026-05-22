.PHONY: build test lint vet clean run docker-build docker-up docker-down help

APP_NAME := mcp-nexus
BUILD_DIR := /tmp

# Go parameters
GO := go
GOFLAGS := -ldflags="-s -w"

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the gateway binary
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(APP_NAME) ./cmd/server/

build-all: ## Build all binaries (gateway + mock backends + seed + client)
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(APP_NAME) ./cmd/server/ &
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/mock-backend ./dev/mock-backend/ &
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/mock-grpc-backend ./dev/mock-grpc-backend/ &
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/seed ./dev/seed/ &
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/mock-client ./dev/mock-client/ &
	wait

test: ## Run all unit tests
	$(GO) test ./... -count=1

test-cover: ## Run tests with coverage
	$(GO) test ./... -count=1 -coverprofile=coverage.out
	$(GO) tool cover -html=coverage.out -o coverage.html

lint: ## Run go vet
	$(GO) vet ./...

vet: lint ## Alias for lint

fmt: ## Format code
	$(GO) fmt ./...

tidy: ## Tidy go modules
	$(GO) mod tidy

clean: ## Clean build artifacts and test databases
	rm -f $(BUILD_DIR)/$(APP_NAME) $(BUILD_DIR)/mock-* $(BUILD_DIR)/seed
	rm -f gateway.db coverage.out coverage.html

run: ## Run gateway locally (requires mock backends)
	$(GO) run ./cmd/server/

run-all: ## Run E2E startup script
	bash scripts/run-all.sh

test-all: ## Run full integration test suite
	bash scripts/test-all.sh

docker-build: ## Build Docker image
	docker build -t $(APP_NAME):latest .

docker-build-all: ## Build all Docker images
	docker build -t $(APP_NAME):latest .
	docker build -t mcp-mock-backend -f Dockerfile.mock .
	docker build -t mcp-mock-grpc -f Dockerfile.mock-grpc .

docker-up: ## Start full stack with docker compose
	docker compose up -d

docker-down: ## Stop docker compose stack
	docker compose down

docker-logs: ## View docker compose logs
	docker compose logs -f

seed: ## Run database seed (requires running gateway)
	$(GO) run ./dev/seed/
