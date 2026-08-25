.PHONY: build artifact-local release-package test test-ui fmt tidy

build:
	go build -o bin/reaper-plugin ./cmd/reaper-plugin

artifact-local:
	./scripts/build-local-artifact.sh

release-package:
	./scripts/package-release.sh $(VERSION)

test:
	go test ./...

test-ui:
	node --test ui/**/*.test.js

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy
