package usecase

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// The sweeper used to fill the instant a real player appeared, so a player
// watched five strangers arrive within a second of picking their card. These
// pin the hold-off: how long, that it varies per game, and that it does not
// wobble between sweeps of the SAME game (which would let a game slip in
// early or stall depending on which tick observed it).

func delayUC(d time.Duration) *BotUseCase {
	return &BotUseCase{settings: BotSettings{JoinDelay: d}}
}

func TestJoinDelayWithinExpectedRange(t *testing.T) {
	const base = 12 * time.Second
	uc := delayUC(base)

	for i := 0; i < 500; i++ {
		got := uc.joinDelayFor(uuid.New())
		if got < base {
			t.Fatalf("delay %v is below the configured minimum %v", got, base)
		}
		if got >= base+base/2 {
			t.Fatalf("delay %v reaches or exceeds the 1.5x ceiling %v", got, base+base/2)
		}
	}
}

// Same game, same answer — otherwise the hold-off would shift on every tick.
func TestJoinDelayIsStablePerGame(t *testing.T) {
	uc := delayUC(12 * time.Second)
	id := uuid.New()
	first := uc.joinDelayFor(id)
	for i := 0; i < 50; i++ {
		if got := uc.joinDelayFor(id); got != first {
			t.Fatalf("delay for the same game changed: %v then %v", first, got)
		}
	}
}

// A fixed delay would itself be the pattern players notice, so different games
// must genuinely differ.
func TestJoinDelayVariesAcrossGames(t *testing.T) {
	uc := delayUC(12 * time.Second)
	seen := map[time.Duration]bool{}
	for i := 0; i < 200; i++ {
		seen[uc.joinDelayFor(uuid.New())] = true
	}
	if len(seen) < 20 {
		t.Fatalf("only %d distinct delays across 200 games — jitter is too coarse to hide the pattern", len(seen))
	}
}

// Zero disables the hold-off entirely (previous behaviour), so an operator can
// turn it off from the environment.
func TestJoinDelayZeroDisables(t *testing.T) {
	for _, d := range []time.Duration{0, -1 * time.Second} {
		if got := delayUC(d).joinDelayFor(uuid.New()); got != 0 {
			t.Fatalf("JoinDelay %v should disable the hold-off, got %v", d, got)
		}
	}
}
