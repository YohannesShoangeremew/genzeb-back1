# Running Migrations on Render PostgreSQL

This guide explains how to run database migrations on your Render PostgreSQL database.

## Prerequisites

- Access to your Render dashboard
- PostgreSQL connection string from Render
- `psql` installed locally (or use Render's Shell)

## Method 1: Using Render Dashboard (Easiest - Recommended)

This is the simplest method and doesn't require any local setup.

1. **Go to your Render Dashboard**
   - Navigate to https://dashboard.render.com
   - Select your PostgreSQL database service

2. **Open the Query Editor**
   - Click on **"Connect"** or **"Info"** tab
   - Look for **"Query"** or **"SQL Editor"** option
   - Click to open the SQL query interface

3. **Run Migrations in Order**
   
   Copy and paste each migration file's contents and run them **one at a time**:
   
   **Migration 1: Base Schema**
   - Open `migrations/002_update_schema.sql`
   - Copy all contents
   - Paste into Render's query editor
   - Click **"Run"** or **"Execute"**
   
   **Migration 2: Auth Fields**
   - Open `migrations/003_add_auth_fields.sql`
   - Copy all contents
   - Paste into Render's query editor
   - Click **"Run"**
   
   **Migration 3: Games Schema**
   - Open `migrations/004_create_games_schema.sql`
   - Copy all contents
   - Paste into Render's query editor
   - Click **"Run"**

## Method 2: Using Render Shell (Interactive)

If Render provides a shell/terminal access:

1. **Open Render Shell**
   - Go to your PostgreSQL service in Render dashboard
   - Look for **"Shell"** or **"Terminal"** option
   - Open it

2. **Run Migrations**
   ```bash
   # The DATABASE_URL should be available as an environment variable
   psql "$DATABASE_URL" -f migrations/002_update_schema.sql
   psql "$DATABASE_URL" -f migrations/003_add_auth_fields.sql
   psql "$DATABASE_URL" -f migrations/004_create_games_schema.sql
   ```

## Method 3: Using psql from Local Machine

If you want to run migrations from your local machine:

1. **Get Connection String from Render**
   - Go to your PostgreSQL service in Render dashboard
   - Click on **"Connect"** or **"Info"**
   - Copy the **Internal Database URL** or **External Connection String**
   - It should look like: `postgresql://user:password@host:port/dbname`

2. **Run Migrations Locally**
   ```bash
   # Replace with your actual connection string
   export RENDER_DB_URL="postgresql://user:password@host:port/dbname"
   
   # Run migrations in order
   psql "$RENDER_DB_URL" -f migrations/002_update_schema.sql
   psql "$RENDER_DB_URL" -f migrations/003_add_auth_fields.sql
   psql "$RENDER_DB_URL" -f migrations/004_create_games_schema.sql
   ```

   Or using a script:
   ```bash
   # Make sure you have the connection string
   psql "postgresql://user:password@host:port/dbname" < migrations/002_update_schema.sql
   psql "postgresql://user:password@host:port/dbname" < migrations/003_add_auth_fields.sql
   psql "postgresql://user:password@host:port/dbname" < migrations/004_create_games_schema.sql
   ```

## Method 4: Using a Migration Script

Create a script to run all migrations at once:

```bash
#!/bin/bash

# Set your Render PostgreSQL connection string
RENDER_DB_URL="${RENDER_DB_URL:-postgresql://user:password@host:port/dbname}"

echo "Running migrations on Render PostgreSQL..."
echo ""

echo "Running migration: 002_update_schema.sql"
psql "$RENDER_DB_URL" -f migrations/002_update_schema.sql || exit 1

echo "Running migration: 003_add_auth_fields.sql"
psql "$RENDER_DB_URL" -f migrations/003_add_auth_fields.sql || exit 1

echo "Running migration: 004_create_games_schema.sql"
psql "$RENDER_DB_URL" -f migrations/004_create_games_schema.sql || exit 1

echo ""
echo "All migrations completed successfully!"
```

Save as `run_render_migrations.sh`, make it executable, and run:
```bash
chmod +x run_render_migrations.sh
export RENDER_DB_URL="your-connection-string-here"
./run_render_migrations.sh
```

## Migration Order

**IMPORTANT:** Run migrations in this exact order:

1. `002_update_schema.sql` - Creates users, wallets, and transactions tables
2. `003_add_auth_fields.sql` - Adds authentication fields (role, password)
3. `004_create_games_schema.sql` - Creates games, game_players, and drawn_numbers tables

## Verifying Migrations

After running migrations, verify they were successful:

```sql
-- Check if tables exist
SELECT table_name 
FROM information_schema.tables 
WHERE table_schema = 'public' 
ORDER BY table_name;

-- Expected tables:
-- - users
-- - wallets
-- - transactions
-- - games
-- - game_players
-- - drawn_numbers
```

## Troubleshooting

### Error: "relation already exists"
- Some tables might already exist
- Check which tables exist first
- You may need to drop existing tables if starting fresh (use `000_drop_all_tables.sql` with caution!)

### Error: "permission denied"
- Make sure you're using the correct database user credentials
- Check that the connection string has proper permissions

### Error: "could not connect to server"
- Verify the connection string is correct
- Check if Render PostgreSQL service is running
- Ensure you're using the correct host (internal vs external)

### Error: "column already exists"
- The migration might have been partially run
- Check the current schema state
- You may need to manually fix the schema

## Getting Your Connection String

1. **From Render Dashboard:**
   - Go to your PostgreSQL service
   - Click **"Connect"** or **"Info"**
   - Look for **"Internal Database URL"** or **"Connection String"**
   - Copy the full connection string

2. **From Environment Variables:**
   - If your backend service is on Render, check its environment variables
   - Look for `DATABASE_URL` or similar

## Security Note

⚠️ **Never commit your database connection string to version control!**

Always use environment variables or Render's secure connection methods.

