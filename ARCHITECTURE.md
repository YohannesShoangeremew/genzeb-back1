# Backend Architecture

The Bingo backend is a **Go (Gin)** HTTP + WebSocket API built in a **clean /
layered architecture**: handlers → use cases → repositories → database, with
dependencies pointing inward through interfaces defined in `domain`.

It is backed by **PostgreSQL** (durable source of truth for money & accounts)
and **Redis** (fast, ephemeral live-game state), authenticates players via the
**Telegram Mini App** and admins via **password**, and uses a **Telegram bot**
as the registration gateway. It is containerized and deployed on **Render**.

---

## High-level system

```
                   Telegram                 Players' browsers
                (Bot + Mini App)            (Mini App WebView)
                       │                            │
       webhook  ┌──────┴──────┐    HTTPS / WSS ┌────┴─────┐
       updates  │             ▼                ▼          │
                │      ┌───────────────────────────┐      │
                └─────▶│    Go API (Gin)  :8080     │◀─────┘
                       │    on Render (Docker)      │
                       └────────────┬──────────────┘
                         ┌──────────┴───────────┐
                         ▼                      ▼
                  ┌────────────┐         ┌────────────┐
                  │ PostgreSQL │         │   Redis    │
                  │ (durable)  │         │ (live game)│
                  └────────────┘         └────────────┘
```

---

## Layers (clean architecture)

```
cmd/server/main.go        ← composition root: build & wire everything (manual DI)
        │
        ▼
internal/handler/         ← HTTP layer (Gin): parse request, call use case, write JSON
internal/middleware/      ← cross-cutting: JWT auth, admin gate, CORS
        │
        ▼
internal/usecase/         ← business logic / rules (the "what"): auth, user, wallet, game
        │
        ▼
internal/domain/          ← entities + repository INTERFACES + DTOs (core, no deps)
        │
        ▼
internal/repository/postgres/   ← interface implementations: SQL queries
```

**Key idea — dependency inversion.** The inner layers (`domain`, `usecase`) do
not import Gin, Postgres, or Redis. `domain` declares interfaces such as
`UserRepository`; `repository/postgres` implements them. Business logic depends
on abstractions, so the storage engine can be swapped without touching it.

---

## Directory map

| Path | Responsibility |
|---|---|
| `cmd/server/main.go` | Entry point. Opens DB/Redis, constructs repos → use cases → handlers, registers routes, starts the server with graceful shutdown. |
| `config/` | Loads all settings from environment variables (DB, Redis, JWT, Telegram, admin secret). |
| `internal/domain/` | Core types (`User`, `Wallet`, `Transaction`, `Game`), repository **interfaces**, and request/response DTOs. |
| `internal/repository/postgres/` | SQL implementations: `user.go`, `wallet.go`, `transaction.go`, `transaction_service.go`, `game.go`. |
| `internal/usecase/` | Business logic: `auth.go`, `user.go`, `wallet.go`, `game.go`. |
| `internal/handler/` | Gin handlers: `auth`, `user`, `wallet`, `game`, `websocket`, `telegram` (bot webhook). |
| `internal/middleware/` | `AuthMiddleware` (verify JWT) and `AdminMiddleware` (require admin role). |
| `pkg/` | Reusable, framework-free helpers (see below). |
| `migrations/` | Versioned SQL schema (`init.sql` + numbered migrations). |

### `pkg/` — reusable toolbox

| Package | Purpose |
|---|---|
| `pkg/jwt` | Issue / verify JWT tokens. |
| `pkg/auth` | bcrypt password hashing. |
| `pkg/telegram` | `initdata.go` (verify Mini App signatures) + `bot.go` (Bot API client). |
| `pkg/bingo` | Game engine: `card.go` (card generation), `draw.go` (number draws). |
| `pkg/redis` | Redis client + live game-state service. |
| `pkg/referral` | Referral-code generation. |
| `pkg/utils` | Phone normalization / validation. |

---

## Request lifecycle (example: player joins a game)

