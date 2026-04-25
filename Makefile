.PHONY: test build run

test:
	go test ./...

build:
	go build ./cmd/agentx

run:
	go run ./cmd/agentx
