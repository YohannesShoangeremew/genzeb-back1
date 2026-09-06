# Project Context — Habesha Bingo

A real-money, multiplayer **75-ball bingo** platform played through a **Telegram
Mini App**, with an admin dashboard for operators. This file is the high-level
map of the whole system plus a log of recent work. For backend internals see
[`ARCHITECTURE.md`](./ARCHITECTURE.md); for game rules see
[`GAME_RULES.md`](./GAME_RULES.md) / [`GAME_RULES_QUICK.md`](./GAME_RULES_QUICK.md).

---

## 1. The three repositories

| Repo | Path | Remote | Hosts | Deploy |
|---|---|---|---|---|
| **Backend** | `~/bingo-backend` | `bingo-backend` | Go (Gin) HTTP + WebSocket API | Render (Docker) — `https://bingo-api-c6un.onrender.com` |
| **Frontend** | `~/bingo-miniapp` | `bingo-frontend` | Player Mini App (`src/`) **and** Admin dashboard (`admin/`) | Vercel (two projects) |

> ⚠️ `~/bingo-admin/` is a **stale, divergent, non-deployed** clone. Do **not**
> edit it — the live admin lives at `~/bingo-miniapp/admin/`.

Both frontends are React + Vite + TypeScript + Tailwind. The player app uses
`react-router` + `zustand` + `@tanstack/react-query`; the admin app uses
`react-router` + `zustand`.

---

## 2. What the system does

- **Players** open the Mini App inside Telegram. They are authenticated via
  Telegram `initData` (HMAC verified with the bot token → JWT).
- **Admins** log in with **phone + password** (bcrypt → JWT) to the admin
  dashboard. Admin routes additionally require an `admin` role claim.
- A **Telegram bot** is the registration gateway (webhook secured by a shared
  secret header).
- **Money** (wallets, transactions, games) lives in **PostgreSQL** (durable,
  row-locked for safety). **Redis** holds ephemeral live-game state and is the
  pub/sub backbone for the real-time WebSocket stream.
- Payments (Telebirr) are **manual**: players send money to the house
  accounts and submit a reference; an admin approves the deposit. No automated
  payment gateway.

---

## 3. Game model (75-ball bingo)

- **Card:** 5×5 grid, columns **B I N G O**. Each letter owns 15 numbers:
  B 1–15, I 16–30, N 31–45, G 46–60, O 61–75 → **75 numbers total**. Center
  cell (`N` middle) is **FREE**. Cards are deterministic, IDs **1–200**.
- **Stakes / game types:** REGULAR=10 birr, VIP=50 birr, both open to everyone.
  (The earlier G1–G7 tiers were retired in migration 014.)
- **Cards per player:** up to **4** in one game (migration 015), each costing one
  full stake (e.g. 4 cards on a 10-birr table = 40 birr), gated by wallet
  balance. The **card** is the unit of play — claims, eliminations and refunds
  are per-card. Cap enforced in the use case; a card is still unique per game.
- **House cut:** 20%. Prize pool = `stake × number of cards × 0.8` (grows per
  **card**, not per person). Winner takes the whole pool.
- **Flow:** `WAITING` → (2nd **distinct** player joins) `COUNTDOWN` (60s) →
  `DRAWING` (a number every 3s) → `FINISHED` (a valid bingo) or `CANCELLED`
  (refunded). `player_count` counts DISTINCT people, so one person holding
  several cards can't start a game alone.
- **Min players:** 2 distinct people. **Win patterns:** any row, column, either
  diagonal, or four corners (validated server-side in `pkg/bingo`, mirrored
  client-side in `src/lib/bingo.ts`).
