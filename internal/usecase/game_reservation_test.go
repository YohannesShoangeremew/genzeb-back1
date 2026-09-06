//go:build integration

// Integration tests for the reservation model: picking a card only reserves it
// (no charge); the stake is charged for everyone when the countdown ends and the
// game starts. Run against the local dev DB/Redis, same as game_integration_test:
//
//	DB_HOST=127.0.0.1 DB_USER=postgres DB_PASSWORD=... DB_NAME=bingo \
//	  go test -tags=integration -run Reservation ./internal/usecase/ -v
package usecase

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

func (h *harness) seedWaitingGame() uuid.UUID {
	return h.seedWaitingGameType(domain.GameTypeRegular, 10)
}

func (h *harness) seedWaitingGameType(gameType domain.GameType, betAmount float64) uuid.UUID {
	h.t.Helper()
	id := uuid.New()
	_, err := h.db.Exec(
		`INSERT INTO games (id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut)
		 VALUES ($1,$2,'WAITING',$3,2,0,0,0.2)`, id, gameType, betAmount)
	if err != nil {
		h.t.Fatalf("seed waiting game: %v", err)
	}
	h.ids.games = append(h.ids.games, id)
	return id
}

// A player can hold several cards in one game, but not participate in REGULAR
// and VIP games simultaneously. Leaving the first game releases that guard.
func TestIntegration_Reservation_BlocksOtherTierUntilLeave(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("Cross-Tier", 71)
	h.setBalance(user, 200)
	regularID := h.seedWaitingGameType(domain.GameTypeRegular, 10)
	vipID := h.seedWaitingGameType(domain.GameTypeVIP, 50)

	if _, err := h.uc.JoinGame(ctx, regularID, domain.JoinGameRequest{UserID: user, CardID: 1}); err != nil {
		t.Fatalf("reserve regular card: %v", err)
	}
	if _, err := h.uc.JoinGame(ctx, regularID, domain.JoinGameRequest{UserID: user, CardID: 2}); err != nil {
		t.Fatalf("same-game second card should remain allowed: %v", err)
	}

	if _, err := h.uc.JoinGame(ctx, vipID, domain.JoinGameRequest{UserID: user, CardID: 3}); err == nil {
		t.Fatal("expected VIP reservation to be blocked while REGULAR is active")
	} else if !strings.Contains(err.Error(), "active REGULAR game") {
		t.Fatalf("unexpected cross-tier error: %v", err)
	}
	if _, active := h.cardState(vipID, user, 3); active {
		t.Fatal("blocked VIP card must not be reserved")
	}

	if err := h.uc.LeaveGame(ctx, regularID, domain.LeaveGameRequest{UserID: user}); err != nil {
		t.Fatalf("leave regular game: %v", err)
	}
	if _, err := h.uc.JoinGame(ctx, vipID, domain.JoinGameRequest{UserID: user, CardID: 3}); err != nil {
		t.Fatalf("reserve VIP after leaving REGULAR: %v", err)
	}
}

// The per-user wallet lock must make simultaneous cross-tier reservations
// deterministic: exactly one tier wins and the other is rejected.
func TestIntegration_Reservation_ConcurrentCrossTierAllowsExactlyOne(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("Concurrent-Tier", 72)
	h.setBalance(user, 200)
	regularID := h.seedWaitingGameType(domain.GameTypeRegular, 10)
	vipID := h.seedWaitingGameType(domain.GameTypeVIP, 50)

	type result struct {
		gameID uuid.UUID
		err    error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i, gameID := range []uuid.UUID{regularID, vipID} {
		wg.Add(1)
		go func(cardID int, gameID uuid.UUID) {
			defer wg.Done()
			<-start
			_, err := h.uc.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: user, CardID: cardID})
			results <- result{gameID: gameID, err: err}
		}(i+10, gameID)
	}
	close(start)
	wg.Wait()
	close(results)

	succeeded := 0
	failed := 0
	for got := range results {
		if got.err == nil {
			succeeded++
			continue
		}
		failed++
		if !strings.Contains(got.err.Error(), "active ") {
			t.Fatalf("unexpected join error for %s: %v", got.gameID, got.err)
		}
	}
	if succeeded != 1 || failed != 1 {
		t.Fatalf("want one success and one rejection, got successes=%d failures=%d", succeeded, failed)
	}
}

func (h *harness) setBalance(userID uuid.UUID, bal float64) {
	h.t.Helper()
	if _, err := h.db.Exec(`UPDATE wallets SET balance=$2 WHERE user_id=$1`, userID, bal); err != nil {
		h.t.Fatalf("set balance: %v", err)
	}
}

