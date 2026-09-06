//go:build integration

package usecase

import (
	"context"
	"testing"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/repository/postgres"
)

// botUC builds a BotUseCase wired to the same live Postgres + Redis the game
// harness uses, so the auto-sweeper runs against real JoinGame/countdown logic.
// minimumRoomPlayers is how full a game should get; the pool is seeded a little larger
// so distinct bots are always available.
func (h *harness) botUC(minimumRoomPlayers int) (*BotUseCase, domain.BotRepository) {
	h.t.Helper()
	botRepo := postgres.NewBotRepository(h.db)
	uc := NewBotUseCase(
		botRepo,
		postgres.NewUserRepository(h.db),
		postgres.NewWalletRepository(h.db),
		postgres.NewTransactionRepository(h.db),
		postgres.NewGameRepository(h.db),
		h.uc,              // real GameUseCase → real JoinGame path
		h.uc.redisService, // shared game-state service used by the bot use case
		h.db,
		BotSettings{PoolSize: minimumRoomPlayers + 3, WalletFloat: 1000, MaxJoinsPerTick: 10},
	)
	if err := uc.EnsureBotPool(context.Background(), minimumRoomPlayers+3); err != nil {
		h.t.Fatalf("seed bot pool: %v", err)
	}
	// Enable dynamic staffing, including bot-only rooms.
	if err := botRepo.UpdateConfig(context.Background(), &domain.BotConfig{
		Enabled: true, MinimumRoomPlayers: minimumRoomPlayers, Tiers: "REGULAR",
	}); err != nil {
		h.t.Fatalf("set bot config: %v", err)
	}
	return uc, botRepo
}

// Empty rooms are now staffed directly: zero real players means the configured
// minimum is supplied entirely by bots, without requiring lobby browse state.
func TestIntegration_Ambient_FillsEmptyRoomWithoutBrowse(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	uc, botRepo := h.botUC(3)
	gameID := h.seedWaitingGame()
	uc.sweep(ctx)

	got, err := botRepo.CountBotsInGame(ctx, gameID)
	if err != nil {
		t.Fatalf("count bots: %v", err)
	}
	if got != 3 {
		t.Fatalf("expected 3 bots in an empty room, got %d", got)
	}
}

// Large gaps are filled in bounded batches so a room does not receive dozens of
// automated joins in one sweep.
func TestIntegration_Ambient_RespectsPerTickJoinLimit(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	uc, botRepo := h.botUC(50)
	gameID := h.seedWaitingGame()
	uc.sweep(ctx)

	got, err := botRepo.CountBotsInGame(ctx, gameID)
	if err != nil {
		t.Fatalf("count bots: %v", err)
	}
	if got != 10 {
		t.Fatalf("expected one capped batch of 10 bots, got %d", got)
	}
}

// FillGame (the admin "add bots" button) must KEEP the classic guard: it never
// seeds a game with zero real players. Only the automatic sweeper opts into
// bot-only games.
func TestIntegration_Ambient_ManualFillStillGuarded(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	uc, botRepo := h.botUC(3)
	gameID := h.seedWaitingGame()

	res, err := uc.FillGame(ctx, gameID, 3)
	if err != nil {
		t.Fatalf("FillGame: %v", err)
	}
	if res.Added != 0 {
		t.Fatalf("FillGame must not seed a zero-real game, added %d", res.Added)
	}
	got, _ := botRepo.CountBotsInGame(ctx, gameID)
	if got != 0 {
		t.Fatalf("expected 0 bots after guarded FillGame, got %d", got)
	}
}
