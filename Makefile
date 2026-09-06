.PHONY: build run test clean migrate-up migrate-all migrate-reset db-drop docker-up docker-down hash-password

# Build the application
build:
	go build -o bin/server cmd/server/main.go

# Run the application
run:
	go run cmd/server/main.go

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/

# Run all database migrations in order
migrate-all:
	@echo "Running all migrations..."
	DOCKER_HOST=unix:///run/docker.sock docker exec -i bingo-postgres psql -U postgres -d bingo < migrations/002_update_schema.sql || \
	psql -d bingo -f migrations/002_update_schema.sql
	DOCKER_HOST=unix:///run/docker.sock docker exec -i bingo-postgres psql -U postgres -d bingo < migrations/003_add_auth_fields.sql || \
	psql -d bingo -f migrations/003_add_auth_fields.sql

# Run specific migration (for development)
migrate-up:
	DOCKER_HOST=unix:///run/docker.sock docker exec -i bingo-postgres psql -U postgres -d bingo < migrations/002_update_schema.sql || \
	psql -d bingo -f migrations/002_update_schema.sql
	DOCKER_HOST=unix:///run/docker.sock docker exec -i bingo-postgres psql -U postgres -d bingo < migrations/003_add_auth_fields.sql || \
	psql -d bingo -f migrations/003_add_auth_fields.sql

# Drop all tables (WARNING: This will delete all data!)
db-drop:
	@echo "WARNING: Dropping all tables and deleting all data!"
	DOCKER_HOST=unix:///run/docker.sock docker exec bingo-postgres psql -U postgres -d bingo -c "DROP TABLE IF EXISTS transactions CASCADE; DROP TABLE IF EXISTS wallets CASCADE; DROP TABLE IF EXISTS users CASCADE; DROP FUNCTION IF EXISTS update_updated_at_column() CASCADE;" || \
	psql -d bingo -f migrations/000_drop_all_tables.sql || \
	echo "Note: If using Docker, ensure container is running. If using local PostgreSQL, ensure psql is in PATH."
	@echo "All tables dropped!"

# Reset database (WARNING: This will drop all tables and recreate them)
migrate-reset: db-drop
	@echo "Recreating database schema..."
	DOCKER_HOST=unix:///run/docker.sock docker exec -i bingo-postgres psql -U postgres -d bingo < migrations/002_update_schema.sql || \
	psql -d bingo -f migrations/002_update_schema.sql
	DOCKER_HOST=unix:///run/docker.sock docker exec -i bingo-postgres psql -U postgres -d bingo < migrations/003_add_auth_fields.sql || \
	psql -d bingo -f migrations/003_add_auth_fields.sql

# Start docker services (PostgreSQL and Redis)
docker-up:
	DOCKER_HOST=unix:///run/docker.sock docker-compose up -d

# Stop docker services
docker-down:
	DOCKER_HOST=unix:///run/docker.sock docker-compose down

# Connect to database
db-shell:
	DOCKER_HOST=unix:///run/docker.sock docker exec -it bingo-postgres psql -U postgres -d bingo

# View database tables
db-tables:
	DOCKER_HOST=unix:///run/docker.sock docker exec bingo-postgres psql -U postgres -d bingo -c "\dt"

# Install dependencies
deps:
	go mod download
	go mod verify

# Hash password for admin user creation
hash-password:
	@if [ -z "$(PASSWORD)" ]; then \
		echo "Usage: make hash-password PASSWORD=your_password"; \
		exit 1; \
	fi
	@go run scripts/create_admin.go $(PASSWORD)

