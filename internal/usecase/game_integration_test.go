//go:build integration

// End-to-end integration tests for automatic bingo detection and pot splitting.
// They drive the REAL GameUseCase against a live Postgres + Redis, exercising the
// full path: seed a DRAWING game with players, feed the winning drawn numbers,
// run checkAutoBingo, then assert the game finished, wallets were credited, the
// pot split correctly, and per-card winner state was recorded.
//
// Run (against the local dev DB/Redis):
//
//	DB_HOST=127.0.0.1 DB_USER=postgres DB_PASSWORD=... DB_NAME=bingo \
//	  go test -tags=integration -run Integration ./internal/usecase/ -v
package usecase

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/repository/postgres"
	"github.com/bingo/backend/pkg/bingo"
	redisPkg "github.com/bingo/backend/pkg/redis"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	goredis "github.com/redis/go-redis/v9"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// harness wires the real use case to live infra and provides seed/cleanup.
type harness struct {
	t   *testing.T
	db  *sql.DB
	rdb *goredis.Client
	uc  *GameUseCase
	ids struct {
		users []uuid.UUID
		games []uuid.UUID
	}
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dsn := "host=" + env("DB_HOST", "127.0.0.1") +
		" port=" + env("DB_PORT", "5432") +
		" user=" + env("DB_USER", "postgres") +
		" password=" + env("DB_PASSWORD", "postgres") +
		" dbname=" + env("DB_NAME", "bingo") +
		" sslmode=" + env("DB_SSLMODE", "disable")

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("no database reachable (%v) — skipping integration test", err)
	}

	rdb := goredis.NewClient(&goredis.Options{
		Addr:     env("REDIS_HOST", "127.0.0.1") + ":" + env("REDIS_PORT", "6379"),
		Password: os.Getenv("REDIS_PASSWORD"),
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("no redis reachable (%v) — skipping integration test", err)
	}

	uc := NewGameUseCase(
		postgres.NewGameRepository(db),
		postgres.NewWalletRepository(db),
		postgres.NewTransactionRepository(db),
		postgres.NewUserRepository(db),
		postgres.NewBonusRepository(db),
		postgres.NewBotRepository(db),
		nil,
		db,
		redisPkg.NewGameStateService(rdb),
	)
	return &harness{t: t, db: db, rdb: rdb, uc: uc}
}

// seedUser inserts a user + wallet and returns the user id. tgSuffix keeps
// telegram_id / phone / referral unique per test player.
func (h *harness) seedUser(name string, tgSuffix int64) uuid.UUID {
	h.t.Helper()
	id := uuid.New()
	tg := int64(990000000) + tgSuffix
	_, err := h.db.Exec(
		`INSERT INTO users (id, telegram_id, first_name, phone_number, referal_code, role)
		 VALUES ($1,$2,$3,$4,$5,'user')`,
		id, tg, name, "0900"+itoa(tg), "REF"+itoa(tg),
	)
	if err != nil {
		h.t.Fatalf("seed user: %v", err)
	}
	if _, err := h.db.Exec(`INSERT INTO wallets (user_id, balance) VALUES ($1, 0)`, id); err != nil {
		h.t.Fatalf("seed wallet: %v", err)
	}
	h.ids.users = append(h.ids.users, id)
	return id
}

func (h *harness) seedBot(name string, suffix int64) uuid.UUID {
	h.t.Helper()
	id := uuid.New()
	tg := int64(botTelegramIDBase) - suffix
	_, err := h.db.Exec(
		`INSERT INTO users (id, telegram_id, first_name, phone_number, referal_code, role, is_bot)
		 VALUES ($1,$2,$3,$4,$5,'user',true)`,
		id, tg, name, "BOT-"+itoa(suffix), "BOTREF"+itoa(suffix),
	)
	if err != nil {
		h.t.Fatalf("seed bot: %v", err)
	}
	if _, err := h.db.Exec(`INSERT INTO wallets (user_id, balance) VALUES ($1, 0)`, id); err != nil {
		h.t.Fatalf("seed bot wallet: %v", err)
	}
	h.ids.users = append(h.ids.users, id)
	return id
}

// seedDrawingGame inserts a game already in DRAWING with the given prize pool.
func (h *harness) seedDrawingGame(prizePool float64) uuid.UUID {
	h.t.Helper()
	id := uuid.New()
	_, err := h.db.Exec(
		`INSERT INTO games (id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut, started_at)
		 VALUES ($1,'REGULAR','DRAWING',10,2,0,$2,0, now())`,
		id, prizePool,
	)
	if err != nil {
		h.t.Fatalf("seed game: %v", err)
	}
	h.ids.games = append(h.ids.games, id)
	return id
}

