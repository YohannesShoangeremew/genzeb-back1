//go:build integration

package usecase

import (
	"context"
	"testing"

	"github.com/bingo/backend/internal/domain"
)

func TestIntegration_WelcomeBonus_AdminToggle(t *testing.T) {
	h := newHarness(t)
	defer h.cleanup()
	ctx := context.Background()
	uc := h.userUC()

	var original bool
	if err := h.db.QueryRow(`SELECT welcome_bonus_enabled FROM app_settings WHERE id = 1`).Scan(&original); err != nil {
		t.Skipf("welcome bonus toggle migration not applied: %v", err)
	}
	defer h.db.Exec(`UPDATE app_settings SET welcome_bonus_enabled=$1 WHERE id=1`, original)

	if _, err := h.db.Exec(`UPDATE app_settings SET welcome_bonus_enabled=false WHERE id=1`); err != nil {
		t.Fatalf("disable welcome bonus: %v", err)
	}
	withoutBonus, _, err := uc.CreateUser(ctx, domain.CreateUserRequest{
		TelegramID: 991000101, FirstName: "Welcome-Off", Phone: "0911990101",
	})
	if err != nil {
		t.Fatalf("create user while off: %v", err)
	}
	h.ids.users = append(h.ids.users, withoutBonus.ID)
	if got := h.bonusBalance(withoutBonus.ID); got != 0 {
		t.Fatalf("welcome bonus while off = %.2f, want 0", got)
	}

	if _, err := h.db.Exec(`UPDATE app_settings SET welcome_bonus_enabled=true WHERE id=1`); err != nil {
		t.Fatalf("enable welcome bonus: %v", err)
	}
	withBonus, _, err := uc.CreateUser(ctx, domain.CreateUserRequest{
		TelegramID: 991000102, FirstName: "Welcome-On", Phone: "0911990102",
	})
	if err != nil {
		t.Fatalf("create user while on: %v", err)
	}
	h.ids.users = append(h.ids.users, withBonus.ID)
	if got := h.bonusBalance(withBonus.ID); got != domain.DefaultUserBalance {
		t.Fatalf("welcome bonus while on = %.2f, want %.2f", got, domain.DefaultUserBalance)
	}
}
