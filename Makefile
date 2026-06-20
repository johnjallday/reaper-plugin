.PHONY: build test fmt tidy

build:
	go build -o bin/reaper-plugin ./cmd/reaper-plugin

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy
