.PHONY: test build run dev web-build

test:
	go test ./...
	bash scripts/dev_test.sh

build:
	go build ./cmd/agentx

run:
	go run ./cmd/agentx

dev:
	bash scripts/dev.sh

web-build:
	cd web && npm install && npm run build
