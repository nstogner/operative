.PHONY: build-sandbox
build-sandbox:
	docker build -t sandbox-python:latest ./pkg/sandbox/docker/image

.PHONY: generate
generate:
	buf generate pkg/sandbox/api

.PHONY: test
test:
	CGO_ENABLED=1 go test -v -skip TestIntegration ./...

.PHONY: test-integration
test-integration:
	sh -c 'if [ -f .env ]; then set -a; source .env; set +a; fi; CGO_ENABLED=1 go test -v -run TestIntegration ./...'

.PHONY: dev
dev:
	@if lsof -i :8080 -sTCP:LISTEN -t >/dev/null; then echo "Error: Port 8080 is already in use."; exit 1; fi
	@if lsof -i :5173 -sTCP:LISTEN -t >/dev/null; then echo "Error: Port 5173 is already in use."; exit 1; fi
	@if [ -f .env ]; then set -a; source .env; set +a; fi; \
	trap 'kill 0' EXIT; \
	(cd web && npm install && npm run dev) & \
	CGO_ENABLED=1 go run cmd/operative/main.go & \
	wait

.PHONY: test-e2e
test-e2e:
	cd web && npm install && npx playwright install chromium
	@if [ -f .env ]; then set -a; source .env; set +a; fi; \
	trap 'kill 0' EXIT; \
	CGO_ENABLED=1 go run cmd/operative/main.go & \
	sleep 5; \
	(cd web && npx playwright test --reporter=list)

.PHONY: build
build:
	# Build frontend
	cd web && npm install && npm run build
	# Build backend (embeds frontend)
	CGO_ENABLED=1 go build -o bin/operative cmd/operative/main.go

.PHONY: install-deps
install-deps:
	go mod download
	cd web && npm install
