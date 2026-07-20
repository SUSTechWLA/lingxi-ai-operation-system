.PHONY: backend-build backend-run backend-test build run test clean tidy migrate docker-up docker-down frontend-install frontend-dev frontend-build gen-ts api-types-check

APP_NAME := tangying-ai-os
BUILD_DIR := build
MAIN_PKG := ./cmd/tangying-ai-os

backend-build:
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PKG)
	@echo "Build complete: $(BUILD_DIR)/$(APP_NAME)"

backend-run: backend-build
	./$(BUILD_DIR)/$(APP_NAME)

backend-test:
	go test ./... -v -count=1

build: backend-build

run: backend-run

test: backend-test

tidy:
	go mod tidy

clean:
	rm -rf $(BUILD_DIR)
	go clean

migrate:
	@echo "Running migrations..."
	go run $(MAIN_PKG) -migrate

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
	cd ../frontend && npm install

frontend-dev:
	cd ../frontend && npm run dev

frontend-build:
	cd ../frontend && npm run build

gen-ts:
	go run ./cmd/gen-ts

api-types-check:
	go run ./cmd/gen-ts -check

sandbox-build:
	@echo "Building Rust sandbox..."
	. $$HOME/.cargo/env && cd sandbox && cargo build --release
	cp sandbox/target/release/tangying-sandbox build/
	@echo "Sandbox build complete: build/tangying-sandbox"

sandbox-run:
	. $$HOME/.cargo/env && cd sandbox && cargo run

sandbox-clean:
	cd sandbox && cargo clean

frontend-lint:
	cd ../frontend && npm run lint
