package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// PromoCode is a redeemable bonus code (created by an admin, redeemed from the
// Telegram bot's "ፕሮሞ ኮድ" menu button). Each user may redeem a code once; the
// bonus is credited straight to their wallet.
type PromoCode struct {
	Code           string     `json:"code"`
	BonusAmount    float64    `json:"bonus_amount"`
	MaxRedemptions *int       `json:"max_redemptions,omitempty"` // nil = unlimited
	RedeemedCount  int        `json:"redeemed_count"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"` // nil = never expires
	Active         bool       `json:"active"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Distinct redemption failures so the bot can answer each case precisely.
var (
	ErrPromoNotFound        = errors.New("promo code not found")
	ErrPromoInactive        = errors.New("promo code is no longer active")
	ErrPromoExpired         = errors.New("promo code has expired")
	ErrPromoExhausted       = errors.New("promo code redemption limit reached")
	ErrPromoAlreadyRedeemed = errors.New("promo code already redeemed by this user")
)

// PromoRepository manages promo codes and their redemptions.
type PromoRepository interface {
	// Redeem atomically validates the code, records the redemption (one per
	// user per code) and credits the bonus to the user's wallet. Returns the
	// credited amount.
	Redeem(ctx context.Context, code string, userID uuid.UUID) (float64, error)
	Create(ctx context.Context, promo *PromoCode) error
	List(ctx context.Context) ([]*PromoCode, error)
	SetActive(ctx context.Context, code string, active bool) error
}
