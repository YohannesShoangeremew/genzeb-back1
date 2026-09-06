# Railway Deployment Guide

This guide will walk you through deploying the Bingo backend to Railway.

## Prerequisites

1. A Railway account (sign up at [railway.app](https://railway.app))
2. Your code pushed to a Git repository (GitHub, GitLab, or Bitbucket)

## Step-by-Step Deployment

### Step 1: Create a New Project on Railway

1. Go to [railway.app](https://railway.app) and sign in
2. Click **"New Project"**
3. Select **"Deploy from GitHub repo"** (or your Git provider)
4. Select your repository containing the backend code
5. Railway will automatically detect it's a Go project

### Step 2: Add PostgreSQL Database

1. In your Railway project, click **"+ New"**
2. Select **"Database"** → **"Add PostgreSQL"**
3. Railway will automatically create a PostgreSQL service
4. Note the connection details (they'll be available as environment variables)

### Step 3: Add Redis (Required for Game Features)

**Important:** Redis is required for the game system to work properly. Without Redis:
- Games will work but without real-time WebSocket updates
- Game state caching will be disabled
- Real-time player count and number drawing events won't work

1. In your Railway project, click **"+ New"**
2. Select **"Database"** → **"Add Redis"**
3. Railway will automatically create a Redis service
4. Note: The application will start without Redis but game real-time features will be disabled

### Step 4: Configure Environment Variables

Go to your service settings and add these environment variables:

#### Required Variables:

```bash
# Server Configuration
SERVER_PORT=8080
SERVER_HOST=0.0.0.0

# Database Configuration (Railway provides these automatically)
# These are automatically set by Railway when you add PostgreSQL:
# DATABASE_URL (Railway provides this)
# Or manually set:
DB_HOST=${{Postgres.PGHOST}}
DB_PORT=${{Postgres.PGPORT}}
DB_USER=${{Postgres.PGUSER}}
DB_PASSWORD=${{Postgres.PGPASSWORD}}
DB_NAME=${{Postgres.PGDATABASE}}
DB_SSLMODE=require

# JWT Configuration
JWT_SECRET=your-super-secret-jwt-key-change-this-in-production
JWT_EXPIRATION_HOURS=24

# Redis Configuration (Required for game real-time features)
# Railway provides these automatically when you add Redis:
# Or manually set:
REDIS_HOST=${{Redis.REDIS_HOST}}
REDIS_PORT=${{Redis.REDIS_PORT}}
REDIS_PASSWORD=${{Redis.REDIS_PASSWORD}}

# Note: If Redis is not available, the app will start but:
# - WebSocket endpoints will be disabled
# - Game real-time updates won't work
# - Games will still function but without live updates
```

**Important Notes:**
- Railway automatically provides `DATABASE_URL` when you add PostgreSQL
- You can reference other services using `${{ServiceName.VARIABLE}}` syntax
- Generate a strong `JWT_SECRET` (use a random string generator)

### Step 5: Update Config to Use DATABASE_URL (Optional)

Railway provides `DATABASE_URL` which is a connection string. You can either:
- Use `DATABASE_URL` directly, or
- Parse it into individual components

For now, the current config uses individual variables which Railway can provide.

### Step 6: Run Database Migrations

After deployment, you need to run migrations. You have two options:

#### Option A: Using Railway CLI (Recommended)

1. Install Railway CLI:
   ```bash
   npm i -g @railway/cli
   # Or using Homebrew:
   brew install railway
   ```

2. Login to Railway:
   ```bash
   railway login
   ```

3. Link your project:
   ```bash
   railway link
   ```

4. Run migrations:

   **Method 1: Using Railway Dashboard (Easiest - Recommended)**
   1. Go to Railway dashboard → Your PostgreSQL service
   2. Click **"Connect"** → **"Query"**
   3. Copy and paste the contents of each migration file and run:
      - `migrations/002_update_schema.sql` (users, wallets, transactions)
      - `migrations/003_add_auth_fields.sql` (authentication fields)
      - `migrations/004_create_games_schema.sql` (games, game_players, drawn_numbers)
   4. Click **"Run"** after each migration

   **Method 2: Using Railway Shell (Interactive)**
   ```bash
   # Start interactive Railway shell for PostgreSQL service
   railway shell --service Postgres
   
   # Inside the shell, run migrations:
   psql "$DATABASE_URL" -f migrations/002_update_schema.sql
   psql "$DATABASE_URL" -f migrations/003_add_auth_fields.sql
   psql "$DATABASE_URL" -f migrations/004_create_games_schema.sql
   
   # Exit when done
   exit
   ```

   **Method 3: Using Railway Run with Service**
   ```bash
   # Run commands in PostgreSQL service context
   railway run --service Postgres psql "$DATABASE_URL" -f migrations/002_update_schema.sql
   railway run --service Postgres psql "$DATABASE_URL" -f migrations/003_add_auth_fields.sql
   railway run --service Postgres psql "$DATABASE_URL" -f migrations/004_create_games_schema.sql
   ```

#### Option B: Using Railway Dashboard Query Tool

1. Go to your PostgreSQL service in Railway dashboard
2. Click on **"Connect"** → **"Query"**
3. Copy and paste the contents of each migration file and run:
   - `migrations/002_update_schema.sql` (users, wallets, transactions tables)
   - `migrations/003_add_auth_fields.sql` (role and password fields)
   - `migrations/004_create_games_schema.sql` (games, game_players, drawn_numbers tables)

### Step 7: Configure Build Settings

Railway should auto-detect Go, but verify:

1. Go to your service → **Settings** → **Build**
2. Ensure:
   - **Build Command**: `go build -o server cmd/server/main.go`
   - **Start Command**: `./server`
   - **Root Directory**: `/` (or leave empty if root)

**Note:** The `railway.json` and `Procfile` files in the repository already configure this, so Railway should use those settings automatically.

### Step 8: Deploy

1. Railway will automatically deploy when you push to your connected branch
2. Or manually trigger a deployment from the dashboard
3. Check the **Deployments** tab for build logs

### Step 9: Verify Deployment

1. Once deployed, Railway will provide a public URL (e.g., `https://your-app.railway.app`)
2. Test the health endpoint:
   ```bash
   curl https://your-app.railway.app/health
   ```
3. Should return: `{"status":"ok"}`

4. Test game endpoints (if Redis is configured):
   ```bash
   # Get available games
   curl https://your-app.railway.app/api/v1/games
   
   # Should return: {"games":[]} (empty if no games yet)
   ```

5. Verify WebSocket endpoint (if Redis is configured):
   - The WebSocket endpoint is available at: `wss://your-app.railway.app/api/v1/ws/game/{gameId}?user_id={userId}`
   - Test using a WebSocket client or browser console

### Step 10: Create Admin User

After deployment, create an admin user:

1. Register a user via API:
   ```bash
   curl -X POST https://your-app.railway.app/api/v1/user/register \
     -H "Content-Type: application/json" \
     -d '{
       "telegram_id": 123456789,
       "first_name": "Admin",
       "last_name": "User",
       "phone": "+1234567890"
     }'
   ```

2. Hash a password:
   ```bash
   # Locally
   go run scripts/create_admin.go your_password
   ```

3. Update user to admin using Railway CLI:
   ```bash
   railway run psql $DATABASE_URL -c \
     "UPDATE users SET role = 'admin', password = '\$2a\$10\$hashed_password_here' WHERE telegram_id = 123456789;"
   ```

## Environment Variables Reference

### Server Config
- `SERVER_PORT`: Port to run server on (default: 8080)
- `SERVER_HOST`: Host to bind to (use `0.0.0.0` for Railway)

### Database Config
- `DB_HOST`: PostgreSQL host (provided by Railway)
- `DB_PORT`: PostgreSQL port (provided by Railway)
- `DB_USER`: PostgreSQL user (provided by Railway)
- `DB_PASSWORD`: PostgreSQL password (provided by Railway)
- `DB_NAME`: Database name (provided by Railway)
- `DB_SSLMODE`: SSL mode (use `require` for Railway)

### JWT Config
- `JWT_SECRET`: Secret key for JWT tokens (generate a strong random string)
- `JWT_EXPIRATION_HOURS`: Token expiration in hours (default: 24)

### Redis Config (Required for Game Features)
- `REDIS_HOST`: Redis host (provided by Railway when you add Redis service)
- `REDIS_PORT`: Redis port (provided by Railway, typically 6379)
- `REDIS_PASSWORD`: Redis password (if set by Railway)

**Note:** 
- Without Redis, the application will start but game real-time features will be disabled
- WebSocket endpoints require Redis to function
- Game state caching and pub/sub events require Redis

## Troubleshooting

### Build Fails
- Check build logs in Railway dashboard
- Ensure Go version is compatible (Go 1.21+)
- Verify `go.mod` is correct

### Database Connection Fails
- Verify environment variables are set correctly
- Check that PostgreSQL service is running
- Ensure `DB_SSLMODE=require` for Railway's PostgreSQL

### Application Crashes
- Check application logs in Railway dashboard
- Verify all required environment variables are set
- Check database migrations have been run
- If you see Redis connection errors, the app will still start but game features will be limited

### Redis Connection Issues
- **Warning messages are OK**: If you see "Warning: Failed to connect to Redis", the app will still start
- **Game features disabled**: Without Redis, games work but without real-time WebSocket updates
- **To enable Redis**: Add Redis service in Railway and set `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD`
- **Check Redis service**: Ensure Redis service is running in Railway dashboard

### WebSocket Not Working
- Verify Redis is configured and connected
- Check that `REDIS_HOST`, `REDIS_PORT` are set correctly
- Test WebSocket connection: `wss://your-app.railway.app/api/v1/ws/game/{gameId}?user_id={userId}`
- Ensure user is already a player in the game before connecting

### Port Issues
- Railway automatically assigns a `PORT` environment variable
- You may need to update config to use `PORT` instead of `SERVER_PORT`
- Or set `SERVER_PORT=$PORT` in environment variables

## Updating Your Application

1. Push changes to your Git repository
2. Railway will automatically detect and deploy
3. Or manually trigger deployment from dashboard

## Monitoring

- View logs: Railway dashboard → Your service → **Logs**
- View metrics: Railway dashboard → Your service → **Metrics**
- Set up alerts: Railway dashboard → Project → **Settings** → **Notifications**

## Production Checklist

- [ ] Set strong `JWT_SECRET` environment variable
- [ ] Run all database migrations (002, 003, 004)
- [ ] Add Redis service (required for game features)
- [ ] Configure Redis environment variables
- [ ] Create admin user
- [ ] Test all endpoints (user, wallet, game, admin)
- [ ] Test WebSocket connection
- [ ] Set up custom domain (optional)
- [ ] Configure CORS for your frontend domain
- [ ] Set up monitoring and alerts
- [ ] Review and optimize connection pool settings
- [ ] Test game creation and joining flow
- [ ] Verify real-time game updates via WebSocket

## Custom Domain (Optional)

1. Go to your service → **Settings** → **Networking**
2. Click **"Generate Domain"** or **"Add Custom Domain"**
3. Follow the DNS configuration instructions

## Database Tables Overview

After running all migrations, your database will have:

1. **users** - User accounts (telegram_id, phone_number, referral_code, role, password)
2. **wallets** - User wallets (balance, demo_balance)
3. **transactions** - Transaction ledger (deposits, withdrawals, transfers)
4. **games** - Game instances (game_type, state, bet_amount, prize_pool, winner_id)
5. **game_players** - Players in games (user_id, card_id, is_eliminated)
6. **drawn_numbers** - Drawn numbers history (letter, number, drawn_at)

## Game System Features

The deployed backend includes:

- **7 Game Types**: G1 (5), G2 (7), G3 (10), G4 (20), G5 (50), G6 (100), G7 (200)
- **Game States**: WAITING → COUNTDOWN → DRAWING → FINISHED → CLOSED
- **Real-time Updates**: WebSocket events for game state changes
- **Server-Authoritative**: All game logic runs on the server
- **Atomic Transactions**: All money operations are transactional
- **Redis Caching**: Game state cached in Redis for performance

## WebSocket Endpoints

- **Connection**: `wss://your-app.railway.app/api/v1/ws/game/{gameId}?user_id={userId}`
- **Events**: GAME_STATUS, NUMBER_DRAWN, WINNER, PLAYER_JOINED, PLAYER_LEFT, etc.
- **Requires**: Redis service to be configured
- **Authentication**: User must be a player in the game

## Cost Optimization

- Railway offers a free tier with usage limits
- Monitor your usage in the dashboard
- Consider upgrading if you exceed free tier limits
- Redis service adds to costs but is required for game features

