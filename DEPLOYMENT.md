# Deployment & Safe-Update Guide — EDL Bingo

How to ship changes to production **without downtime, without losing work, and
without breaking things for players.** Read this before your first deploy, and
follow the recipe every time.

---

## The setup (what runs where)

Everything runs on **Railway** (project `terrific-enjoyment`, `production` env):

| Service | Repo | What it is |
|---|---|---|
| `bingo-api` | `bingo-backend` | The Go API + Telegram bot (this repo) |
| `miniapp` | `bingo-frontend` (`/`) | The player Mini App |
| `admin` | `bingo-frontend` (`/admin`) | The admin dashboard |
| `Postgres` | — | The database (no public access) |
| `Redis` | — | Live game state + rate limiting |

**Every push to `main` auto-deploys.** Railway rebuilds the affected service and
switches traffic to the new version **only after `/health` passes** — so a
normal deploy has **zero downtime**; players never hit a dead server.

Bot: **@EDL_Bingobot** (webhook → the API's `/telegram/webhook`).

---

## The safe-update recipe (do this every time)

### 1. Work on a branch, not `main`
`main` is always "live" — pushing it deploys. So experiment on a branch:

```bash
git checkout -b my-feature      # work + commit freely here
# ...make changes, build, test...
git checkout main
git merge my-feature
git push                        # THIS is what deploys
```

A branch lets you build and test without touching what players are using.

### 2. Test before you push

```bash
# Backend (bingo-backend)
go build ./...
go test ./...

# Frontend (bingo-frontend/admin or /)
npx tsc --noEmit
npm run build
```

If these pass, it's safe to push. If they fail, fix before pushing — a failed
build on Railway just keeps the old (working) version live, but don't rely on
that as your safety net.

### 3. Push → automatic zero-downtime deploy
`git push origin main`. Railway builds, health-checks, then switches over.
Watch the deploy in the Railway dashboard (or the service's Deployments tab).

### 4. If something's wrong → roll back instantly
Railway → the service → **Deployments** → pick the previous good deploy →
**Redeploy**. Back to the last working version in seconds.

---

## ⚠️ Database migrations — the one thing that needs care

Migrations are **NOT applied automatically** — the app never runs `.sql` files
on boot. You apply them **by hand** against production Postgres. This is the
only part of shipping that can bite you, so follow these rules.

### The golden rule

> **Make schema changes ADDITIVE and backward-compatible, and run the migration
> BEFORE deploying the code that needs it.**

- **Adding** a column / table / index → safe. Run the migration first; the old
  running code ignores it, and the new code uses it. Zero downtime. (This is how
  every migration in `migrations/` this far was done — e.g. referrals `035`.)
- **Removing or renaming** a column → do it in **two separate deploys**:
  1. Ship code that stops using the column. Wait until it's live.
  2. *Later*, drop the column.
  Never drop/rename a column the currently-running code still reads — that
  breaks live players mid-deploy, and breaks rollback.
- New nullable columns and new `CHECK` values are safe; adding a `NOT NULL`
  column **must** have a `DEFAULT` (see `035_referrals.sql`).

Keeping migrations additive is also what makes **rollback safe** — the old code
still runs fine against the newer schema.

### How to apply a migration to production

Production Postgres has **no public access**, so you open a temporary TCP proxy,
run the migration, then delete the proxy. The Railway CLI does not work in some
environments — use the dashboard, or the GraphQL API.

**Option A — Railway dashboard (simplest):**
1. Railway → **Postgres** service → **Data** (query) tab.
2. Paste the contents of the new `migrations/03X_*.sql` file and run it.

**Option B — psql over a temporary proxy:**
1. Railway → Postgres → **Settings → Networking → TCP Proxy** → create one
   (app port `5432`). Note the host + port.
2. Get the password: Postgres service → **Variables** → `POSTGRES_PASSWORD`.
3. Run it:
   ```bash
   PGPASSWORD=<pw> psql -h <proxy-host> -p <proxy-port> -U bingo -d bingo \
     -v ON_ERROR_STOP=1 -f migrations/03X_your_migration.sql
   ```
4. **Delete the TCP proxy** when done — never leave the DB reachable.

**Before a risky migration, take a backup:** Railway Postgres → Backups (or
`pg_dump` over the proxy). Every migration here is written to be re-runnable
(`IF NOT EXISTS`, idempotent updates), but a backup is cheap insurance.

### Migration + code ordering for a normal (additive) change
1. Write the migration + the code, test locally (apply the migration to your dev
   DB first, then run the code against it).
2. **Apply the migration to production.**
3. **Then** push the code (which deploys).

That order means the old version and the new version both work throughout the
deploy window — no downtime, no errors.

---

## Environment variables

Set per-service in Railway → the service → **Variables**. Notable ones:

- API: `TELEGRAM_BOT_TOKEN`, `TELEGRAM_WEBHOOK_SECRET`, `TELEGRAM_MINIAPP_URL`,
  `TELEGRAM_BOT_USERNAME`, `ALLOWED_ORIGINS`, `TRUSTED_PROXIES`, `VERIFY_*`
  (payment accounts + key), `DATABASE_URL`/`DB_*`, `REDIS_*`, `JWT_SECRET`.
- Frontend (`VITE_*`) are **build-time** — changing one needs a **rebuild**
  (redeploy), not just a restart, and must be declared as `ARG` in the
  Dockerfile.

Changing an env var restarts the service (still health-checked, so no downtime).

---

## Why the server won't crash under load

- **Panic recovery on every background goroutine** — one bad game/edge case is
  logged and isolated to that operation; the server stays up for everyone else.
- **Graceful shutdown** — in-flight requests finish (10s) on deploy/restart.
- **Health-checked rolling deploys** — new version must pass `/health` before
  taking traffic.
- **Rate limiting** (Redis, per-action) — floods/abuse are throttled.
- **Atomic money operations** — deposits, withdrawals, refunds, referral
  payouts use locked DB transactions; concurrent players can't corrupt balances.
- **Auto-restart on failure** (Railway restart policy, up to 10 retries).

At scale, also: keep **daily DB backups** on, size the **Railway plan** for peak
concurrent players, and add **basic uptime/error alerting**.

---

## Quick reference

```
Push to main            = deploy (zero-downtime, automatic)
Branch for risky work   = merge to main only when ready
Migrations              = additive, run them BEFORE the code that needs them
Removing a column       = two deploys (stop using it, then drop it later)
Rollback                = Railway → Deployments → redeploy previous (instant)
Before every push       = build + test (go build/test, or tsc + vite build)
```
