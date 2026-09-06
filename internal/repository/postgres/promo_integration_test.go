//go:build integration

// Integration test for promo-code redemption against a live Postgres:
// full happy path (wallet credited, transaction recorded, count bumped) plus
// every failure verdict (already redeemed, exhausted, inactive, not found).
//
// Run (against the local dev DB):
//
//	DB_HOST=127.0.0.1 DB_USER=postgres DB_PASSWORD=... DB_NAME=bingo \
//	  go test -tags=integration -run Integration ./internal/repository/postgres/ -v
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func TestIntegrationPromoRedeem(t *testing.T) {
	dsn := "host=" + envOr("DB_HOST", "127.0.0.1") +
		" port=" + envOr("DB_PORT", "5432") +
		" user=" + envOr("DB_USER", "postgres") +
		" password=" + envOr("DB_PASSWORD", "postgres") +
		" dbname=" + envOr("DB_NAME", "bingo") +
		" sslmode=" + envOr("DB_SSLMODE", "disable")
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Skipf("no database reachable (%v) — skipping integration test", err)
	}

	ctx := context.Background()
	repo := NewPromoRepository(db, NewWalletRepository(db), NewTransactionRepository(db), NewBonusRepository(db))
	bonusOf := func(uid uuid.UUID) float64 {
		var b float64
		db.QueryRow(`SELECT COALESCE(SUM(remaining),0) FROM bonus_grants WHERE user_id=$1`, uid).Scan(&b)
		return b
	}

	// Seed two throwaway users with wallets, and a capped promo code.
	code := fmt.Sprintf("PRT%d", time.Now().UnixNano()%1e9)
	var users [3]uuid.UUID
	for i := range users {
		users[i] = uuid.New()
		if _, err := db.Exec(`
			INSERT INTO users (id, telegram_id, first_name, phone_number, referal_code)
			VALUES ($1, $2, 'PromoTest', $3, $4)
		`, users[i], -time.Now().UnixNano()%1e15-int64(i), fmt.Sprintf("2519%08d", time.Now().UnixNano()%1e8+int64(i)), "PRTEST"+uuid.NewString()[:8]); err != nil {
			t.Fatalf("seed user: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO wallets (user_id, balance) VALUES ($1, 0)`, users[i]); err != nil {
			t.Fatalf("seed wallet: %v", err)
		}
	}
	if _, err := db.Exec(`
		INSERT INTO promo_codes (code, bonus_amount, max_redemptions) VALUES ($1, 50, 2)
	`, code); err != nil {
		t.Fatalf("seed promo: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM promo_redemptions WHERE code = $1`, code)
		db.Exec(`DELETE FROM promo_codes WHERE code = $1`, code)
		for _, u := range users {
			db.Exec(`DELETE FROM transactions WHERE user_id = $1`, u)
			db.Exec(`DELETE FROM wallets WHERE user_id = $1`, u)
			db.Exec(`DELETE FROM users WHERE id = $1`, u)
		}
	})

	// Happy path — lowercase input canonicalizes, wallet credited, tx recorded.
	amount, err := repo.Redeem(ctx, " "+string(code[0])+string(code[1:]), users[0])
	if err != nil || amount != 50 {
		t.Fatalf("redeem #1: amount=%v err=%v, want 50/nil", amount, err)
	}
	// Promo credits PLAY-ONLY bonus now, not withdrawable cash: wallet stays 0,
	// the 50 lands in bonus_grants.
	var balance float64
	db.QueryRow(`SELECT balance FROM wallets WHERE user_id = $1`, users[0]).Scan(&balance)
	if balance != 0 {
		t.Fatalf("wallet balance after redeem = %v, want 0 (promo is bonus)", balance)
	}
	if b := bonusOf(users[0]); b != 50 {
		t.Fatalf("bonus after redeem = %v, want 50", b)
	}
	var txCount int
	// Audit trail is the bonus grant (reason carries the promo code).
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM bonus_grants
		WHERE user_id = $1 AND amount = 50 AND reason LIKE 'Promo code:%'
	`, users[0]).Scan(&txCount); err != nil || txCount != 1 {
		t.Fatalf("audit bonus-grant count = %d (err %v), want 1", txCount, err)
	}

	// Same user again → already redeemed, balance unchanged.
	if _, err := repo.Redeem(ctx, code, users[0]); !errors.Is(err, domain.ErrPromoAlreadyRedeemed) {
		t.Fatalf("redeem #2 err = %v, want ErrPromoAlreadyRedeemed", err)
	}
	if b := bonusOf(users[0]); b != 50 {
		t.Fatalf("bonus after duplicate redeem = %v, want 50 (unchanged)", b)
	}

	// Second user takes the last slot (cap 2)…
	if _, err := repo.Redeem(ctx, code, users[1]); err != nil {
		t.Fatalf("redeem #3 (user 2): %v", err)
	}
	// …third user finds it exhausted.
	if _, err := repo.Redeem(ctx, code, users[2]); !errors.Is(err, domain.ErrPromoExhausted) {
		t.Fatalf("redeem #4 err = %v, want ErrPromoExhausted", err)
	}

	// Deactivated code → inactive.
	if err := repo.SetActive(ctx, code, false); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := repo.Redeem(ctx, code, users[2]); !errors.Is(err, domain.ErrPromoInactive) {
		t.Fatalf("redeem #5 err = %v, want ErrPromoInactive", err)
	}

	// Unknown code → not found.
	if _, err := repo.Redeem(ctx, "NOPE"+code, users[2]); !errors.Is(err, domain.ErrPromoNotFound) {
		t.Fatalf("redeem #6 err = %v, want ErrPromoNotFound", err)
	}
}
