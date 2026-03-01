APP_NAME=llmpool

.PHONY: run build test lint up down

run:
	go run ./cmd/api

build:
	go build -o bin/$(APP_NAME) ./cmd/api

test:
	go test ./...

lint:
	go vet ./...

up:
	docker compose up --build

down:
	docker compose down
