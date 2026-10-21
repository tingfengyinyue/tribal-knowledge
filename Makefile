.PHONY: run build up down migrate

run:
	go run ./cmd/server

build:
	go build -o bin/tk-server ./cmd/server

up:
	docker compose up -d

down:
	docker compose down

test:
	go test ./...
