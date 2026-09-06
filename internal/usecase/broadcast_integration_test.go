//go:build integration

package usecase

import (
	"context"
	"testing"

	"github.com/bingo/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

// Who a broadcast targets is the one part of this feature that cannot be
// tested with a fake, and getting it wrong is expensive: filler bots carry
// large NEGATIVE synthetic Telegram ids, so sending to them would fail once
// per bot — hundreds of errors that would swamp the failure count and make a
// healthy run look broken.
func TestIntegration_Broadcast_RecipientsExcludeBotsAndBanned(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	repo := postgres.NewBroadcastRepository(h.db)

	real1 := h.seedUser("CastReal1", 9301)
	real2 := h.seedUser("CastReal2", 9302)
	banned := h.seedUser("CastBanned", 9303)
	if _, err := h.db.Exec(`UPDATE users SET banned = true WHERE id = $1`, banned); err != nil {
		t.Fatalf("ban user: %v", err)
	}

	// A filler bot, with the same negative-id scheme EnsureBotPool uses.
	botID := uuid.New()
	if _, err := h.db.Exec(`
		INSERT INTO users (id, telegram_id, first_name, phone_number, referal_code, role, is_bot)
		VALUES ($1, $2, 'CastBot', 'BOT-99300001', 'CASTREF1', 'user', true)
	`, botID, botTelegramIDBase-99300001); err != nil {
		t.Fatalf("seed bot: %v", err)
	}
	defer h.db.Exec(`DELETE FROM users WHERE id=$1`, botID)

	got, err := repo.Recipients(ctx)
	if err != nil {
		t.Fatalf("recipients: %v", err)
	}

	included := map[uuid.UUID]bool{}
	for _, r := range got {
		included[r.UserID] = true
		if r.TelegramID <= 0 {
			t.Errorf("recipient %s has non-positive telegram id %d — that chat cannot exist", r.UserID, r.TelegramID)
		}
	}

	if !included[real1] || !included[real2] {
		t.Error("a real registered player was left out of the audience")
	}
	if included[botID] {
		t.Error("a filler bot was included — every send to it would fail")
	}
	if included[banned] {
		t.Error("a banned user was included")
	}
}
