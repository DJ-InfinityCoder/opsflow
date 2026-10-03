.PHONY: migrate migrate-down seed run worker test lint

migrate:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

seed: migrate
	@echo "No seed data is defined until the domain schema is added."

run:
	go run ./cmd/api

worker:
	go run ./cmd/worker

test:
	go test ./... -race -count=1

lint:
	go vet ./...
