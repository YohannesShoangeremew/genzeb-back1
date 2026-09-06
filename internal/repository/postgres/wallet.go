package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

type walletRepository struct {
	db *sql.DB
}

// NewWalletRepository creates a new PostgreSQL wallet repository
func NewWalletRepository(db *sql.DB) domain.WalletRepository {
	return &walletRepository{db: db}
}

// Create inserts a new wallet into the database
func (r *walletRepository) Create(ctx context.Context, tx *sql.Tx, wallet *domain.Wallet) error {
	query := `
		INSERT INTO wallets (user_id, balance, demo_balance, updated_at)
		VALUES ($1, $2, $3, $4)
	`

	now := time.Now()
	wallet.UpdatedAt = now

	var err error
	if tx != nil {
		_, err = tx.ExecContext(
			ctx,
			query,
			wallet.UserID,
			wallet.Balance,
			wallet.DemoBalance,
			wallet.UpdatedAt,
		)
	} else {
		_, err = r.db.ExecContext(
			ctx,
			query,
			wallet.UserID,
			wallet.Balance,
			wallet.DemoBalance,
			wallet.UpdatedAt,
		)
	}

	if err != nil {
		return fmt.Errorf("failed to create wallet: %w", err)
	}

	return nil
}

// FindByUserID finds a wallet by user ID
func (r *walletRepository) FindByUserID(ctx context.Context, userID uuid.UUID) (*domain.Wallet, error) {
	query := `
		SELECT user_id, balance, demo_balance, updated_at
		FROM wallets
		WHERE user_id = $1
	`

	wallet := &domain.Wallet{}

	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&wallet.UserID,
		&wallet.Balance,
		&wallet.DemoBalance,
		&wallet.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("wallet not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find wallet: %w", err)
	}

	return wallet, nil
}

// LockForUpdate locks a wallet row for update (used in transactions)
func (r *walletRepository) LockForUpdate(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (*domain.Wallet, error) {
	query := `
		SELECT user_id, balance, demo_balance, updated_at
		FROM wallets
		WHERE user_id = $1
		FOR UPDATE
	`

	wallet := &domain.Wallet{}

	err := tx.QueryRowContext(ctx, query, userID).Scan(
		&wallet.UserID,
		&wallet.Balance,
		&wallet.DemoBalance,
		&wallet.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("wallet not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to lock wallet: %w", err)
	}

	return wallet, nil
}

// UpdateBalance updates the balance of a wallet (used in transactions)
func (r *walletRepository) UpdateBalance(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64) error {
	query := `
		UPDATE wallets
		SET balance = balance + $2, updated_at = $3
		WHERE user_id = $1
	`

	now := time.Now()

	result, err := tx.ExecContext(ctx, query, userID, amount, now)
	if err != nil {
		return fmt.Errorf("failed to update balance: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("wallet not found")
	}

	return nil
}

// Update updates a wallet
// GetTotalBalance calculates the sum of all wallet balances
func (r *walletRepository) GetTotalBalance(ctx context.Context) (float64, error) {
	// Exclude filler-bot wallets: their balance is house float, not real
	// player money, and would otherwise inflate the platform's total balance.
	query := `
		SELECT COALESCE(SUM(w.balance), 0)
		FROM wallets w
		JOIN users u ON u.id = w.user_id
		WHERE u.is_bot = false
	`

	var totalBalance float64
	err := r.db.QueryRowContext(ctx, query).Scan(&totalBalance)
	if err != nil {
		return 0, fmt.Errorf("failed to get total balance: %w", err)
	}

	return totalBalance, nil
}
