.PHONY: test build run web-build

test:
	go test ./...

build:
	go build ./cmd/agentx

run:
	go run ./cmd/agentx

web-build:
	cd web && npm install && npm run build
