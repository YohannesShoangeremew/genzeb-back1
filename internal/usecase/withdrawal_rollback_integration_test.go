//go:build integration

package usecase

import (
	"context"
	"testing"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

// pendingWithdrawal inserts a pending withdrawal row (the balance is assumed
// already deducted, as the live Withdraw path does) and returns its id.
func (h *harness) pendingWithdrawal(userID uuid.UUID, amount float64) uuid.UUID {
	h.t.Helper()
	var id uuid.UUID
	err := h.db.QueryRow(
		`INSERT INTO transactions (user_id, type, category, amount, status, reference)
		 VALUES ($1,'withdraw','withdrawal',$2,'pending','test-wd') RETURNING id`,
		userID, amount,
	).Scan(&id)
	if err != nil {
		h.t.Fatalf("insert pending withdrawal: %v", err)
	}
	return id
}

// A farmer: 50 deposited, 0 won, requesting 550 (mostly referral). Rolling back
// returns only the 50 genuine to cash; the other 500 becomes play-only bonus.
func TestIntegration_RejectToBonus_FarmerSplit(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	uc := h.walletUC()

	user := h.seedUser("Farmer", 801)
	h.addCompletedDeposit(user, 50, "DEP") // genuine: 50 deposited, no winnings
	wid := h.pendingWithdrawal(user, 550)
	h.setBalance(user, 0) // balance already drained (withdrawal reserved + bets)

	res, err := uc.RejectWithdrawalToBonus(ctx, wid)
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if res.RealRefunded != 50 || res.BonusGranted != 500 {
		t.Fatalf("split = %.0f cash / %.0f bonus, want 50 / 500", res.RealRefunded, res.BonusGranted)
	}
	if got := h.balance(user); got != 50 {
		t.Fatalf("real balance = %.0f, want 50", got)
	}
	if got := h.bonusBalance(user); got != 500 {
		t.Fatalf("bonus balance = %.0f, want 500", got)
	}
}

// A real winner: 100 deposited, 1632 won, requesting 1350. Rolling back returns
// the whole 1350 to cash (it's all backed by genuine winnings) and 0 to bonus.
func TestIntegration_RejectToBonus_RealWinnerAllCash(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	uc := h.walletUC()

	user := h.seedUser("Winner", 802)
	h.addCompletedDeposit(user, 100, "DEP")
	h.addSystemDeposit(user, domain.TransactionCategoryWinnings, 1632, "WIN")
	wid := h.pendingWithdrawal(user, 1350)
	h.setBalance(user, 2)

	res, err := uc.RejectWithdrawalToBonus(ctx, wid)
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if res.RealRefunded != 1350 || res.BonusGranted != 0 {
		t.Fatalf("split = %.0f cash / %.0f bonus, want 1350 / 0", res.RealRefunded, res.BonusGranted)
	}
	if got := h.balance(user); got != 1352 {
		t.Fatalf("real balance = %.0f, want 1352", got)
	}
	if got := h.bonusBalance(user); got != 0 {
		t.Fatalf("bonus balance = %.0f, want 0", got)
	}
}