// addReservation inserts an UNPAID card row directly (bypassing JoinGame), so
// the commit path can be tested in isolation from the async countdown goroutine.
func (h *harness) addReservation(gameID, userID uuid.UUID, cardID, order int) {
	h.t.Helper()
	joinedAt := time.Now().Add(time.Duration(order) * time.Second)
	if _, err := h.db.Exec(
		`INSERT INTO game_players (id, game_id, user_id, card_id, paid, is_eliminated, joined_at)
		 VALUES ($1,$2,$3,$4,false,false,$5)`,
		uuid.New(), gameID, userID, cardID, joinedAt); err != nil {
		h.t.Fatalf("add reservation: %v", err)
	}
}

func (h *harness) forceCountdown(gameID uuid.UUID, playerCount int, prize float64) {
	h.t.Helper()
	if _, err := h.db.Exec(
		`UPDATE games SET state='COUNTDOWN', player_count=$2, prize_pool=$3,
		 countdown_ends = now() + interval '40 seconds' WHERE id=$1`,
		gameID, playerCount, prize); err != nil {
		h.t.Fatalf("force countdown: %v", err)
	}
}

// cardState returns (paid, active) for a specific card row.
func (h *harness) cardState(gameID, userID uuid.UUID, cardID int) (paid bool, active bool) {
	h.t.Helper()
	var leftAt sql.NullTime
	err := h.db.QueryRow(
		`SELECT paid, left_at FROM game_players WHERE game_id=$1 AND user_id=$2 AND card_id=$3`,
		gameID, userID, cardID).Scan(&paid, &leftAt)
	if err != nil {
		return false, false
	}
	return paid, !leftAt.Valid
}

func (h *harness) gameState(gameID uuid.UUID) *domain.Game {
	h.t.Helper()
	g, err := h.uc.gameRepo.FindByID(context.Background(), gameID)
	if err != nil {
		h.t.Fatalf("find game: %v", err)
	}
	return g
}

// Reserving a card must NOT move any money — the wallet is untouched until the
// countdown ends.
func TestIntegration_Reservation_NoChargeUntilStart(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	user := h.seedUser("Reserver", 51)
	h.setBalance(user, 50)
	gameID := h.seedWaitingGame()

	if _, err := h.uc.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: user, CardID: 1}); err != nil {
		t.Fatalf("reserve card 1: %v", err)
	}
	if _, err := h.uc.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: user, CardID: 2}); err != nil {
		t.Fatalf("reserve card 2: %v", err)
	}

	if b := h.balance(user); b != 50 {
		t.Fatalf("balance changed on reserve: want 50, got %v", b)
	}
	if paid, active := h.cardState(gameID, user, 1); paid || !active {
		t.Fatalf("card 1 should be active+unpaid, got paid=%v active=%v", paid, active)
	}
	if paid, _ := h.cardState(gameID, user, 2); paid {
		t.Fatalf("card 2 should be unpaid")
	}
	t.Log("reserve OK: two cards reserved, wallet still 50")
}

// When the countdown ends, every reserved card is charged and the game starts.
func TestIntegration_Reservation_ChargeAtStart(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	u1 := h.seedUser("Alice", 52)
	u2 := h.seedUser("Bob", 53)
	h.setBalance(u1, 50)
	h.setBalance(u2, 50)
	gameID := h.seedWaitingGame()

	// Two reserved cards; force the pre-start state, then commit.
	h.addReservation(gameID, u1, 10, 0)
	h.addReservation(gameID, u2, 20, 1)
	h.forceCountdown(gameID, 2, 16) // projected pool 2*10*0.8

	h.uc.startDrawing(ctx, gameID)

	g := h.gameState(gameID)
	if g.State != domain.GameStateDrawing {
		t.Fatalf("expected DRAWING, got %s", g.State)
	}
	if b := h.balance(u1); b != 40 {
		t.Fatalf("u1 not charged: want 40, got %v", b)
	}
	if b := h.balance(u2); b != 40 {
		t.Fatalf("u2 not charged: want 40, got %v", b)
	}
	if paid, active := h.cardState(gameID, u1, 10); !paid || !active {
		t.Fatalf("u1 card should be paid+active, got paid=%v active=%v", paid, active)
	}
	if g.PrizePool != 16 {
		t.Fatalf("prize pool: want 16, got %v", g.PrizePool)
	}
	if g.PlayerCount != 2 {
		t.Fatalf("player count: want 2, got %d", g.PlayerCount)
	}
	t.Log("commit OK: both charged 10, DRAWING, pool 16")
}