```
POST /api/v1/games/:gameId/join
  │
  ├─ Gin router → CORS, Logger, Recovery middleware
  ├─ AuthMiddleware: validate JWT → put user_id in context
  │
  ├─ GameHandler.JoinGame: bind request, read user_id
  │
  ├─ GameUseCase.JoinGame: business rules
  │     ├─ begin DB transaction
  │     ├─ WalletRepository.LockForUpdate → deduct stake
  │     ├─ GameRepository.AddPlayer
  │     ├─ TransactionRepository.Create (audit)
  │     └─ commit
  │
  ├─ Redis: update live game state (players, taken cards)
  └─ JSON response → player
```

---

## Data stores — different jobs

### PostgreSQL — durable source of truth
- Tables: `users`, `wallets`, `transactions`, `games`, `game_players`.
- Holds money, balances, history, accounts — anything that must survive restarts.
- Financial operations use **DB transactions + row locks** (`LockForUpdate`) so
  balances cannot be corrupted under concurrency.

### Redis — fast ephemeral live state
- Active game state, drawn numbers, card availability.
- Real-time backbone for live games.
- Fast reads/writes during a game without hammering Postgres.

---

## Real-time games (WebSocket)

- `internal/handler/websocket.go` uses **gorilla/websocket**.
- Players connect to `/api/v1/ws/game?type=VIP` (by type) or `/ws/game/:gameId`.
- The server pushes drawn numbers and game events live; cards update instantly.
- Redis holds the shared game state that all connected clients observe.

---

## Authentication model

Two entry paths, one token system:

1. **Players (Mini App):** Telegram `initData` → `pkg/telegram.Validate`
   (HMAC-SHA256 with the bot token) → issue JWT.
2. **Admins (dashboard):** phone + password → bcrypt verify → issue JWT.
3. Protected routes run `AuthMiddleware` (verify JWT); admin routes additionally
   run `AdminMiddleware` (role check).
4. The **bot webhook** (`POST /telegram/webhook`) is secured by a shared secret
   header (`X-Telegram-Bot-Api-Secret-Token`), not JWT.

---

## External integrations

- **Telegram Bot API** (`pkg/telegram/bot.go`) — outbound: send messages /
  buttons; inbound: webhook updates (registration gateway).
- **Telegram Mini App** — `initData` signature verification for player login.
- **Mobile-money deposits** — Telebirr, CBE Birr, and M-Pesa references are recorded; deposits are auto-verified via the verify.leul.et integration when configured, otherwise settled manually via admin approval.

---

## API surface (overview)

| Group | Examples | Auth |
|---|---|---|
| Auth | `POST /auth/login`, `/auth/telegram`, `/auth/create-admin` | public |
| Bot webhook | `POST /telegram/webhook` | secret header |
| Public reads | `GET /games`, `/games/:id/state`, `/cards/:id` | none |
| Player (self) | `GET /me`, `/me/wallet`, `POST /wallet/deposit`, `/games/:id/join`, `/games/:id/bingo` | JWT |
| WebSocket | `GET /ws/game`, `/ws/game/:gameId` | query/JWT |
| Admin | `/admin/users`, `/admin/transactions/...`, `/admin/users/:id/{role,ban,make-admin,adjust-balance}` | JWT + admin |

---

## Deployment

- **`Dockerfile`** — multi-stage Go build producing a small runtime image.
- **`render.yaml`** — Render Blueprint provisioning the API (Docker) + Postgres
  + Redis, auto-wiring connection env vars.
- **Config via env vars** — `DATABASE`/`DB_*`, `REDIS_URL`, `JWT_SECRET`,
  `TELEGRAM_BOT_TOKEN`, `TELEGRAM_WEBHOOK_SECRET`, `TELEGRAM_MINIAPP_URL`,
  `SECRET_CODE`.
- Graceful shutdown on SIGINT/SIGTERM; connection pool tuned for concurrency
  (`SetMaxOpenConns(100)`).
- Migrations are applied to the database separately (see `RENDER_MIGRATIONS.md`).

---

## In one sentence

A **Gin** HTTP/WebSocket API in **clean-architecture layers**, backed by
**Postgres** for durable money/accounts and **Redis** for live game state,
authenticating players via **Telegram Mini App initData** and admins via
**password**, with a **Telegram bot** as the registration gateway — containerized
and deployed on **Render**.
