#!/bin/bash

# Script to run all migrations on Render PostgreSQL
# Usage: 
#   export RENDER_DB_URL="postgresql://user:password@host:port/dbname"
#   ./run_render_migrations.sh

set -e  # Exit on error

# Check if RENDER_DB_URL is set
if [ -z "$RENDER_DB_URL" ]; then
    echo "Error: RENDER_DB_URL environment variable is not set"
    echo ""
    echo "Usage:"
    echo "  export RENDER_DB_URL=\"postgresql://user:password@host:port/dbname\""
    echo "  ./run_render_migrations.sh"
    echo ""
    echo "You can get your connection string from:"
    echo "  Render Dashboard → Your PostgreSQL Service → Connect → Internal Database URL"
    exit 1
fi

echo "=========================================="
echo "Running Migrations on Render PostgreSQL"
echo "=========================================="
echo ""

# Check if psql is available
if ! command -v psql &> /dev/null; then
    echo "Error: psql is not installed or not in PATH"
    echo "Install PostgreSQL client tools to use this script"
    exit 1
fi

# Test connection
echo "Testing database connection..."
if ! psql "$RENDER_DB_URL" -c "SELECT 1;" > /dev/null 2>&1; then
    echo "Error: Could not connect to database"
    echo "Please verify your RENDER_DB_URL connection string"
    exit 1
fi
echo "✓ Connection successful"
echo ""

# Run migrations in order
MIGRATIONS=(
    "002_update_schema.sql"
    "003_add_auth_fields.sql"
    "004_create_games_schema.sql"
)

for migration in "${MIGRATIONS[@]}"; do
    if [ ! -f "migrations/$migration" ]; then
        echo "Error: Migration file not found: migrations/$migration"
        exit 1
    fi
    
    echo "Running migration: $migration"
    if psql "$RENDER_DB_URL" -f "migrations/$migration"; then
        echo "✓ $migration completed successfully"
    else
        echo "✗ $migration failed"
        exit 1
    fi
    echo ""
done

echo "=========================================="
echo "All migrations completed successfully!"
echo "=========================================="

# Verify tables were created
echo ""
echo "Verifying tables..."
psql "$RENDER_DB_URL" -c "
SELECT table_name 
FROM information_schema.tables 
WHERE table_schema = 'public' 
  AND table_type = 'BASE TABLE'
ORDER BY table_name;
"

