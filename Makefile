.PHONY: all build test clean docker-up docker-down docker-logs test-api

APP_NAME = financial-ledger-engine

all: test build

build:
	go build -v -o bin/$(APP_NAME) ./cmd/api

test:
	go test -v ./...

clean:
	rm -rf bin/

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down -v

docker-logs:
	docker compose logs -f

test-api:
	@bash scripts/test_api.sh

