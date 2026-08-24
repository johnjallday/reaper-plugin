.PHONY: build artifact-local test test-ui fmt tidy

build:
	go build -o bin/reaper-plugin ./cmd/reaper-plugin

artifact-local:
	./scripts/build-local-artifact.sh

test:
	go test ./...

test-ui:
	node --test ui/**/*.test.js

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy
