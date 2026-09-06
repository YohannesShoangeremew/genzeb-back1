//go:build integration

package usecase

import (
	"context"
	"testing"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

// The same approval service completes automatically verified and manually
// approved deposits, so this pins both paths at their shared atomic boundary.
func TestIntegration_DepositBonus_OnlyTelebirrAndCBEBirrOnceCompleted(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()

	var oldEnabled bool
	var oldPercentage float64
	if err := h.db.QueryRow(`SELECT deposit_bonus_enabled, deposit_bonus_percentage::float8 FROM app_settings WHERE id=1`).Scan(&oldEnabled, &oldPercentage); err != nil {
		t.Skipf("deposit bonus migration not applied: %v", err)
	}
	defer h.db.Exec(`UPDATE app_settings SET deposit_bonus_enabled=$1, deposit_bonus_percentage=$2 WHERE id=1`, oldEnabled, oldPercentage)
	if _, err := h.db.Exec(`UPDATE app_settings SET deposit_bonus_enabled=true, deposit_bonus_percentage=50 WHERE id=1`); err != nil {
		t.Fatalf("enable deposit bonus: %v", err)
	}
	svc := postgres.NewTransactionService(
		h.db,
		postgres.NewWalletRepository(h.db),
		postgres.NewTransactionRepository(h.db),
		postgres.NewBonusRepository(h.db),
	)

	cases := []struct {
		name      string
		method    domain.PaymentMethod
		deposit   float64
		wantBonus float64
		suffix    int64
	}{
		{name: "Telebirr", method: domain.PaymentMethodTelebirr, deposit: 100, wantBonus: 50, suffix: 9801},
		{name: "CBE Birr", method: domain.PaymentMethodCBEBirr, deposit: 75.55, wantBonus: 37.78, suffix: 9802},
		{name: "M-Pesa excluded", method: domain.PaymentMethodMpesa, deposit: 100, wantBonus: 0, suffix: 9803},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userID := h.seedUser("DepositBonus"+tc.name, tc.suffix)
			depositID := uuid.New()
			ref := "BONUS-" + depositID.String()
			if _, err := h.db.Exec(`
				INSERT INTO transactions (id, user_id, type, category, amount, status, transaction_type, transaction_id)
				VALUES ($1,$2,'deposit','deposit',$3,'pending',$4,$5)
			`, depositID, userID, tc.deposit, string(tc.method), ref); err != nil {
				t.Fatalf("seed pending deposit: %v", err)
			}

			if _, err := svc.ApproveDeposit(ctx, depositID); err != nil {
				t.Fatalf("approve deposit: %v", err)
			}
			if got := h.balance(userID); got != tc.deposit {
				t.Fatalf("withdrawable balance = %.2f, want %.2f", got, tc.deposit)
			}
			if got := h.bonusBalance(userID); got != tc.wantBonus {
				t.Fatalf("bonus balance = %.2f, want %.2f", got, tc.wantBonus)
			}

			// A retry cannot complete the same deposit or mint a second reward.
			if _, err := svc.ApproveDeposit(ctx, depositID); err == nil {
				t.Fatal("second approval unexpectedly succeeded")
			}
			if got := h.bonusBalance(userID); got != tc.wantBonus {
				t.Fatalf("bonus after retry = %.2f, want %.2f", got, tc.wantBonus)
			}
		})
	}
}
