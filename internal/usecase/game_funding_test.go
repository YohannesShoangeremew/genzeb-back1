package usecase

import (
	"testing"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

func TestSummarizeGameFundingCountsDistinctRealPaidPlayers(t *testing.T) {
	walletUser := uuid.New()
	bonusUser := uuid.New()
	mixedUser := uuid.New()
	leftAt := time.Now()

	players := []*domain.GamePlayer{
		{UserID: walletUser, Paid: true},
		{UserID: walletUser, Paid: true}, // a second wallet-funded card still counts once
		{UserID: bonusUser, Paid: true, PaidFromBonus: true},
		{UserID: mixedUser, Paid: true},
		{UserID: mixedUser, Paid: true, PaidFromBonus: true},
		{UserID: uuid.New(), Paid: true, IsBot: true},
		{UserID: uuid.New(), Paid: false},
		{UserID: uuid.New(), Paid: true, LeftAt: &leftAt},
	}

	got := summarizeGameFunding(players)
	if got.TotalPlayers != 3 || got.WalletPlayers != 2 || got.BonusPlayers != 2 || got.MixedPlayers != 1 {
		t.Fatalf("unexpected funding stats: %+v", got)
	}
}
