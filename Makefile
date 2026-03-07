APP_NAME=llmpool
GOLANGCI_LINT=$(shell go env GOPATH)/bin/golangci-lint
MIGRATE ?= migrate
DB_DSN ?= postgres://postgres:postgres@localhost:5432/llmpool?sslmode=disable
MIGRATIONS_DIR ?= db/migrations
GO_PACKAGES=$(shell go list ./... | grep -v '/web/')

.PHONY: run build test lint up down migrate-up migrate-up-docker migrate-down migrate-version migrate-force

run:
	go run ./cmd/api

build:
	go build -o bin/$(APP_NAME) ./cmd/api

test:
	go test $(GO_PACKAGES)

lint:
	$(GOLANGCI_LINT) run ./...

up:
	docker compose up --build

down:
	docker compose down

migrate-up:
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DB_DSN)" up

migrate-up-docker:
	docker compose run --rm migrate

migrate-down:
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DB_DSN)" down 1

migrate-version:
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DB_DSN)" version

migrate-force:
	@test -n "$(VERSION)" || (printf "VERSION is required, e.g. make migrate-force VERSION=1\n" && exit 1)
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DB_DSN)" force $(VERSION)
