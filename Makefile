.PHONY: up down logs migrate migrate-down run-api run-worker run-crawler tidy web-install web-dev seed-env up-infra

up:
	docker compose up -d --build

up-infra:
	docker compose up -d postgres redis minio minio-init

down:
	docker compose down

logs:
	docker compose logs -f --tail=200

migrate:
	docker compose run --rm migrate

migrate-down:
	goose -dir migrations postgres "$$DATABASE_URL" down

tidy:
	go mod tidy

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

run-crawler:
	go run ./cmd/crawler

web-install:
	cd apps/web && npm install && npm install react-router-dom @babylonjs/core @babylonjs/gui

web-dev:
	cd apps/web && npm run dev

seed-env:
	cp -n .env.example .env || true
