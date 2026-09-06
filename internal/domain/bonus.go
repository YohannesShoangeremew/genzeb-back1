package domain

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// BonusGrant is one award of play-only money. `Remaining` falls as the player
// stakes it, and the grant stops being spendable the moment ExpiresAt passes —
// there is no sweep job, expiry is applied when the balance is read.
type BonusGrant struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Amount    float64   `json:"amount"`
	Remaining float64   `json:"remaining"`
	Reason    *string   `json:"reason,omitempty"`
	GrantedAt time.Time `json:"granted_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// BonusBalance is what a player currently holds in play-only money.
type BonusBalance struct {
	Amount float64 `json:"amount"`
	// NextExpiry is when the soonest-expiring live grant runs out, so the
	// client can warn the player. Nil when there is no bonus.
	NextExpiry *time.Time `json:"next_expiry,omitempty"`
}

// BonusConfig is the single-row admin policy.
type BonusConfig struct {
	Enabled bool `json:"enabled"`
	// ExpiryDays applies to grants made from now on. Changing it never
	// shortens or extends a grant already issued.
	ExpiryDays int `json:"expiry_days"`
	// Announcement is shown to players next to their bonus balance, so the
	// operator can describe the current promotion without a deploy.
	Announcement string    `json:"announcement"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UpdateBonusConfigRequest is a partial update from the admin dashboard.
type UpdateBonusConfigRequest struct {
	Enabled      *bool   `json:"enabled"`
	ExpiryDays   *int    `json:"expiry_days"`
	Announcement *string `json:"announcement"`
}

// GrantBonusRequest is an admin awarding bonus to one player.
type GrantBonusRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
	Amount float64   `json:"amount" binding:"required,gt=0"`
	Reason string    `json:"reason"`
}

// BonusRepository stores play-only money.
//
// Everything that moves money takes a *sql.Tx, because bonus spending happens
// inside the same transaction as the stake it pays for: a bonus consumed
// without the matching card, or a card charged without the bonus consumed,
// would both be corruption.
type BonusRepository interface {
	// Grant awards bonus expiring after the configured number of days.
	Grant(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64, reason string) (*BonusGrant, error)
	// GrantWithExpiry awards bonus lapsing after an explicit number of minutes,
	// for a campaign that wants a short, urgency-driven lifetime instead of the
	// general policy default.
	GrantWithExpiry(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64, reason string, expiresInMinutes int) (*BonusGrant, error)
	// GrantDepositOnce awards the configured deposit reward at most once per
	// deposit transaction. The database uniqueness guard makes retries idempotent.
	GrantDepositOnce(ctx context.Context, tx *sql.Tx, transactionID, userID uuid.UUID, amount float64) (bool, error)
	// GrantReferralOnce claims an invitee's first real deposit and awards the
	// referrer play-only bonus. The users-row guard makes concurrent deposits
	// and approval retries idempotent.
	GrantReferralOnce(ctx context.Context, tx *sql.Tx, inviteeID, referrerID uuid.UUID, amount float64) (bool, error)
	// Balance returns spendable bonus (expired grants excluded) and the
	// soonest upcoming expiry.
	Balance(ctx context.Context, userID uuid.UUID) (*BonusBalance, error)
	// SpendableForUpdate reads live bonus while LOCKING the grant rows, so the
	// caller can decide how to split a purchase between bonus and cash before
	// mutating anything. Without this the charge path would have to consume
	// bonus first and undo it on discovering the player cannot afford the cash
	// remainder — leaving bonus stranded if the undo were ever missed.
	SpendableForUpdate(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (float64, error)
	// ConsumeForStake spends live bonus on WHOLE units of unitPrice (a game
	// card), soonest-expiring first, up to maxUnits. It returns how many units
	// the bonus covered and the earliest expiry it drew from.
	//
	// Whole units, not an arbitrary amount, because a card cannot be half
	// bought: consuming a part-card's worth of bonus would leave money debited
	// against nothing. Bonus too small for a single card is simply left alone.
	//
	// Callers MUST hold a transaction; the grant rows are locked for update so
	// two concurrent stakes cannot both spend the same bonus.
	ConsumeForStake(ctx context.Context, tx *sql.Tx, userID uuid.UUID, unitPrice float64, maxUnits int) (int, *time.Time, error)
	// Restore returns refunded bonus under its ORIGINAL deadline, so a player
	// cannot extend a bonus by repeatedly joining and leaving games. A
	// deadline already in the past is dropped rather than resurrected.
	Restore(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64, expiresAt time.Time, reason string) error
	// ListGrants returns a player's grants, newest first, for the admin view.
	ListGrants(ctx context.Context, userID uuid.UUID, limit int) ([]*BonusGrant, error)
	// TotalOutstanding is the house's live bonus liability across all players.
	TotalOutstanding(ctx context.Context) (float64, error)

	GetConfig(ctx context.Context) (*BonusConfig, error)
	UpdateConfig(ctx context.Context, cfg *BonusConfig) error
}
