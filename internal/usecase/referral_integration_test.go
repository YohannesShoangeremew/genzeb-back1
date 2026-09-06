//go:build integration

package usecase

import (
	"context"
	"testing"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/repository/postgres"
	"github.com/google/uuid"
)

func (h *harness) userUC() *UserUseCase {
	h.t.Helper()
	return NewUserUseCase(
		postgres.NewUserRepository(h.db),
		postgres.NewWalletRepository(h.db),
		postgres.NewBonusRepository(h.db),
		h.db,
	)
}

// Signup records the referral but pays nothing. The referrer receives one
// PLAY-ONLY reward only after the invited player completes a real deposit.
func TestIntegration_Referral_PaidAfterFirstRealDeposit(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	uc := h.userUC()

	var oldEnabled bool
	var oldAmount float64
	if err := h.db.QueryRow(`SELECT referral_enabled, referral_amount::float8 FROM app_settings WHERE id=1`).Scan(&oldEnabled, &oldAmount); err != nil {
		t.Skipf("referral settings migration not applied: %v", err)
	}
	defer h.db.Exec(`UPDATE app_settings SET referral_enabled=$1, referral_amount=$2 WHERE id=1`, oldEnabled, oldAmount)
	if _, err := h.db.Exec(`UPDATE app_settings SET referral_enabled=true, referral_amount=$1 WHERE id=1`, domain.ReferralRewardAmount); err != nil {
		t.Fatalf("enable referral reward: %v", err)
	}

	referrer, _, err := uc.CreateUser(ctx, domain.CreateUserRequest{
		TelegramID: 991000001, FirstName: "Referrer", Phone: "0911990001",
	})
	if err != nil {
		t.Fatalf("create referrer: %v", err)
	}
	h.ids.users = append(h.ids.users, referrer.ID)

	invited, _, err := uc.CreateUser(ctx, domain.CreateUserRequest{
		TelegramID: 991000002, FirstName: "Invited", Phone: "0911990002",
		ReferrerCode: referrer.ReferalCode,
	})
	if err != nil {
		t.Fatalf("create invited: %v", err)
	}
	h.ids.users = append(h.ids.users, invited.ID)

	if got := h.bonusBalance(referrer.ID); got != domain.DefaultUserBalance {
		t.Fatalf("referrer bonus at signup = %.2f, want welcome bonus %.2f only", got, domain.DefaultUserBalance)
	}
	if invited.ReferredBy == nil || *invited.ReferredBy != referrer.ID {
		t.Fatalf("invited.referred_by not set to referrer")
	}
	var rewarded bool
	if err := h.db.QueryRow(`SELECT referral_rewarded FROM users WHERE id=$1`, invited.ID).Scan(&rewarded); err != nil {
		t.Fatalf("read referral_rewarded: %v", err)
	}
	if rewarded {
		t.Fatal("referral was marked rewarded before a deposit")
	}

	svc := postgres.NewTransactionService(
		h.db,
		postgres.NewWalletRepository(h.db),
		postgres.NewTransactionRepository(h.db),
		postgres.NewBonusRepository(h.db),
	)
	approveRealDeposit := func(amount float64) uuid.UUID {
		depositID := uuid.New()
		if _, err := h.db.Exec(`
			INSERT INTO transactions (id, user_id, type, category, amount, status, transaction_type, transaction_id)
			VALUES ($1,$2,'deposit','deposit',$3,'pending',$4,$5)
		`, depositID, invited.ID, amount, string(domain.PaymentMethodTelebirr), "REFERRAL-"+depositID.String()); err != nil {
			t.Fatalf("seed pending deposit: %v", err)
		}
		if _, err := svc.ApproveDeposit(ctx, depositID); err != nil {
			t.Fatalf("approve deposit: %v", err)
		}
		return depositID
	}

	firstDepositID := approveRealDeposit(100)
	wantBonus := domain.DefaultUserBalance + domain.ReferralRewardAmount
	if got := h.bonusBalance(referrer.ID); got != wantBonus {
		t.Fatalf("referrer bonus after first deposit = %.2f, want %.2f", got, wantBonus)
	}
	if got := h.balance(referrer.ID); got != 0 {
		t.Fatalf("referrer withdrawable balance = %.2f, want 0", got)
	}
	if err := h.db.QueryRow(`SELECT referral_rewarded FROM users WHERE id=$1`, invited.ID).Scan(&rewarded); err != nil || !rewarded {
		t.Fatalf("referral_rewarded after deposit = %v (err=%v), want true", rewarded, err)
	}

	if _, err := svc.ApproveDeposit(ctx, firstDepositID); err == nil {
		t.Fatal("retry unexpectedly approved the same deposit")
	}
	approveRealDeposit(50)
	if got := h.bonusBalance(referrer.ID); got != wantBonus {
		t.Fatalf("referrer bonus after retry/second deposit = %.2f, want one reward %.2f", got, wantBonus)
	}
}

// No code → no referrer, no reward, no error.
func TestIntegration_Referral_NoCode_NoReward(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	uc := h.userUC()

	solo, _, err := uc.CreateUser(ctx, domain.CreateUserRequest{
		TelegramID: 991000003, FirstName: "Solo", Phone: "0911990003",
	})
	if err != nil {
		t.Fatalf("create solo: %v", err)
	}
	h.ids.users = append(h.ids.users, solo.ID)

	if solo.ReferredBy != nil {
		t.Fatalf("solo should have no referrer")
	}
	// Welcome credit is play-only bonus now: 0 cash, DefaultUserBalance bonus.
	if got := h.balance(solo.ID); got != 0 {
		t.Fatalf("solo real balance = %.2f, want 0", got)
	}
	if got := h.bonusBalance(solo.ID); got != domain.DefaultUserBalance {
		t.Fatalf("solo bonus balance = %.2f, want %.2f (welcome)", got, domain.DefaultUserBalance)
	}
}
