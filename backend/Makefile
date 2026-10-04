.PHONY: migrate migrate-down seed run worker test lint

ARGS ?=

migrate:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

seed: migrate
	go run ./cmd/seed $(ARGS)

run:
	go run ./cmd/api

worker:
	go run ./cmd/worker

test:
	go test ./... -race -count=1

lint:
	go vet ./...
