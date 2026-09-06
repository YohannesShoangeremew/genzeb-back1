package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrVerifierUnavailable means the payment verifier could not produce a verdict
// (network failure, timeout, 5xx, auth, or rate-limit). Callers should fall back
// to manual admin approval rather than rejecting the deposit. It does NOT mean
// the receipt was rejected — a definitive "receipt invalid" is a plain error.
var ErrVerifierUnavailable = errors.New("payment verifier unavailable")

// TransactionType represents the type of transaction
type TransactionType string

const (
	TransactionTypeDeposit     TransactionType = "deposit"
	TransactionTypeWithdraw    TransactionType = "withdraw"
	TransactionTypeTransferIn  TransactionType = "transfer_in"
	TransactionTypeTransferOut TransactionType = "transfer_out"
)

// TransactionCategory records what a money movement actually WAS, independent of
// its balance direction (Type). Type only says deposit/withdraw, so a game prize,
// a stake refund and an admin credit all share Type == deposit; Category is what
// lets the admin UI tell them apart ("Winnings" vs "Deposit" vs "Refund").
type TransactionCategory string

const (
	TransactionCategoryDeposit        TransactionCategory = "deposit"         // real money in (e.g. Telebirr top-up)
	TransactionCategoryWithdrawal     TransactionCategory = "withdrawal"      // real money out (payout to phone)
	TransactionCategoryBet            TransactionCategory = "bet"             // stake placed on a game card
	TransactionCategoryWinnings       TransactionCategory = "winnings"        // prize paid to a game winner
	TransactionCategoryRefund         TransactionCategory = "refund"          // stake returned (left/cancelled game)
	TransactionCategoryTransferIn     TransactionCategory = "transfer_in"     // received from another player
	TransactionCategoryTransferOut    TransactionCategory = "transfer_out"    // sent to another player
	TransactionCategoryAdminCredit    TransactionCategory = "admin_credit"    // manual balance increase by an admin
	TransactionCategoryAdminDebit     TransactionCategory = "admin_debit"     // manual balance decrease by an admin
	TransactionCategoryBotFunding     TransactionCategory = "bot_funding"     // house money injected to bankroll a bot wallet
	TransactionCategoryReferralReward TransactionCategory = "referral_reward" // play-only reward paid to a referrer on their invitee's first real deposit
	// Bonus (play-only money) movements. Kept separate from admin_credit so
	// promotional giveaway can be reported apart from manual adjustments —
	// previously promo bonuses were indistinguishable from admin credits.
	TransactionCategoryBonusGrant   TransactionCategory = "bonus_grant"   // play-only bonus awarded
	TransactionCategoryBonusStake   TransactionCategory = "bonus_stake"   // bonus spent on a card
	TransactionCategoryBonusRefund  TransactionCategory = "bonus_refund"  // bonus-funded stake returned as bonus
	TransactionCategoryBonusExpired TransactionCategory = "bonus_expired" // unspent bonus lapsed
)

// TransactionStatus represents the status of a transaction
type TransactionStatus string

// AdminTransactionFilter describes one admin transaction tab. Search is applied
// in PostgreSQL before LIMIT/OFFSET so pagination covers the full result set.
type AdminTransactionFilter struct {
	Status          *TransactionStatus
	Type            *TransactionType
	Types           []TransactionType
	Category        *TransactionCategory
	RealPlayersOnly bool
}

const (
	TransactionStatusPending   TransactionStatus = "pending"
	TransactionStatusCompleted TransactionStatus = "completed"
	TransactionStatusFailed    TransactionStatus = "failed"
	TransactionStatusCancelled TransactionStatus = "cancelled"
)

// PaymentMethod represents the payment method
type PaymentMethod string

const (
	PaymentMethodTelebirr PaymentMethod = "Telebirr"
	PaymentMethodCBEBirr  PaymentMethod = "CBEBirr"
	PaymentMethodMpesa    PaymentMethod = "Mpesa"
)

// Note: bank CBE ("CBE") is not an accepted payment method; the house accepts
// CBE Birr (CBE's phone-based mobile money) instead. Historical transactions
// may still carry transaction_type = "CBE" in the database; that value reads
// back fine as a plain string but can no longer be submitted.

// SupportedPaymentMethods lists the methods a player may use for deposits and
// withdrawals — all phone-based mobile money. Verification of each is delegated
// to the external verifier (verify.leul.et). Keep this in sync with the
// transaction_type DB CHECK constraint (migrations/026_cbebirr_transaction_type.sql).
var SupportedPaymentMethods = []PaymentMethod{
	PaymentMethodTelebirr,
	PaymentMethodCBEBirr,
	PaymentMethodMpesa,
}

// IsSupportedPaymentMethod reports whether m is an accepted payment method.
func IsSupportedPaymentMethod(m PaymentMethod) bool {
	for _, s := range SupportedPaymentMethods {
		if m == s {
			return true
		}
	}
	return false
}

// PaymentVerificationResult contains normalized data returned by an external
// payment verifier.
type PaymentVerificationResult struct {
	Provider  PaymentMethod `json:"provider"`
	Reference string        `json:"reference"`
	Amount    float64       `json:"amount"`
	Status    string        `json:"status,omitempty"`
}

// PaymentVerificationRequest carries everything the external verifier needs to
// look up a receipt: the method and its reference (for CBE Birr and M-Pesa the
// receipt number). CBE Birr and M-Pesa lookups additionally need a phone
// involved in the transaction — the verifier supplies the house number of the
// method from config, so nothing extra is required from the player.
type PaymentVerificationRequest struct {
	Method    PaymentMethod
	Reference string
	// UserID is the player the lookup is for. It is not sent to the verifier; it
	// only tags the audit-log row so an admin can see who submitted a receipt.
	UserID uuid.UUID
}

