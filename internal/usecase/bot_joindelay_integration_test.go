//go:build integration

package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/bingo/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

// SecondsSinceFirstRealPlayer decides whether bots are allowed to start
// arriving, so getting it wrong either strands a player alone in an empty room
// or lets the bots pour in immediately — the thing the hold-off exists to stop.
// The subtleties worth pinning: it must ignore BOTS (or the first bot to join
// would reset the clock and the rest would follow instantly), ignore players
// who LEFT, and take the EARLIEST joiner rather than the latest.
func TestIntegration_SecondsSinceFirstRealPlayer(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	repo := postgres.NewBotRepository(h.db)

	gameID := h.seedWaitingGame()

	// No players at all yet.
	if _, hasReal, err := repo.SecondsSinceFirstRealPlayer(ctx, gameID); err != nil {
		t.Fatalf("empty game: %v", err)
	} else if hasReal {
		t.Fatal("empty game reported a real player")
	}

	// A bot alone must NOT count — otherwise the hold-off would be satisfied
	// by the very bots it is meant to delay.
	botID := uuid.New()
	if _, err := h.db.Exec(
		`INSERT INTO users (id, telegram_id, first_name, phone_number, referal_code, role, is_bot)
		 VALUES ($1,$2,'DelayBot','BOT-99000001','BOTREF99','user',true)`,
		botID, botTelegramIDBase-99000001,
	); err != nil {
		t.Fatalf("seed bot: %v", err)
	}
	defer h.db.Exec(`DELETE FROM users WHERE id=$1`, botID)
	h.addPlayer(gameID, botID, 401, 1)

	if _, hasReal, err := repo.SecondsSinceFirstRealPlayer(ctx, gameID); err != nil {
		t.Fatalf("bot-only game: %v", err)
	} else if hasReal {
		t.Fatal("a bot satisfied the real-player check — bots would unblock their own hold-off")
	}

	// A real player joins. addPlayer dates joins joinOrder seconds into the
	// FUTURE to control ordering, so pin joined_at to the app's clock here —
	// that is what the production insert path writes, and what the query
	// compares against.
	userID := h.seedUser("DelayReal", 8801)
	h.addPlayer(gameID, userID, 402, 2)
	if _, err := h.db.Exec(
		`UPDATE game_players SET joined_at = $3 WHERE game_id=$1 AND user_id=$2`,
		gameID, userID, time.Now(),
	); err != nil {
		t.Fatalf("pin joined_at: %v", err)
	}

	age, hasReal, err := repo.SecondsSinceFirstRealPlayer(ctx, gameID)
	if err != nil {
		t.Fatalf("with a real player: %v", err)
	}
	if !hasReal {
		t.Fatal("real player was not detected")
	}
	if age < 0 || age > 30 {
		t.Fatalf("age %.1fs is implausible for a player who just joined", age)
	}

	// Backdate that join: the hold-off should now be considered elapsed.
	if _, err := h.db.Exec(
		`UPDATE game_players SET joined_at = now() - interval '10 minutes'
		 WHERE game_id=$1 AND user_id=$2`, gameID, userID,
	); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	age, _, err = repo.SecondsSinceFirstRealPlayer(ctx, gameID)
	if err != nil {
		t.Fatalf("after backdating: %v", err)
	}
	if age < 500 {
		t.Fatalf("age %.1fs, expected ~600 after backdating 10 minutes", age)
	}

	// A second, later real player must not move the clock forward — the
	// earliest joiner is what the hold-off is measured from.
	user2 := h.seedUser("DelayReal2", 8802)
	h.addPlayer(gameID, user2, 403, 3)
	age2, _, err := repo.SecondsSinceFirstRealPlayer(ctx, gameID)
	if err != nil {
		t.Fatalf("with a second player: %v", err)
	}
	if age2 < 500 {
		t.Fatalf("a later joiner reset the clock to %.1fs — must track the EARLIEST real player", age2)
	}

	// A player who left is no longer active and must be ignored.
	if _, err := h.db.Exec(
		`UPDATE game_players SET left_at = now() WHERE game_id=$1 AND user_id=$2`, gameID, userID,
	); err != nil {
		t.Fatalf("mark left: %v", err)
	}
	age3, hasReal, err := repo.SecondsSinceFirstRealPlayer(ctx, gameID)
	if err != nil {
		t.Fatalf("after leave: %v", err)
	}
	if !hasReal {
		t.Fatal("the remaining real player should still count")
	}
	if age3 > 500 {
		t.Fatalf("age %.1fs still reflects the departed player — left_at is not being honoured", age3)
	}
}
