.PHONY: build test fmt tidy

build:
	go build -o bin/reaper-mcp ./cmd/reaper-mcp

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy
