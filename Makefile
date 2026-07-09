# Load .env for local dev so every target sees DATABASE_URL; production uses real injected env vars.
ifeq (,$(wildcard .env))
$(shell cp .env.example .env 2>/dev/null)
endif
-include .env
export

.PHONY: run test tidy db-up db-down migrate-up migrate-down migrate-verify help

# Default target
help:
	@echo "Available targets:"
	@echo "  run            - Run the application"
	@echo "  test           - Run tests"
	@echo "  tidy           - Tidy Go modules"
	@echo "  db-up          - Start PostgreSQL container"
	@echo "  db-down        - Stop PostgreSQL container"
	@echo "  migrate-up     - Run database migrations (up)"
	@echo "  migrate-down   - Rollback database migrations (down)"
	@echo "  migrate-verify - Run migrations and verify tables exist"

# Run the application
run:
	go run ./cmd/server

# Run tests
test:
	go test ./...

# Tidy Go modules
tidy:
	go mod tidy

# Start PostgreSQL container
db-up:
	docker-compose up -d

# Stop PostgreSQL container
db-down:
	docker-compose down

# Run migrations up
migrate-up:
	@if ! command -v migrate &> /dev/null; then \
		echo "Error: golang-migrate CLI not found. Install it first:"; \
		echo "  macOS: brew install golang-migrate"; \
		echo "  Linux: see https://github.com/golang-migrate/migrate/tree/master/cmd/migrate"; \
		exit 1; \
	fi
	migrate -path migrations -database "$$DATABASE_URL" up

# Run migrations down
migrate-down:
	@if ! command -v migrate &> /dev/null; then \
		echo "Error: golang-migrate CLI not found. Install it first:"; \
		echo "  macOS: brew install golang-migrate"; \
		echo "  Linux: see https://github.com/golang-migrate/migrate/tree/master/cmd/migrate"; \
		exit 1; \
	fi
	migrate -path migrations -database "$$DATABASE_URL" down 1

# Run migrations and verify tables
migrate-verify:
	@echo "Running migrations..."
	@$(MAKE) migrate-up
	@echo ""
	@echo "Verifying tables..."
	@docker exec -it postgres psql -U app -d transfers -c "\dt" || \
		(echo "Error: Could not verify tables. Is PostgreSQL running? (make db-up)" && exit 1)
	@echo ""
	@echo "✓ Migration verification complete"
