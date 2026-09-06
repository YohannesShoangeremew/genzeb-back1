package domain

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Campaign statuses. A campaign is never deleted — ending it preserves the
// record of money given away.
const (
	BonusCampaignStatusActive = "active"
	BonusCampaignStatusEnded  = "ended"
)

// Claim refusals. These are returned to the player verbatim-ish by the handler,
// so they are named errors rather than strings: the HTTP layer has to tell
// "you already claimed" (fine, show the balance) apart from "slots gone" (show
// the try-tomorrow message) apart from a genuine fault.
var (
	ErrNoActiveCampaign       = errors.New("no bonus campaign is running right now")
	ErrCampaignExhausted      = errors.New("all bonus slots have been claimed")
	ErrCampaignAlreadyClaimed = errors.New("you have already claimed this bonus")
	ErrCampaignNotEligible    = errors.New("only players who have deposited before can claim")
)

// Machine-readable refusal codes sent to the client.
//
// The client must never render the error text: it is English prose, and the
// app is bilingual. Both the claim endpoint and the status endpoint report the
// SAME code for the same situation, so the app needs one mapping from code to
// translated string rather than one per endpoint.
const (
	ReasonNoCampaign     = "no_campaign"
	ReasonExhausted      = "exhausted"
	ReasonAlreadyClaimed = "already_claimed"
	ReasonNotEligible    = "not_eligible"
	ReasonRefused        = "refused"
)

// ReasonCode maps a refusal to its stable client-facing code.
func ReasonCode(err error) string {
	switch {
	case errors.Is(err, ErrNoActiveCampaign):
		return ReasonNoCampaign
	case errors.Is(err, ErrCampaignExhausted):
		return ReasonExhausted
	case errors.Is(err, ErrCampaignAlreadyClaimed):
		return ReasonAlreadyClaimed
	case errors.Is(err, ErrCampaignNotEligible):
		return ReasonNotEligible
	default:
		return ReasonRefused
	}
}

// BonusCampaign is a "first N players" giveaway: a pot split into a fixed
// number of equal slots, claimed first-come-first-served.
type BonusCampaign struct {
	ID     uuid.UUID `json:"id"`
	Amount float64   `json:"total_amount"`
	Slots  int       `json:"slots"`
	// AmountPerSlot is frozen at creation so every claimer gets the same,
	// already-rounded figure.
	AmountPerSlot float64 `json:"amount_per_slot"`
	ClaimedCount  int     `json:"claimed_count"`
	Announcement  string  `json:"announcement"`
	// ExpiryMinutes is how long a claimed bonus lasts, in minutes. Nil means
	// the campaign uses the general bonus_config.expiry_days default — most
	// giveaways set a short value here (e.g. 180 for three hours) for urgency.
	ExpiryMinutes *int       `json:"expiry_minutes,omitempty"`
	Status        string     `json:"status"`
	CreatedBy     *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	EndedAt       *time.Time `json:"ended_at,omitempty"`
}

// SlotsLeft is what the app shows as "6 of 10 left".
func (c *BonusCampaign) SlotsLeft() int {
	left := c.Slots - c.ClaimedCount
	if left < 0 {
		return 0
	}
	return left
}

// BonusCampaignClaim is one player's successful claim.
type BonusCampaignClaim struct {
	CampaignID uuid.UUID  `json:"campaign_id"`
	UserID     uuid.UUID  `json:"user_id"`
	GrantID    *uuid.UUID `json:"grant_id,omitempty"`
	Amount     float64    `json:"amount"`
	Position   int        `json:"position"`
	ClaimedAt  time.Time  `json:"claimed_at"`
	// Name/Phone are filled only by the admin listing, so the operator can see
	// who claimed without a second round of lookups.
	Name  string `json:"name,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// CreateBonusCampaignRequest is an admin starting today's giveaway.
type CreateBonusCampaignRequest struct {
	TotalAmount  float64 `json:"total_amount" binding:"required,gt=0"`
	Slots        int     `json:"slots" binding:"required,gt=0"`
	Announcement string  `json:"announcement"`
	// ExpiryMinutes sets how long a claimed bonus lasts. Nil/0 falls back to
	// the general policy expiry. The admin UI sends this as a value-plus-unit
	// (minutes/hours/days) already reduced to minutes.
	ExpiryMinutes *int `json:"expiry_minutes"`
	// Broadcast sends the announcement to every player on Telegram. Defaults
	// to false: creating the campaign and telling two thousand people about it
	// are different-sized mistakes to make by accident.
	Broadcast bool `json:"broadcast"`
}

// BonusCampaignStatus is the player-facing view — the campaign plus what this
// particular player can do about it.
type BonusCampaignStatus struct {
	Campaign *BonusCampaign `json:"campaign"`
	// Claimed is whether this player already took a slot.
	Claimed bool `json:"claimed"`
	// ClaimedAmount is what they got, when Claimed.
	ClaimedAmount float64 `json:"claimed_amount,omitempty"`
	// CanClaim is the single flag the client needs to enable the button.
	CanClaim bool `json:"can_claim"`
	// Reason explains a false CanClaim, so the app can say why rather than
	// showing a dead button.
	Reason string `json:"reason,omitempty"`
}

// BonusCampaignRepository stores first-N giveaway campaigns and their claims.
type BonusCampaignRepository interface {
	Create(ctx context.Context, c *BonusCampaign) error
	// Active returns the running campaign, or nil when there is none.
	Active(ctx context.Context) (*BonusCampaign, error)
	// ActiveForUpdate reads the running campaign while LOCKING its row, so a
	// caller can check the slot count and increment it without another claimer
	// interleaving. This is what makes the first-N cap exact under a stampede;
	// callers MUST hold a transaction.
	ActiveForUpdate(ctx context.Context, tx *sql.Tx) (*BonusCampaign, error)
	FindByID(ctx context.Context, id uuid.UUID) (*BonusCampaign, error)
	List(ctx context.Context, limit int) ([]*BonusCampaign, error)
	// RecordClaim inserts the claim and increments the campaign's counter in
	// one step, returning the 1-based position taken. It returns
	// ErrCampaignAlreadyClaimed when this player already holds a slot and
	// ErrCampaignExhausted when the slots ran out.
	RecordClaim(ctx context.Context, tx *sql.Tx, campaignID, userID uuid.UUID, amount float64, grantID uuid.UUID) (int, error)
	// FindClaim returns this player's claim on a campaign, or nil.
	FindClaim(ctx context.Context, campaignID, userID uuid.UUID) (*BonusCampaignClaim, error)
	// ListClaims returns a campaign's claimers in claim order, for the admin.
	ListClaims(ctx context.Context, campaignID uuid.UUID) ([]*BonusCampaignClaim, error)
	// End closes a campaign. Idempotent: ending an already-ended campaign is
	// not an error, because the slot-exhaustion path and an admin pressing
	// Stop can legitimately race.
	End(ctx context.Context, tx *sql.Tx, id uuid.UUID) error
	// HasCompletedDeposit reports whether the player has ever put real money
	// in. This is the anti-multi-account rule: a throwaway Telegram account
	// costs nothing to make, but one that has completed a deposit is a real
	// customer.
	HasCompletedDeposit(ctx context.Context, userID uuid.UUID) (bool, error)
}