// addPlayer joins a card to the game. joinOrder makes joined_at deterministic so
// the earliest joiner is the primary winner / remainder recipient.
func (h *harness) addPlayer(gameID, userID uuid.UUID, cardID, joinOrder int) {
	h.t.Helper()
	joinedAt := time.Now().Add(time.Duration(joinOrder) * time.Second)
	_, err := h.db.Exec(
		`INSERT INTO game_players (game_id, user_id, card_id, is_eliminated, joined_at)
		 VALUES ($1,$2,$3,false,$4)`,
		gameID, userID, cardID, joinedAt,
	)
	if err != nil {
		h.t.Fatalf("add player: %v", err)
	}
}

// drawTopRow feeds the 5 numbers of a card's top row into Redis, completing that
// card's top-row bingo under auto-daub.
func (h *harness) drawTopRow(gameID uuid.UUID, cardID int) {
	h.t.Helper()
	card := bingo.GenerateCard(cardID)
	for col := 0; col < 5; col++ {
		n := card.Numbers[0][col]
		err := h.uc.redisService.AddDrawnNumber(context.Background(), gameID, domain.DrawnNumber{
			Number: n, DrawnAt: time.Now(),
		})
		if err != nil {
			h.t.Fatalf("add drawn number: %v", err)
		}
	}
}

func (h *harness) drawSequenceUntilAutoBingo(ctx context.Context, gameID uuid.UUID, game *domain.Game, numbers []int) (int, bool) {
	h.t.Helper()
	for i, n := range numbers {
		err := h.uc.redisService.AddDrawnNumber(ctx, gameID, domain.DrawnNumber{
			Letter:  domain.BingoLetter(bingo.GetLetterForNumber(n)),
			Number:  n,
			DrawnAt: time.Now(),
		})
		if err != nil {
			h.t.Fatalf("add drawn number: %v", err)
		}
		if h.uc.checkAutoBingo(ctx, gameID, game) {
			return i, true
		}
	}
	return -1, false
}

func (h *harness) balance(userID uuid.UUID) float64 {
	h.t.Helper()
	var b float64
	if err := h.db.QueryRow(`SELECT balance::float8 FROM wallets WHERE user_id=$1`, userID).Scan(&b); err != nil {
		h.t.Fatalf("read balance: %v", err)
	}
	return b
}

func (h *harness) cleanup() {
	ctx := context.Background()
	// Let the async "spawn next game" goroutine settle, then remove any empty
	// WAITING REGULAR game it may have created, plus all seeded rows.
	time.Sleep(600 * time.Millisecond)
	for _, u := range h.ids.users {
		h.db.Exec(`DELETE FROM transactions WHERE user_id=$1`, u)
	}
	for _, g := range h.ids.games {
		h.uc.redisService.PublishEvent(ctx, g, "cleanup", map[string]any{}) // no-op; keeps import honest
		h.db.Exec(`DELETE FROM game_players WHERE game_id=$1`, g)
		h.db.Exec(`DELETE FROM games WHERE id=$1`, g)
	}
	h.db.Exec(`DELETE FROM game_players WHERE user_id = ANY($1)`, pqUUIDs(h.ids.users))
	for _, u := range h.ids.users {
		h.db.Exec(`DELETE FROM wallets WHERE user_id=$1`, u)
		h.db.Exec(`DELETE FROM users WHERE id=$1`, u)
	}
	// Best-effort: drop the freshly spawned empty next game.
	h.db.Exec(`DELETE FROM games WHERE game_type='REGULAR' AND state='WAITING' AND player_count=0 AND created_at > now() - interval '30 seconds'`)
	h.db.Close()
}

func TestIntegration_AutoBingo_SingleWinner(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("Solo", 1)
	gameID := h.seedDrawingGame(18)
	h.addPlayer(gameID, user, 1, 0)

	// Draw exactly card 1's top row → it auto-completes.
	h.drawTopRow(gameID, 1)

	game, err := h.uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		t.Fatalf("find game: %v", err)
	}
	if !h.uc.checkAutoBingo(ctx, gameID, game) {
		t.Fatalf("expected auto-bingo to resolve the game")
	}

	// Game finished, winner recorded, full pot paid, card flagged.
	g2, _ := h.uc.gameRepo.FindByID(ctx, gameID)
	if g2.State != domain.GameStateFinished {
		t.Fatalf("expected FINISHED, got %s", g2.State)
	}
	if g2.WinnerID == nil || *g2.WinnerID != user {
		t.Fatalf("winner_id mismatch: %v", g2.WinnerID)
	}
	if got := h.balance(user); got != 18 {
		t.Fatalf("expected full pot 18 credited, got %v", got)
	}
	var isWinner bool
	var prizeWon float64
	h.db.QueryRow(`SELECT is_winner, prize_won::float8 FROM game_players WHERE game_id=$1 AND user_id=$2`, gameID, user).Scan(&isWinner, &prizeWon)
	if !isWinner || prizeWon != 18 {
		t.Fatalf("expected card flagged winner with prize_won=18, got is_winner=%v prize_won=%v", isWinner, prizeWon)
	}
	t.Logf("single-winner OK: FINISHED, %s paid 18, card flagged", user)
}

