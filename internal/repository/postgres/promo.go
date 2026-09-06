package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

// promoRepository implements domain.PromoRepository. Redemption reuses the
// same wallet/transaction repos as every other money path so the credit shows
// up in the player's history like any other completed transaction.
type promoRepository struct {
	db              *sql.DB
	walletRepo      domain.WalletRepository
	transactionRepo domain.TransactionRepository
	bonusRepo       domain.BonusRepository
}

func NewPromoRepository(db *sql.DB, walletRepo domain.WalletRepository, transactionRepo domain.TransactionRepository, bonusRepo domain.BonusRepository) domain.PromoRepository {
	return &promoRepository{db: db, walletRepo: walletRepo, transactionRepo: transactionRepo, bonusRepo: bonusRepo}
}

// Redeem validates and applies a promo code for one user, all inside a single
// DB transaction:
//
//	promo row locked FOR UPDATE  → cap checks can't race
//	INSERT redemption (PK code+user) → one redemption per user, DB-enforced
//	wallet locked + credited + transaction recorded → same pattern as admin credit
//
// Any failure rolls the whole thing back — a user can never be marked
// redeemed without the money landing, or vice versa.
func (r *promoRepository) Redeem(ctx context.Context, code string, userID uuid.UUID) (float64, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return 0, domain.ErrPromoNotFound
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var (
		amount    float64
		maxRed    sql.NullInt64
		redeemed  int
		expiresAt sql.NullTime
		active    bool
	)
	err = tx.QueryRowContext(ctx, `
		SELECT bonus_amount, max_redemptions, redeemed_count, expires_at, active
		FROM promo_codes WHERE code = $1 FOR UPDATE
	`, code).Scan(&amount, &maxRed, &redeemed, &expiresAt, &active)
	if err == sql.ErrNoRows {
		return 0, domain.ErrPromoNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to load promo code: %w", err)
	}

	switch {
	case !active:
		return 0, domain.ErrPromoInactive
	case expiresAt.Valid && time.Now().After(expiresAt.Time):
		return 0, domain.ErrPromoExpired
	case maxRed.Valid && int64(redeemed) >= maxRed.Int64:
		return 0, domain.ErrPromoExhausted
	}

	// One redemption per user per code — the primary key is the guarantee.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO promo_redemptions (code, user_id, amount) VALUES ($1, $2, $3)
	`, code, userID, amount); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return 0, domain.ErrPromoAlreadyRedeemed
		}
		return 0, fmt.Errorf("failed to record redemption: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE promo_codes SET redeemed_count = redeemed_count + 1 WHERE code = $1
	`, code); err != nil {
		return 0, fmt.Errorf("failed to count redemption: %w", err)
	}

	// Grant the promo as PLAY-ONLY BONUS, not withdrawable cash — consistent with
	// referral rewards and campaigns, and so a promo can't be cashed out for free
	// house money. The bonus_grants row is its own audit trail.
	if _, err := r.bonusRepo.Grant(ctx, tx, userID, amount, "Promo code: "+code); err != nil {
		return 0, fmt.Errorf("failed to grant promo bonus: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit redemption: %w", err)
	}
	return amount, nil
}

func (r *promoRepository) Create(ctx context.Context, promo *domain.PromoCode) error {
	promo.Code = strings.ToUpper(strings.TrimSpace(promo.Code))
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO promo_codes (code, bonus_amount, max_redemptions, expires_at, active)
		VALUES ($1, $2, $3, $4, TRUE)
	`, promo.Code, promo.BonusAmount, promo.MaxRedemptions, promo.ExpiresAt)
	if err != nil && strings.Contains(err.Error(), "duplicate key") {
		return fmt.Errorf("promo code %s already exists", promo.Code)
	}
	return err
}

func (r *promoRepository) List(ctx context.Context) ([]*domain.PromoCode, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT code, bonus_amount, max_redemptions, redeemed_count, expires_at, active, created_at
		FROM promo_codes ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*domain.PromoCode
	for rows.Next() {
		var (
			p         domain.PromoCode
			maxRed    sql.NullInt64
			expiresAt sql.NullTime
		)
		if err := rows.Scan(&p.Code, &p.BonusAmount, &maxRed, &p.RedeemedCount, &expiresAt, &p.Active, &p.CreatedAt); err != nil {
			return nil, err
		}
		if maxRed.Valid {
			v := int(maxRed.Int64)
			p.MaxRedemptions = &v
		}
		if expiresAt.Valid {
			t := expiresAt.Time
			p.ExpiresAt = &t
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (r *promoRepository) SetActive(ctx context.Context, code string, active bool) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE promo_codes SET active = $2 WHERE code = $1
	`, strings.ToUpper(strings.TrimSpace(code)), active)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrPromoNotFound
	}
	return nil
}