- **Refund rules:** leave during `WAITING`/`COUNTDOWN` → refund the dropped
  card(s); leave during `DRAWING` → forfeit. A wrong claim eliminates **only
  that card** (the player's other cards keep playing); when no cards remain in
  play the game auto-cancels and refunds every stake.

---

## 4. Backend layout (clean architecture)

```
cmd/server/main.go        composition root: wires repos → usecases → handlers, routes
internal/handler/         Gin HTTP/WS handlers (auth, user, wallet, game, websocket, telegram)
internal/middleware/      JWT auth + admin gate
internal/usecase/         business rules (auth, user, wallet, game)
internal/domain/          entities, repo interfaces, DTOs, constants
internal/repository/postgres/  SQL implementations
pkg/                      jwt, auth(bcrypt), telegram, bingo(engine), redis, referral, utils
migrations/               init.sql + numbered migrations
```

Money operations run inside DB transactions with `SELECT … FOR UPDATE` row
locks. The single-winner guard is an atomic conditional `UPDATE … WHERE
state='DRAWING'`.

### Key API surface
- Public: `GET /games`, `/games/recent-winners`, `/games/:id/state`, `/cards/:id`
- Player (JWT): `/me`, `/me/wallet`, `POST /wallet/{deposit,withdraw,transfer}`,
  `POST /games/:id/{join,leave,bingo}`, `GET /me/games/:id/cards` (all of the
  player's cards in a game). `join` takes one `card_id`; `leave` takes an
  optional `card_id` (one card vs. whole game); `bingo` requires a `card_id`.
- WebSocket: `/ws/game?type=VIP` or `/ws/game/:gameId`
- Admin (JWT + admin): `/admin/users…`, `/admin/transactions…`,
  `/admin/stats/dashboard`, **`/admin/games`**, **`/admin/games/:id`**,
  **`/admin/games/:id/cancel`**

---

## 5. Real-time WebSocket events

The WS handler forwards the backend's `{event, data}` envelope verbatim from
Redis pub/sub. Events: `INITIAL_STATE`, `GAME_STATUS` (carries `status`; the
countdown one also carries `prize_pool`/`player_count`), `COUNTDOWN`,
`NUMBER_DRAWN`, `PLAYER_JOINED`, `PLAYER_LEFT` (both now carry the live
`prize_pool` + `player_count` so every connected client stays in sync — not just
the freshly-connected one), `PLAYER_ELIMINATED` (`userId`), `WINNER` (`user_id`,
`winner_name`, `prize`, `card_id`), `NEW_GAME_AVAILABLE`. The player app maps
these to live UI updates.

---

## 6. Recent progress

### Today (2026-06-25) — all merged to `main` and auto-deploying
1. **Two stake tiers (REGULAR / VIP)** — collapsed the seven G1–G7 tiers
   (5/7/10/20/50/100/200) into **REGULAR = 10 birr** and **VIP = 50 birr**, both
   open to everyone. Migration `014` relabels old rows + swaps the `game_type`
   CHECK; `CreateOrGetGame` now rejects unsupported tiers (closed a hole where an
   unknown type silently made a 0-birr game).
2. **Premium VIP lobby card** — the player Lobby is now two full-width tier
   cards; the VIP one stands out with a gold gradient + border + glow, a crown,
   an animated shine sweep, a glowing bet amount, a gold CTA and a heavier haptic.
3. **Up to 4 cards per player** — migration `015` drops
   `UNIQUE(game_id, user_id)` (keeps `UNIQUE(game_id, card_id)`); the cap of 4 is
   enforced in the use case. The card became the unit of play: per-card
   claim/elimination (a wrong claim kills only that card), per-card leave/refund,
   prize pool scales with cards, and `player_count` tracks DISTINCT people so the
   2-player start rule still needs two real people. Added
   `GET /me/games/:id/cards`; game history dedupes to one row per game. Frontend:
   multi-select `CardSelect` (running cost + cap + affordability), multi-card
   `GameRoom` with a scrollable stack and a per-card BINGO button.
4. **Live prize-sync fix** — the prize pool was only sent in `INITIAL_STATE`, so
   an already-connected player's pool froze (two players saw different prizes —
   8 vs 16). `PLAYER_JOINED`/`PLAYER_LEFT`/countdown `GAME_STATUS` now carry live
   `prize_pool` + `player_count`, and the room reads them.
5. **Render DB** — migrations `014` and `015` applied to production.
6. **Verification** — a throwaway harness drove the **real `GameUseCase` against
   live Postgres + Redis**: **32/32 checks** green — multi-card join, the 4-card
   cap, distinct-player counting, per-card leave/refund, a valid BINGO payout
   (winner takes the whole pool), per-card elimination, all-eliminated
   cancel+refund, and a pub/sub assertion that the join/countdown broadcasts now
   carry the live `prize_pool`. (Browser-level Mini App test not yet run.)
7. **Auto-mark (auto-daub)** — players asked to stop tapping each called number.
   The game room now **marks every drawn number on every card automatically**;
   marks are **derived** from the drawn-number set (`autoMarked(card, drawn)` in
   `src/lib/bingo.ts`), so a card is always in sync with what's been called.
   **Frontend-only** (`bingo-miniapp/src/`): the server already re-validates every
   claim against the real drawn numbers, so auto-marking changes nothing backend
   and can't be cheated.
8. **Automatic BINGO + pot splitting** (backend) — the game no longer waits for
   a player to tap BINGO. After **each drawn number**, `GameUseCase.checkAutoBingo`
   scans every active card and, because auto-daub means marks == drawn numbers on
   the card, declares a win the instant `ValidateBingo(card, drawn)` is true.
   - **Co-winners split the pot**: every card completing on the **same draw** is a
     winner and shares the prize pool evenly (`splitPot`, santim-rounded, remainder
     to the earliest joiner so the payout total is exactly the pool). One user
     holding several winning cards gets one share per card.
   - All winner resolution flows through the shared `finalizeWinners`, whose atomic
     `ClaimWinner` guard (DRAWING→FINISHED) still prevents any double payout if the
     now-vestigial manual `POST /games/:id/bingo` races the auto-check.
   - Per-card winner state is recorded on `game_players.is_winner` / `prize_won`
     (**migration `017_add_game_player_winnings.sql` — apply before deploy**).
     `games.winner_id` still stores the single *primary* winner for recent-winners;
     game history derives per-user win/amount from the per-card flags.
   - `winner` WebSocket event is backward-compatible: top-level fields = primary
     winner, plus new `winners[]` (each `{user_id, winner_name, prize, card_id,
     marked_numbers}`), `prize_pool`, and `split` bool. Frontend can hide the
     BINGO button and render all winners.

> ⚠️ Frontend and backend share the tier codes and `card_id` contract — when a
> change touches both they must deploy together. (The auto-mark change above is
> frontend-only — safe to ship alone.) Both auto-deploy from `main`.

### Prior cycle

#### Backend (`bingo-backend`, all merged to `main`)
- **Admin game management** — `GET /admin/games` (list, filters, pagination),
  `GET /admin/games/:id` (detail + players), `POST /admin/games/:id/cancel`
  (force-cancel + refund every active player, atomic, game-row locked).
- **Stuck-game auto-refund (money-trap fix)** — previously, when all 75 numbers
  were drawn with no winner, the draw loop spun forever and stakes were locked
  permanently. Now it auto-cancels and refunds everyone. Admin cancel and this
  path share one `cancelGameAndRefund` function.
- **Idempotent rejoin** — `JoinGame` returns the existing player if already in
  the game (reconnect-safe; no double charge).
- **Public recent-winners endpoint** — `GET /games/recent-winners` for the
  lobby trust feed.

#### Player Mini App (`bingo-miniapp/src/`, merged to `main`)
- **WebSocket fixes** — read `GAME_STATUS.status` (was `state`); show a
  "cancelled — refunded" overlay; handle `PLAYER_JOINED`/`PLAYER_LEFT`; read
  `PLAYER_ELIMINATED.userId`.
- **One-page game room** — fixed to the viewport (no scroll); the tall 75-cell
  master board replaced by a compact "called" strip (current ball + recent
  calls, lettered like `N42`); the card flexes to fit.
- **Manual marking** — removed auto-daub; the player taps each called number.
  *(Superseded 2026-06-25: auto-mark restored by player request — see Today #7.)*
- **Winner reveal** — every player (incl. losers/eliminated) sees who won and
  how much at game end.
- **Recent-winners lobby feed** — persistent, auto-refreshing list of recent
  winners (name · stake · prize · time).
- **Deposit destinations** — the deposit screen shows the house Telebirr number
  with a copy button. Configured via `VITE_*` env
  (`VITE_TELEBIRR_NUMBER`, `VITE_PAYMENT_NAME`).
- **UI refinement** — tighter, more professional sizing via the shared
  `Button`/`Card`/`Sheet` primitives.
- **Back navigation** — wired Telegram's native `BackButton` app-wide (returns
  to lobby on every non-home screen) + a game-room back chevron (non-destructive
  minimize, distinct from the "Leave"/refund button).

#### Admin dashboard (`bingo-miniapp/admin/`, merged to `main`)
- **Games section** — list (state/type filters), detail + player roster,
  **Cancel & refund** a game. The tool to recover money from stuck/abandoned
  games.
- **Mobile responsive** — off-canvas drawer + hamburger; tables scroll inside
  their card instead of stretching the page.

#### Verification
End-to-end tested against a local stack rebuilt from `main` (identical to
deployed code): seeded users, minted JWTs, drove join/leave/countdown, a real
win + payout, admin cancel+refund, and the 75-draw auto-refund — **44 checks,
all green**. (One blank-names failure was local schema drift — migration
`012_add_user_banned` not applied locally; production has it.)

---

## 7. Configuration & gotchas

- **Backend env:** `DATABASE`/`DB_*`, `REDIS_*`, `JWT_SECRET` (Render-generated,
  never exposed — so prod JWTs can't be minted locally), `SECRET_CODE`,
  `TELEGRAM_BOT_TOKEN`, `TELEGRAM_WEBHOOK_SECRET`, `TELEGRAM_MINIAPP_URL`.
- **Frontend env (player):** `VITE_API_BASE`, `VITE_WS_BASE`,
  `VITE_BOT_USERNAME`, and the deposit-account `VITE_*` above. Dev shims
  (`VITE_DEV_TELEGRAM_SHIM`, `VITE_DEV_MOCK_AUTH`) must be **off** in prod.
- **Render free Postgres** is deleted ~30 days after creation — fine for now,
  not durable long-term.
- **Local dev:** `docker compose up -d` runs api(:8000) + postgres(:5432) +
  redis. Rebuild the api image (`docker compose build api`) to pick up code
  changes — a stale container will 404 on new routes. Migrations are **not**
  auto-applied to the local DB; apply new ones by hand (the local DB now has
  `014`/`015`, plus throwaway rows from the 2026-06-25 e2e harness).
- **Migration gotcha:** a migration that swaps a CHECK constraint must DROP the
  old constraint *before* UPDATE-ing rows to the new values, or the still-active
  old constraint rejects the update (learned applying `014`).
- **Deploy order when a frontend change depends on a new backend endpoint:**
  ship the **backend first**, then the frontend (the lobby feed degrades
  gracefully — just hidden — if the endpoint isn't there yet).

---

## 8. Open follow-ups

- Set the **real Telebirr deposit account number**.
- **Browser-level smoke test** of the deployed Mini App (multi-card buy, the
  2-distinct-players start, per-card claim/refund) — logic is covered by the
  use-case + pub/sub tests, but no real browser/WS round-trip has been driven.
- Optional UX: show **total cards in play** in the room (today `player_count` is
  distinct people while the pool scales with cards, so the two can differ).
- Eyeball the **VIP lobby card**, UI sizing, and **Telegram back button** on a
  real device (taste/native behavior can't be verified in CI).
- Optional: louder error surfacing in admin game-detail enrichment; concurrency
  stress-testing of simultaneous claims/joins (guards exist, not load-tested).
