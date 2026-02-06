.PHONY: build-sandbox

build-sandbox:
	docker build -t sandbox-python:latest ./pkg/sandbox/docker/image