// Full flow through the REAL use case: two players reserve (no charge), the
// countdown ends (everyone charged, game starts), the winner's line is drawn,
// auto-bingo resolves the game, and the pot is paid out. Asserts wallet balances
// at every step.
func TestIntegration_Reservation_EndToEnd(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	winner := h.seedUser("E2E-Win", 61)
	loser := h.seedUser("E2E-Lose", 62)
	h.setBalance(winner, 50)
	h.setBalance(loser, 50)
	gameID := h.seedWaitingGame() // REGULAR, bet 10, house_cut 0.2

	// 1) Both players RESERVE a card via the real join path — no charge yet.
	if _, err := h.uc.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: winner, CardID: 1}); err != nil {
		t.Fatalf("winner reserve: %v", err)
	}
	if _, err := h.uc.JoinGame(ctx, gameID, domain.JoinGameRequest{UserID: loser, CardID: 2}); err != nil {
		t.Fatalf("loser reserve: %v", err)
	}
	if h.balance(winner) != 50 || h.balance(loser) != 50 {
		t.Fatalf("reserve must not charge: winner=%v loser=%v", h.balance(winner), h.balance(loser))
	}
	if g := h.gameState(gameID); g.PlayerCount != 2 {
		t.Fatalf("expected 2 players after reservations, got %d", g.PlayerCount)
	}
	t.Log("step 1 OK: both reserved, no charge, 2 players")

	// 2) Countdown ends → commit charges everyone and drawing starts.
	h.forceCountdown(gameID, 2, 16)
	h.uc.startDrawing(ctx, gameID)
	if h.balance(winner) != 40 || h.balance(loser) != 40 {
		t.Fatalf("commit must charge each 10: winner=%v loser=%v", h.balance(winner), h.balance(loser))
	}
	g := h.gameState(gameID)
	if g.State != domain.GameStateDrawing {
		t.Fatalf("expected DRAWING, got %s", g.State)
	}
	if g.PrizePool != 16 {
		t.Fatalf("prize pool want 16 got %v", g.PrizePool)
	}
	t.Log("step 2 OK: both charged 10, DRAWING, pool 16")

	// 3) Draw the winner's top row → auto-bingo resolves the game.
	h.drawTopRow(gameID, 1)
	if !h.uc.checkAutoBingo(ctx, gameID, h.gameState(gameID)) {
		t.Fatalf("expected auto-bingo to resolve")
	}

	// 4) Winner paid the full pot; loser stays down their stake.
	gf := h.gameState(gameID)
	if gf.State != domain.GameStateFinished {
		t.Fatalf("expected FINISHED, got %s", gf.State)
	}
	if b := h.balance(winner); b != 56 { // 50 - 10 stake + 16 pot
		t.Fatalf("winner balance want 56 got %v", b)
	}
	if b := h.balance(loser); b != 40 { // 50 - 10 stake
		t.Fatalf("loser balance want 40 got %v", b)
	}
	t.Log("E2E OK: reserve(0) -> charge(10 each) -> draw -> winner +16 (bal 56), loser 40")
}

// A reserver who can no longer cover their cards at commit is dropped without a
// charge; if that leaves too few players the game reverts to WAITING.
func TestIntegration_Reservation_DropUnfundedAtStart(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	u1 := h.seedUser("Payer", 54)
	u2 := h.seedUser("Broke", 55)
	h.setBalance(u1, 50)
	h.setBalance(u2, 50)
	gameID := h.seedWaitingGame()

	h.addReservation(gameID, u1, 11, 0)
	h.addReservation(gameID, u2, 21, 1)
	// Bob spent his money between reserving and the countdown ending.
	h.setBalance(u2, 5)
	h.forceCountdown(gameID, 2, 16)

	h.uc.startDrawing(ctx, gameID)

	g := h.gameState(gameID)
	if b := h.balance(u1); b != 40 {
		t.Fatalf("u1 should be charged 10: want 40, got %v", b)
	}
	if b := h.balance(u2); b != 5 {
		t.Fatalf("u2 should NOT be charged: want 5, got %v", b)
	}
	if _, active := h.cardState(gameID, u2, 21); active {
		t.Fatalf("u2 unfunded card should be dropped")
	}
	if paid, active := h.cardState(gameID, u1, 11); !paid || !active {
		t.Fatalf("u1 card should be paid+active, got paid=%v active=%v", paid, active)
	}
	// Only one paying player remains → cannot start.
	if g.State != domain.GameStateWaiting {
		t.Fatalf("expected revert to WAITING, got %s", g.State)
	}
	if g.PlayerCount != 1 {
		t.Fatalf("player count: want 1, got %d", g.PlayerCount)
	}
	t.Log("drop OK: unfunded card released, no charge, reverted to WAITING")
}