// VerificationOutcome classifies a verifier lookup for the admin audit log.
type VerificationOutcome string

const (
	VerificationVerified    VerificationOutcome = "verified"    // auto-credited
	VerificationRejected    VerificationOutcome = "rejected"    // definitive negative verdict
	VerificationUnavailable VerificationOutcome = "unavailable" // no verdict → manual review
)

// VerificationLog is one recorded verifier lookup, surfaced in the admin
// dashboard so operators can inspect what the verifier actually returned for a
// receipt (raw provider JSON + verdict) when investigating a disputed deposit.
type VerificationLog struct {
	ID          uuid.UUID           `json:"id" db:"id"`
	UserID      *uuid.UUID          `json:"user_id,omitempty" db:"user_id"`
	Method      PaymentMethod       `json:"method" db:"method"`
	Reference   string              `json:"reference" db:"reference"`
	Outcome     VerificationOutcome `json:"outcome" db:"outcome"`
	Reason      string              `json:"reason" db:"reason"`
	Amount      *float64            `json:"amount,omitempty" db:"amount"`
	RawResponse string              `json:"raw_response" db:"raw_response"`
	CreatedAt   time.Time           `json:"created_at" db:"created_at"`
	PlayerName  string              `json:"player_name,omitempty"`
	PlayerPhone string              `json:"player_phone,omitempty"`
}

// VerificationRecorder persists verifier lookups for the admin audit log. The
// verifier calls Record after every lookup; a nil recorder disables logging.
// Implementations must be best-effort: a logging failure must never break a
// deposit.
type VerificationRecorder interface {
	Record(ctx context.Context, entry *VerificationLog)
}

// VerificationLogRepository is the admin-side reader for the audit log, plus the
// recorder the verifier writes through.
type VerificationLogRepository interface {
	VerificationRecorder
	List(ctx context.Context, reference string, limit, offset int) ([]*VerificationLog, int, error)
	// LatestByReference returns the most recent lookup for a receipt reference, or
	// nil when none exists. Used to block approval of a definitively-rejected
	// receipt. Reference match is case-insensitive.
	LatestByReference(ctx context.Context, reference string) (*VerificationLog, error)
}

type PaymentVerifier interface {
	Verify(ctx context.Context, req PaymentVerificationRequest) (*PaymentVerificationResult, error)
	// Available reports whether the verifier is reachable right now. Deposit entry
	// is gated on this so a player is never asked to pay into a window in which
	// their receipt could not be verified (which would leave them asking "where is
	// my money?"). The result is cached briefly so it doesn't add a network round
	// trip to every check.
	Available(ctx context.Context) bool
}

// Compile-time assurance that a repository satisfying VerificationLogRepository
// can be passed anywhere a plain VerificationRecorder is expected.
var _ VerificationRecorder = (VerificationLogRepository)(nil)

// Transaction represents a transaction entity in the domain
type Transaction struct {
	ID              uuid.UUID           `json:"id" db:"id"`
	UserID          uuid.UUID           `json:"user_id" db:"user_id"`
	Type            TransactionType     `json:"type" db:"type"`
	Category        TransactionCategory `json:"category,omitempty" db:"category"`
	Amount          float64             `json:"amount" db:"amount"`
	Status          TransactionStatus   `json:"status" db:"status"`
	TransactionType *PaymentMethod      `json:"transaction_type,omitempty" db:"transaction_type"`
	TransactionID   *string             `json:"transaction_id,omitempty" db:"transaction_id"`
	Reference       *string             `json:"reference,omitempty" db:"reference"`
	CreatedAt       time.Time           `json:"created_at" db:"created_at"`
	// PlayerName / PlayerPhone are populated only by the admin list queries that
	// JOIN users, so the dashboard can label each row with who it belongs to
	// without loading every user client-side.
	PlayerName  *string `json:"player_name,omitempty"`
	PlayerPhone *string `json:"player_phone,omitempty"`
}

// DepositRequest represents the data needed to create a deposit.
// UserID is populated from the authenticated JWT, not the request body.
type DepositRequest struct {
	UserID          uuid.UUID     `json:"-"`
	Amount          float64       `json:"amount" binding:"required,gt=0"`
	TransactionType PaymentMethod `json:"transaction_type" binding:"required"`
	TransactionID   string        `json:"transaction_id" binding:"required"`
}

// WithdrawRequest represents the data needed to create a withdrawal.
// UserID is populated from the authenticated JWT, not the request body.
// AccountNumber is ignored: payouts always go to the user's verified
// registration phone (see WalletUseCase.Withdraw), so a client cannot redirect
// money to another account. The field is retained for backward compatibility.
type WithdrawRequest struct {
	UserID        uuid.UUID     `json:"-"`
	Amount        float64       `json:"amount" binding:"required,gt=0"`
	AccountNumber string        `json:"account_number"`
	AccountType   PaymentMethod `json:"account_type" binding:"required"`
}

// TransferRequest represents the data needed to create a transfer.
// SenderID is populated from the authenticated JWT, not the request body.
type TransferRequest struct {
	SenderID   uuid.UUID `json:"-"`
	ReceiverID uuid.UUID `json:"receiver_id" binding:"required"`
	Amount     float64   `json:"amount" binding:"required,gt=0"`
}

// TransferHistoryEntry represents a transfer transaction with user information
type TransferHistoryEntry struct {
	Transaction *Transaction `json:"transaction"`
	To          *User        `json:"to,omitempty"` // User info for transfer_out (receiver) or transfer_in (sender)
}