func TestIntegration_AutoBingo_FixedDrawSequenceBotWinsFirst(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	bot := h.seedBot("FixedDrawBot", 9201)
	human := h.seedUser("FixedDrawHuman", 9202)
	gameID := h.seedDrawingGame(18)
	h.addPlayer(gameID, bot, 1, 0)
	h.addPlayer(gameID, human, 2, 1)

	botCard := bingo.GenerateCard(1)
	fixedDraw := []int{
		botCard.Numbers[0][0],
		botCard.Numbers[0][1],
		botCard.Numbers[0][2],
		botCard.Numbers[0][3],
		botCard.Numbers[0][4],
	}

	game, err := h.uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		t.Fatalf("find game: %v", err)
	}
	resolvedAt, resolved := h.drawSequenceUntilAutoBingo(ctx, gameID, game, fixedDraw)
	if !resolved {
		t.Fatalf("expected fixed draw sequence to auto-resolve the game")
	}
	if resolvedAt != len(fixedDraw)-1 {
		t.Fatalf("auto-bingo resolved after draw %d, want %d", resolvedAt+1, len(fixedDraw))
	}

	g2, err := h.uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		t.Fatalf("find resolved game: %v", err)
	}
	if g2.State != domain.GameStateFinished {
		t.Fatalf("expected FINISHED, got %s", g2.State)
	}
	if g2.WinnerID == nil || *g2.WinnerID != bot {
		t.Fatalf("expected bot winner %s, got %v", bot, g2.WinnerID)
	}
	if got := h.balance(bot); got != 18 {
		t.Fatalf("expected bot to receive full pot 18, got %v", got)
	}
	if got := h.balance(human); got != 0 {
		t.Fatalf("expected human balance to remain 0, got %v", got)
	}

	var botWinner, humanWinner bool
	var botPrize, humanPrize float64
	if err := h.db.QueryRow(
		`SELECT is_winner, prize_won::float8 FROM game_players WHERE game_id=$1 AND user_id=$2`,
		gameID, bot,
	).Scan(&botWinner, &botPrize); err != nil {
		t.Fatalf("read bot card outcome: %v", err)
	}
	if err := h.db.QueryRow(
		`SELECT is_winner, prize_won::float8 FROM game_players WHERE game_id=$1 AND user_id=$2`,
		gameID, human,
	).Scan(&humanWinner, &humanPrize); err != nil {
		t.Fatalf("read human card outcome: %v", err)
	}
	if !botWinner || botPrize != 18 {
		t.Fatalf("expected bot card flagged winner with prize_won=18, got is_winner=%v prize_won=%v", botWinner, botPrize)
	}
	if humanWinner || humanPrize != 0 {
		t.Fatalf("expected human card not to win, got is_winner=%v prize_won=%v", humanWinner, humanPrize)
	}
}

func TestIntegration_AutoBingo_SplitPot(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	userA := h.seedUser("Early", 2) // joins first → primary winner
	userB := h.seedUser("Late", 3)
	gameID := h.seedDrawingGame(18)
	h.addPlayer(gameID, userA, 1, 0)
	h.addPlayer(gameID, userB, 2, 1)

	// Both cards' top rows are drawn on the same set → both complete → split.
	h.drawTopRow(gameID, 1)
	h.drawTopRow(gameID, 2)

	game, _ := h.uc.gameRepo.FindByID(ctx, gameID)
	if !h.uc.checkAutoBingo(ctx, gameID, game) {
		t.Fatalf("expected auto-bingo to resolve the game")
	}

	g2, _ := h.uc.gameRepo.FindByID(ctx, gameID)
	if g2.State != domain.GameStateFinished {
		t.Fatalf("expected FINISHED, got %s", g2.State)
	}
	// Primary winner (earliest joiner) recorded on the game row.
	if g2.WinnerID == nil || *g2.WinnerID != userA {
		t.Fatalf("expected primary winner %s, got %v", userA, g2.WinnerID)
	}
	// Pot split 9/9, summing to exactly the pool.
	ba, bb := h.balance(userA), h.balance(userB)
	if ba != 9 || bb != 9 {
		t.Fatalf("expected 9/9 split, got A=%v B=%v", ba, bb)
	}
	if ba+bb != 18 {
		t.Fatalf("split must sum to pool 18, got %v", ba+bb)
	}
	// Both cards flagged winners.
	var winners int
	h.db.QueryRow(`SELECT count(*) FROM game_players WHERE game_id=$1 AND is_winner=true`, gameID).Scan(&winners)
	if winners != 2 {
		t.Fatalf("expected 2 winning cards, got %d", winners)
	}
	t.Logf("split-pot OK: FINISHED, primary=%s, A=%v B=%v (sum %v), 2 cards flagged", userA, ba, bb, ba+bb)
}

// --- tiny helpers (avoid extra imports) ---

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func pqUUIDs(ids []uuid.UUID) interface{} {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	return "{" + join(out, ",") + "}"
}

func join(xs []string, sep string) string {
	s := ""
	for i, x := range xs {
		if i > 0 {
			s += sep
		}
		s += x
	}
	return s
}
