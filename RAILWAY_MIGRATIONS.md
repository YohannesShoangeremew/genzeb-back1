# Running Migrations on Railway

## Quick Method: Using Railway Dashboard (Easiest)

1. Go to your Railway project dashboard
2. Click on your **PostgreSQL** service
3. Click on **"Connect"** → **"Query"**
4. Copy and paste the SQL from `migrations/002_update_schema.sql`
5. Click **"Run"**
6. Repeat for `migrations/003_add_auth_fields.sql`

## Method 2: Using Railway Shell

```bash
# 1. Open Railway shell (runs inside Railway environment)
railway shell

# 2. Inside the shell, run migrations:
psql "$DATABASE_URL" < migrations/002_update_schema.sql
psql "$DATABASE_URL" < migrations/003_add_auth_fields.sql

# 3. Exit shell
exit
```

## Method 3: Using Railway Run with Service

```bash
# Make sure you're in the PostgreSQL service context
railway run --service Postgres psql "$DATABASE_URL" < migrations/002_update_schema.sql
railway run --service Postgres psql "$DATABASE_URL" < migrations/003_add_auth_fields.sql
```

## Method 4: Get Public Connection String

If you need to connect from your local machine:

1. Go to Railway dashboard → PostgreSQL service
2. Click **"Connect"** → **"Public Network"**
3. Copy the **Public Network** connection string
4. Use it locally:
   ```bash
   psql "postgresql://user:pass@host:port/dbname" < migrations/002_update_schema.sql
   ```

## Troubleshooting

**Error: "could not translate host name"**
- This happens when `$DATABASE_URL` contains Railway's internal hostname
- Solution: Use `railway shell` or Railway dashboard instead

**Error: "connection refused"**
- Make sure PostgreSQL service is running in Railway
- Check that you're using the correct service context

