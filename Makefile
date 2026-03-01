.PHONY: up down build run dev logs

up:
	docker compose up --build -d

down:
	docker compose down

build:
	go build -o bin/uptime-monitor ./cmd/server

run: build
	PORT=8080 \
	DATABASE_URL='postgres://uptime:uptime@localhost:5432/uptime?sslmode=disable' \
	REDIS_URL='redis://localhost:6379' \
	./bin/uptime-monitor

dev:
	docker compose up -d postgres redis
	PORT=8080 \
	DATABASE_URL='postgres://uptime:uptime@localhost:5432/uptime?sslmode=disable' \
	REDIS_URL='redis://localhost:6379' \
	go run ./cmd/server

logs:
	docker compose logs -f
