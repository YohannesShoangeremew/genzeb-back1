//go:build integration

package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/bingo/backend/internal/domain"
)

// The draw lease must have exactly one owner: while A holds it, B cannot; A can
// renew its own; once A releases, B can take over. This is what stops two
// overlapping instances (old + new during a deploy) from both drawing a game.
func TestIntegration_DrawLease_SingleOwner(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	rs := h.uc.redisService
	gid := h.seedDrawingGame(10)
	ttl := 5 * time.Second

	if ok, _ := rs.AcquireOrRenewDrawLease(ctx, gid, "A", ttl); !ok {
		t.Fatal("A should acquire a free lease")
	}
	if ok, _ := rs.AcquireOrRenewDrawLease(ctx, gid, "B", ttl); ok {
		t.Fatal("B must NOT be able to steal A's live lease")
	}
	if ok, _ := rs.AcquireOrRenewDrawLease(ctx, gid, "A", ttl); !ok {
		t.Fatal("A should be able to renew its own lease")
	}
	rs.ReleaseDrawLease(ctx, gid, "A")
	if ok, _ := rs.AcquireOrRenewDrawLease(ctx, gid, "B", ttl); !ok {
		t.Fatal("B should acquire the lease once A released it")
	}
	rs.ReleaseDrawLease(ctx, gid, "B")
}

// The deploy scenario: a game is left in DRAWING with some numbers already
// drawn (by the departed process). A fresh drawNumbers — exactly what
// ResumeActiveGames launches on boot — must pick it up and keep drawing rather
// than leave it frozen.
func TestIntegration_ResumeDrawing_ContinuesDraw(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	u1 := h.seedUser("Resume1", 701)
	u2 := h.seedUser("Resume2", 702)
	gameID := h.seedDrawingGame(18)
	h.addPlayer(gameID, u1, 1, 0)
	h.addPlayer(gameID, u2, 2, 1)

	// One number already on the board, as if a prior process had drawn it.
	if err := h.uc.redisService.AddDrawnNumber(ctx, gameID, domain.DrawnNumber{
		Letter: domain.BingoLetter("B"), Number: 1, DrawnAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed drawn number: %v", err)
	}
	before, _ := h.uc.redisService.GetDrawnNumbers(ctx, gameID)

	// Resume, as a freshly started process would.
	go h.uc.drawNumbers(ctx, gameID)

	// Give it the first-draw grace plus a couple of draw ticks.
	time.Sleep(domain.FirstDrawDelay + 2*domain.DrawInterval + 700*time.Millisecond)

	after, _ := h.uc.redisService.GetDrawnNumbers(ctx, gameID)
	if len(after) <= len(before) {
		t.Fatalf("resume drew nothing: before=%d after=%d (game stayed frozen)", len(before), len(after))
	}

	// Stop the loop so it doesn't keep running past the test: leaving DRAWING
	// makes drawNumbers exit on its next tick and release the lease.
	if _, err := h.db.Exec(`UPDATE games SET state='CANCELLED' WHERE id=$1`, gameID); err != nil {
		t.Fatalf("stop game: %v", err)
	}
}
