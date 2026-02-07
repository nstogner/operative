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
