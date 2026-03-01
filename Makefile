APP_NAME=llmpool
GOLANGCI_LINT=$(shell go env GOPATH)/bin/golangci-lint

.PHONY: run build test lint up down

run:
	go run ./cmd/api

build:
	go build -o bin/$(APP_NAME) ./cmd/api

test:
	go test ./...

lint:
	$(GOLANGCI_LINT) run ./...

up:
	docker compose up --build

down:
	docker compose down
