# Quick Start Guide

## Prerequisites

- Go 1.21 or higher
- Docker and Docker Compose (recommended)
- OR PostgreSQL 12+ and Redis 6+ installed locally

## Option 1: Using Docker (Recommended)

This is the easiest way to get started:

### 1. Start All Services

```bash
make docker-up
# Or: docker compose up --build -d
```

This will start:
- API on port 8080
- PostgreSQL on port 5432
- Redis on port 6379
- Database will be automatically initialized with the schema

### 2. Test the API

```bash
# Health check
curl http://localhost:8080/health

# Register a user
curl -X POST http://localhost:8080/api/v1/user/register \
  -H "Content-Type: application/json" \
  -d '{
    "telegram_id": 123456789,
    "first_name": "John",
    "last_name": "Doe",
    "phone": "+1234567890"
  }'
```

### 3. Stop Services (when done)

```bash
make docker-down
# Or: docker compose down
```

## Option 2: Local PostgreSQL and Redis

### 1. Install and Start PostgreSQL

```bash
# Create database
createdb bingo

# Run migrations
make migrate-up
```

### 2. Start Redis

```bash
# If installed via package manager
redis-server

# Or using Docker
docker run -d -p 6379:6379 redis:7-alpine
```

### 3. Configure Environment (Optional)

Create a `.env` file:

```bash
# Server
SERVER_PORT=8080
SERVER_HOST=0.0.0.0

# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=bingo
DB_SSLMODE=disable

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# JWT
JWT_SECRET=your-secret-key-change-in-production
```

### 4. Install Dependencies and Run

```bash
make deps
make run
```

## Development Commands

```bash
# Build the application
make build

# Run tests
make test

# Clean build artifacts
make clean

# Reset database (WARNING: drops all data)
make migrate-reset
```

## Verify Installation

1. Check health endpoint: `curl http://localhost:8080/health`
2. Should return: `{"status":"ok"}`

## Troubleshooting

### Database Connection Issues

- Ensure PostgreSQL is running: `pg_isready`
- Check database exists: `psql -l | grep bingo`
- Verify connection string in `.env` or config defaults

### Port Already in Use

- Change `SERVER_PORT` in `.env` or use a different port
- Check if another service is using port 8080: `lsof -i :8080`

### Migration Errors

- If tables already exist, use `make migrate-reset` (WARNING: deletes all data)
- Or manually drop tables and run `make migrate-up` again

## Next Steps

- Read the full [README.md](README.md) for API documentation
- Check available endpoints in the API Endpoints section
- Test wallet operations (deposit, withdraw, transfer)
