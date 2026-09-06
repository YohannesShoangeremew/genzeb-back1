package domain

import (
	"time"

	"github.com/google/uuid"
)

// Wallet represents a wallet entity in the domain
type Wallet struct {
	UserID      uuid.UUID `json:"user_id" db:"user_id"`
	Balance     float64   `json:"balance" db:"balance"`
	DemoBalance float64   `json:"demo_balance" db:"demo_balance"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// CreateWalletRequest represents the data needed to create a new wallet
type CreateWalletRequest struct {
	UserID      uuid.UUID `json:"user_id" binding:"required"`
	Balance     float64   `json:"balance"`
	DemoBalance float64   `json:"demo_balance"`
}
