.PHONY: build run test clean tidy migrate docker-up docker-down frontend-install frontend-dev frontend-build

APP_NAME := lingxi-ai-os
BUILD_DIR := build
MAIN := cmd/lingxi-ai-os/main.go

build:
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) $(MAIN)
	@echo "Build complete: $(BUILD_DIR)/$(APP_NAME)"

run: build
	./$(BUILD_DIR)/$(APP_NAME)

test:
	go test ./... -v -count=1

tidy:
	go mod tidy

clean:
	rm -rf $(BUILD_DIR)
	go clean

migrate:
	@echo "Running migrations..."
	go run $(MAIN) -migrate

docker-up:
	docker compose up -d

docker-down:
	docker compose down

lint:
	golangci-lint run

fmt:
	gofmt -w .
	goimports -w .

frontend-install:
	cd frontend && npm install

frontend-dev:
	cd frontend && npm run dev

frontend-build:
	cd frontend && npm run build

sandbox-build:
	@echo "Building Rust sandbox..."
	. $$HOME/.cargo/env && cd sandbox && cargo build --release
	cp sandbox/target/release/lingxi-sandbox build/
	@echo "Sandbox build complete: build/lingxi-sandbox"

sandbox-run:
	. $$HOME/.cargo/env && cd sandbox && cargo run

sandbox-clean:
	cd sandbox && cargo clean

frontend-lint:
	cd frontend && npm run lint

frontend-test:
	cd frontend && npm run test
