.PHONY: build run test migrate-up migrate-down docker-up docker-down

DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/gamelogserver?sslmode=disable
MIGRATE_CMD = migrate -path migrations -database "$(DATABASE_URL)"

build:
	go build -o bin/server ./cmd/server

run:
	go run ./cmd/server

test:
	go test ./...

migrate-up:
	$(MIGRATE_CMD) up

migrate-down:
	$(MIGRATE_CMD) down

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down
