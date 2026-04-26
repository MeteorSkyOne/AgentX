.PHONY: test build run dev prod web-build

test:
	go test ./...
	bash scripts/dev_test.sh

build:
	go build ./cmd/agentx

run:
	go run ./cmd/agentx

dev:
	bash scripts/dev.sh

prod:
	bash scripts/prod.sh

web-build:
	cd web && pnpm install --frozen-lockfile && pnpm run build
