package domain

import (
	"time"

	"github.com/google/uuid"
)

// User represents a user entity in the domain
type User struct {
	ID          uuid.UUID `json:"id" db:"id"`
	TelegramID  int64     `json:"telegram_id" db:"telegram_id"`
	FirstName   string    `json:"first_name" db:"first_name"`
	LastName    *string   `json:"last_name,omitempty" db:"last_name"`
	PhoneNumber string    `json:"phone_number" db:"phone_number"`
	ReferalCode string    `json:"referal_code" db:"referal_code"`
	// ReferredBy is who invited this user (nil if they signed up on their own),
	// set once at registration from the ?start=ref_<code> deep link.
	ReferredBy *uuid.UUID `json:"referred_by,omitempty" db:"referred_by"`
	Role       string     `json:"role" db:"role"`
	Banned     bool       `json:"banned" db:"banned"`
	IsBot      bool       `json:"is_bot" db:"is_bot"` // house-controlled filler player, not a real account
	Password   *string    `json:"-" db:"password"`    // Never expose password in JSON
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
}

// SetRoleRequest is the admin request to change a user's role.
type SetRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=user admin"`
}

// MakeAdminRequest is the admin request to promote a user to admin and set
// their dashboard password in one step.
type MakeAdminRequest struct {
	Password string `json:"password" binding:"required,min=8"`
}

// AdjustBalanceRequest is the admin request to credit (positive) or debit
// (negative) a user's wallet. Reason is recorded for the audit trail.
type AdjustBalanceRequest struct {
	Amount float64 `json:"amount" binding:"required"`
	Reason string  `json:"reason"`
}

// CreateUserRequest represents the data needed to create a new user
type CreateUserRequest struct {
	TelegramID int64   `json:"telegram_id" binding:"required"`
	FirstName  string  `json:"first_name" binding:"required"`
	LastName   *string `json:"last_name,omitempty"`
	Phone      string  `json:"phone" binding:"required"`
	// ReferrerCode is the referral code from the invite link that brought this
	// user in (optional). Resolved to a referrer at creation; the referrer is
	// paid only once the new user makes their first deposit.
	ReferrerCode string `json:"referrer_code,omitempty"`
}

// UpdateUserNameRequest represents the data needed to update a user's name
type UpdateUserNameRequest struct {
	FirstName string  `json:"first_name" binding:"required"`
	LastName  *string `json:"last_name,omitempty"`
}

// DashboardStats represents the admin dashboard statistics
type DashboardStats struct {
	PendingDeposits    int              `json:"pending_deposits"`
	PendingWithdrawals int              `json:"pending_withdrawals"`
	TotalUsers         int              `json:"total_users"`
	TotalTransactions  int              `json:"total_transactions"`
	TotalBalance       float64          `json:"total_balance"`
	GamesByType        map[GameType]int `json:"games_by_type"`
	TotalHouseCut      float64          `json:"total_house_cut"`
	// RealPlayerGamePnl = real-player stakes − real-player winnings. Negative
	// means the house has paid real players more than they staked (real cash
	// exposure, e.g. real players winning bot-inflated pools).
	RealPlayerGamePnl float64 `json:"real_player_game_pnl"`
}

// UserWithWallet represents a user with their wallet information
type UserWithWallet struct {
	*User
	Wallet *Wallet `json:"wallet,omitempty"`
}
