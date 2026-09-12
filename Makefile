.PHONY: run migrate-up migrate-down proto sqlc test test-unit test-integration test-e2e \
        compose-up compose-down compose-test-up compose-test-down lint certs

MIGRATE_DSN ?= mysql://root:root_password@tcp(127.0.0.1:3306)/gearshare
APP_DSN     ?= gearshare_app:app_password@tcp(127.0.0.1:3306)/gearshare?parseTime=true&multiStatements=true

run:
	go run ./cmd/api

run-notification:
	go run ./cmd/notification-service

run-indexer:
	go run ./cmd/search-indexer

migrate-up:
	migrate -path migrations/mysql -database "$(MIGRATE_DSN)" up

migrate-down:
	migrate -path migrations/mysql -database "$(MIGRATE_DSN)" down 1

proto:
	buf generate

sqlc:
	sqlc generate

lint:
	golangci-lint run ./...

test: test-unit

test-unit:
	go test ./... -short

test-integration:
	# Spins up its own MySQL testcontainer per test — just needs a Docker daemon.
	go test ./... -tags=integration -run TestIntegration -v

test-e2e:
	# Also self-contained via testcontainers; no docker-compose stack needed.
	go test ./test/e2e/... -tags=e2e -v

compose-up:
	docker compose up -d

compose-down:
	docker compose down

compose-test-up:
	docker compose -f docker-compose.test.yml up -d

compose-test-down:
	docker compose -f docker-compose.test.yml down -v

certs:
	bash scripts/gen-dev-certs.sh
