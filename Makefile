.PHONY: build-sandbox
build-sandbox:
	docker build -t sandbox-python:latest ./pkg/sandbox/docker/image

.PHONY: generate
generate:
	buf generate pkg/sandbox/api


.PHONY: test
test:
	go test -v -skip TestIntegration ./...

.PHONY: test-integration
test-integration:
	# Run integration tests across all packages.
	# We rely on 'source .env' to load vars.
	sh -c 'if [ -f .env ]; then set -a; source .env; set +a; fi; go test -v -run TestIntegration ./...'

.PHONY: dev
dev:
	# Run Go server and Vite dev server concurrently with cleanup
	# Load .env if present
	@if [ -f .env ]; then set -a; source .env; set +a; fi; \
	trap 'kill 0' EXIT; \
	(cd web && npm install && npm run dev) & \
	go run cmd/cli/main.go & \
	wait

.PHONY: test-e2e
test-e2e:
	# Ensure deps and browsers are installed
	cd web && npm install && npx playwright install chromium
	# Run tests
	@if [ -f .env ]; then set -a; source .env; set +a; fi; \
	trap 'kill 0' EXIT; \
	go run cmd/cli/main.go & \
	sleep 5; \
	(cd web && npx playwright test --reporter=list)

.PHONY: build
build:
	# Build frontend
	cd web && npm install && npm run build
	# Build backend
	go build -o bin/gemini cmd/cli/main.go

.PHONY: install-deps
install-deps:
	go mod download
	cd web && npm install
