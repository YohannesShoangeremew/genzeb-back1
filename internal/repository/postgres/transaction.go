package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// txWithUserColumns is the SELECT list for admin lists that also want the
// player's name/phone (JOIN users u ON u.id = t.user_id). Pair with
// scanTransactionsWithUser.
const txWithUserColumns = `t.id, t.user_id, t.type, t.category, t.amount, t.status, t.transaction_type, t.transaction_id, t.reference, t.created_at, u.first_name, u.last_name, u.phone_number`

// scanTransactionsWithUser scans the base transaction columns plus the joined
// player name/phone, so admin rows carry who they belong to.
func (r *transactionRepository) scanTransactionsWithUser(rows *sql.Rows) ([]*domain.Transaction, error) {
	var out []*domain.Transaction
	for rows.Next() {
		t := &domain.Transaction{}
		var category, transactionType, transactionID, reference sql.NullString
		var firstName, lastName, phone sql.NullString
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Type, &category, &t.Amount, &t.Status,
			&transactionType, &transactionID, &reference, &t.CreatedAt,
			&firstName, &lastName, &phone,
		); err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		if category.Valid {
			t.Category = domain.TransactionCategory(category.String)
		}
		if transactionType.Valid {
			pm := domain.PaymentMethod(transactionType.String)
			t.TransactionType = &pm
		}
		if transactionID.Valid {
			t.TransactionID = &transactionID.String
		}
		if reference.Valid {
			t.Reference = &reference.String
		}
		if name := strings.TrimSpace(firstName.String + " " + lastName.String); name != "" {
			t.PlayerName = &name
		}
		if phone.Valid && phone.String != "" {
			p := phone.String
			t.PlayerPhone = &p
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

type transactionRepository struct {
	db *sql.DB
}

// NewTransactionRepository creates a new PostgreSQL transaction repository
func NewTransactionRepository(db *sql.DB) domain.TransactionRepository {
	return &transactionRepository{db: db}
}

// Create inserts a new transaction into the database
func (r *transactionRepository) Create(ctx context.Context, tx *sql.Tx, transaction *domain.Transaction) error {
	query := `
		INSERT INTO transactions (id, user_id, type, category, amount, status, transaction_type, transaction_id, reference, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	now := time.Now()
	transaction.CreatedAt = now

	if transaction.ID == uuid.Nil {
		transaction.ID = uuid.New()
	}

	// Empty category maps to NULL so it never trips the CHECK constraint. In
	// practice every creation path sets a category; this only guards legacy code.
	var category any
	if transaction.Category != "" {
		category = string(transaction.Category)
	}

	var err error
	if tx != nil {
		_, err = tx.ExecContext(
			ctx,
			query,
			transaction.ID,
			transaction.UserID,
			transaction.Type,
			category,
			transaction.Amount,
			transaction.Status,
			transaction.TransactionType,
			transaction.TransactionID,
			transaction.Reference,
			transaction.CreatedAt,
		)
	} else {
		_, err = r.db.ExecContext(
			ctx,
			query,
			transaction.ID,
			transaction.UserID,
			transaction.Type,
			category,
			transaction.Amount,
			transaction.Status,
			transaction.TransactionType,
			transaction.TransactionID,
			transaction.Reference,
			transaction.CreatedAt,
		)
	}

	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	return nil
}

// FindByID finds a transaction by ID
func (r *transactionRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Transaction, error) {
	query := `
		SELECT id, user_id, type, category, amount, status, transaction_type, transaction_id, reference, created_at
		FROM transactions
		WHERE id = $1
	`

	transaction := &domain.Transaction{}
	var category sql.NullString
	var transactionType sql.NullString
	var transactionID sql.NullString
	var reference sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&transaction.ID,
		&transaction.UserID,
		&transaction.Type,
		&category,
		&transaction.Amount,
		&transaction.Status,
		&transactionType,
		&transactionID,
		&reference,
		&transaction.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("transaction not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find transaction: %w", err)
	}

	if category.Valid {
		transaction.Category = domain.TransactionCategory(category.String)
	}
	if transactionType.Valid {
		pm := domain.PaymentMethod(transactionType.String)
		transaction.TransactionType = &pm
	}
	if transactionID.Valid {
		transaction.TransactionID = &transactionID.String
	}
	if reference.Valid {
		transaction.Reference = &reference.String
	}

	return transaction, nil
}

// FindByUserID finds transactions by user ID
func (r *transactionRepository) FindByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Transaction, error) {
	query := `
		SELECT id, user_id, type, category, amount, status, transaction_type, transaction_id, reference, created_at
		FROM transactions
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions: %w", err)
	}
	defer rows.Close()

	var transactions []*domain.Transaction
	for rows.Next() {
		transaction := &domain.Transaction{}
		var category sql.NullString
		var transactionType sql.NullString
		var transactionID sql.NullString
		var reference sql.NullString

		err := rows.Scan(
			&transaction.ID,
			&transaction.UserID,
			&transaction.Type,
			&category,
			&transaction.Amount,
			&transaction.Status,
			&transactionType,
			&transactionID,
			&reference,
			&transaction.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}

		if category.Valid {
			transaction.Category = domain.TransactionCategory(category.String)
		}
		if transactionType.Valid {
			pm := domain.PaymentMethod(transactionType.String)
			transaction.TransactionType = &pm
		}
		if transactionID.Valid {
			transaction.TransactionID = &transactionID.String
		}
		if reference.Valid {
			transaction.Reference = &reference.String
		}

		transactions = append(transactions, transaction)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate transactions: %w", err)
	}

	return transactions, nil
}

// FindByUserIDAndType finds transactions by user ID and transaction type
// Excludes game-related transactions (GAME_BET, GAME_REFUND, GAME_PRIZE)
func (r *transactionRepository) FindByUserIDAndType(ctx context.Context, userID uuid.UUID, transactionType domain.TransactionType, limit int) ([]*domain.Transaction, error) {
	query := `
		SELECT id, user_id, type, category, amount, status, transaction_type, transaction_id, reference, created_at
		FROM transactions
		WHERE user_id = $1 
		  AND type = $2
		  AND (reference IS NULL OR reference NOT LIKE 'GAME_%')
		ORDER BY created_at DESC
		LIMIT $3
	`

	rows, err := r.db.QueryContext(ctx, query, userID, transactionType, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

// FindByUserIDAndTypes finds transactions by user ID and multiple transaction types
// Excludes game-related transactions (GAME_BET, GAME_REFUND, GAME_PRIZE)
func (r *transactionRepository) FindByUserIDAndTypes(ctx context.Context, userID uuid.UUID, transactionTypes []domain.TransactionType, limit int) ([]*domain.Transaction, error) {
	if len(transactionTypes) == 0 {
		return []*domain.Transaction{}, nil
	}

	// Build query with IN clause
	query := `
		SELECT id, user_id, type, category, amount, status, transaction_type, transaction_id, reference, created_at
		FROM transactions
		WHERE user_id = $1 
		  AND type = ANY($2)
		  AND (reference IS NULL OR reference NOT LIKE 'GAME_%')
		ORDER BY created_at DESC
		LIMIT $3
	`

	// Convert []TransactionType to []string for PostgreSQL array
	typeStrings := make([]string, len(transactionTypes))
	for i, t := range transactionTypes {
		typeStrings[i] = string(t)
	}

	rows, err := r.db.QueryContext(ctx, query, userID, pq.Array(typeStrings), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

// scanTransactions is a helper function to scan transaction rows
func (r *transactionRepository) scanTransactions(rows *sql.Rows) ([]*domain.Transaction, error) {
	var transactions []*domain.Transaction
	for rows.Next() {
		transaction := &domain.Transaction{}
		var category sql.NullString
		var transactionType sql.NullString
		var transactionID sql.NullString
		var reference sql.NullString

		err := rows.Scan(
			&transaction.ID,
			&transaction.UserID,
			&transaction.Type,
			&category,
			&transaction.Amount,
			&transaction.Status,
			&transactionType,
			&transactionID,
			&reference,
			&transaction.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}

		if category.Valid {
			transaction.Category = domain.TransactionCategory(category.String)
		}
		if transactionType.Valid {
			pm := domain.PaymentMethod(transactionType.String)
			transaction.TransactionType = &pm
		}
		if transactionID.Valid {
			transaction.TransactionID = &transactionID.String
		}
		if reference.Valid {
			transaction.Reference = &reference.String
		}

		transactions = append(transactions, transaction)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate transactions: %w", err)
	}

	return transactions, nil
}

// FindByStatusAndType finds transactions by status and type
func (r *transactionRepository) FindByStatusAndType(ctx context.Context, status domain.TransactionStatus, transactionType domain.TransactionType, limit, offset int) ([]*domain.Transaction, error) {
	query := `
		SELECT ` + txWithUserColumns + `
		FROM transactions t LEFT JOIN users u ON u.id = t.user_id
		WHERE t.status = $1 AND t.type = $2
		ORDER BY t.created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.db.QueryContext(ctx, query, status, transactionType, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactionsWithUser(rows)
}

// FindByStatus finds transactions by status
func (r *transactionRepository) FindByStatus(ctx context.Context, status domain.TransactionStatus, limit, offset int) ([]*domain.Transaction, error) {
	query := `
		SELECT ` + txWithUserColumns + `
		FROM transactions t LEFT JOIN users u ON u.id = t.user_id
		WHERE t.status = $1
		ORDER BY t.created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactionsWithUser(rows)
}

// FindByTypes finds transactions by multiple types
func (r *transactionRepository) FindByTypes(ctx context.Context, transactionTypes []domain.TransactionType, limit, offset int) ([]*domain.Transaction, error) {
	if len(transactionTypes) == 0 {
		return []*domain.Transaction{}, nil
	}

	query := `
		SELECT ` + txWithUserColumns + `
		FROM transactions t LEFT JOIN users u ON u.id = t.user_id
		WHERE t.type = ANY($1)
		ORDER BY t.created_at DESC
		LIMIT $2 OFFSET $3
	`

	typeStrings := make([]string, len(transactionTypes))
	for i, t := range transactionTypes {
		typeStrings[i] = string(t)
	}

	rows, err := r.db.QueryContext(ctx, query, pq.Array(typeStrings), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactionsWithUser(rows)
}

// FindAll finds all transactions with pagination
func (r *transactionRepository) FindAll(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	query := `
		SELECT ` + txWithUserColumns + `
		FROM transactions t LEFT JOIN users u ON u.id = t.user_id
		ORDER BY t.created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find transactions: %w", err)
	}
	defer rows.Close()

	return r.scanTransactionsWithUser(rows)
}

// FindRealPlayerWinnings lists 'winnings' transactions belonging to real (non-bot)
// players, newest first — the admin winners tab.
func (r *transactionRepository) FindRealPlayerWinnings(ctx context.Context, limit, offset int) ([]*domain.Transaction, error) {
	query := `
		SELECT ` + txWithUserColumns + `
		FROM transactions t
		JOIN users u ON u.id = t.user_id
		WHERE t.category = 'winnings' AND u.is_bot = false
		ORDER BY t.created_at DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find real-player winnings: %w", err)
	}
	defer rows.Close()
	return r.scanTransactionsWithUser(rows)
}

// FindWithdrawalsByStatus lists ONLY genuine withdrawal REQUESTS (category
// 'withdrawal') at a given status — excluding bets, which are also stored as
// type 'withdraw' (category 'bet') and otherwise pollute the withdrawals view.
func (r *transactionRepository) FindWithdrawalsByStatus(ctx context.Context, status domain.TransactionStatus, limit, offset int) ([]*domain.Transaction, error) {
	query := `
		SELECT ` + txWithUserColumns + `
		FROM transactions t LEFT JOIN users u ON u.id = t.user_id
		WHERE t.type = 'withdraw' AND t.category = 'withdrawal' AND t.status = $1
		ORDER BY t.created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find withdrawals: %w", err)
	}
	defer rows.Close()
	return r.scanTransactionsWithUser(rows)
}

// CountWithdrawalsByStatus is the total for FindWithdrawalsByStatus (pagination).
func (r *transactionRepository) CountWithdrawalsByStatus(ctx context.Context, status domain.TransactionStatus) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT count(*) FROM transactions WHERE type='withdraw' AND category='withdrawal' AND status=$1`, status).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("failed to count withdrawals: %w", err)
	}
	return n, nil
}

// CountByUser is the total number of transactions for one user (pagination of
// their history on the admin player-detail view).
func (r *transactionRepository) CountByUser(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM transactions WHERE user_id = $1`, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("failed to count user transactions: %w", err)
	}
	return n, nil
}

// CountRealPlayerWinnings is the total for the winners list (pagination).
func (r *transactionRepository) CountRealPlayerWinnings(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
		SELECT count(*) FROM transactions t JOIN users u ON u.id = t.user_id
		WHERE t.category = 'winnings' AND u.is_bot = false`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("failed to count real-player winnings: %w", err)
	}
	return n, nil
}

// UpdateStatus updates the status of a transaction
func (r *transactionRepository) UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status domain.TransactionStatus) error {
	// Guard the transition on the row still being 'pending'. Every caller moves a
	// transaction out of pending exactly once; without this, two concurrent
	// approves/rejects/cancels (admin double-click, client retry) would each pass
	// the stale pre-check and each mutate the wallet, double-crediting/refunding.
	// With the guard the loser updates 0 rows, errors here, and its whole tx —
	// including the balance change — rolls back.
	query := `
		UPDATE transactions
		SET status = $2
		WHERE id = $1 AND status = $3
	`

	result, err := tx.ExecContext(ctx, query, id, status, domain.TransactionStatusPending)
	if err != nil {
		return fmt.Errorf("failed to update transaction status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("transaction not pending (already processed or not found)")
	}

	return nil
}

// ExistsActiveDepositByTransactionID reports whether a non-rejected deposit
// already exists with the given external payment reference. Used to block a
// player reusing a receipt. Rejected/cancelled deposits are excluded so a
// mistakenly-rejected reference can legitimately be resubmitted.
func (r *transactionRepository) ExistsActiveDepositByTransactionID(ctx context.Context, transactionID string) (bool, error) {
	// UPPER() on both sides makes the check case-insensitive so historical rows
	// stored before references were canonicalized to uppercase (see
	// WalletUseCase.Deposit) still block their case-variants from being reused.
	query := `
		SELECT EXISTS (
			SELECT 1 FROM transactions
			WHERE type = 'deposit'
			  AND UPPER(transaction_id) = UPPER($1)
			  AND status IN ('pending', 'completed')
		)
	`
	var exists bool
	if err := r.db.QueryRowContext(ctx, query, transactionID).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check duplicate transaction id: %w", err)
	}
	return exists, nil
}

// CountByStatusAndType counts transactions by status and type
func (r *transactionRepository) CountByStatusAndType(ctx context.Context, status domain.TransactionStatus, transactionType domain.TransactionType) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM transactions
		WHERE status = $1 AND type = $2
	`

	var count int
	err := r.db.QueryRowContext(ctx, query, status, transactionType).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count transactions: %w", err)
	}

	return count, nil
}

// CountAll counts all transactions
func (r *transactionRepository) CountAll(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM transactions`

	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count transactions: %w", err)
	}

	return count, nil
}

// RealPlayerGamePnL returns real-player stakes minus real-player winnings over
// all completed game transactions, excluding bots. Positive = house ahead;
// negative = real cash paid out beyond stakes (exposure).
func (r *transactionRepository) RealPlayerGamePnL(ctx context.Context) (float64, error) {
	query := `
		SELECT
			COALESCE(SUM(amount) FILTER (WHERE category = 'bet'), 0)
			- COALESCE(SUM(amount) FILTER (WHERE category = 'winnings'), 0) AS pnl
		FROM transactions t
		JOIN users u ON u.id = t.user_id
		WHERE u.is_bot = false
		  AND t.status = 'completed'
		  AND t.category IN ('bet', 'winnings')
	`
	var pnl float64
	if err := r.db.QueryRowContext(ctx, query).Scan(&pnl); err != nil {
		return 0, fmt.Errorf("failed to compute real-player game P&L: %w", err)
	}
	return pnl, nil
}

// ListAdmin serves every paginated transaction tab with one shared set of
// filters. Search is applied before pagination across player identity, receipt,
// internal reference, IDs, payment method, category, type, status, and amount.
func (r *transactionRepository) ListAdmin(ctx context.Context, filter domain.AdminTransactionFilter, search string, limit, offset int) ([]*domain.Transaction, int, error) {
	clauses := []string{"1=1"}
	args := make([]any, 0, 8)
	add := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if filter.Status != nil {
		clauses = append(clauses, "t.status = "+add(*filter.Status))
	}
	if filter.Type != nil {
		clauses = append(clauses, "t.type = "+add(*filter.Type))
	}
	if len(filter.Types) > 0 {
		values := make([]string, len(filter.Types))
		for i, transactionType := range filter.Types {
			values[i] = string(transactionType)
		}
		clauses = append(clauses, "t.type = ANY("+add(pq.Array(values))+")")
	}
	if filter.Category != nil {
		clauses = append(clauses, "t.category = "+add(*filter.Category))
	}
	if filter.RealPlayersOnly {
		clauses = append(clauses, "u.is_bot = false")
	}
	search = strings.TrimSpace(search)
	if search != "" {
		placeholder := add("%" + search + "%")
		clauses = append(clauses, `concat_ws(' ', u.first_name, u.last_name, u.phone_number,
			u.telegram_id::text, t.id::text, t.user_id::text, t.transaction_id,
			t.reference, t.transaction_type::text, t.type::text, t.category::text,
			t.status::text, t.amount::text) ILIKE `+placeholder)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")

	var total int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM transactions t LEFT JOIN users u ON u.id = t.user_id"+where,
		args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count admin transactions: %w", err)
	}

	limitArg := add(limit)
	offsetArg := add(offset)
	query := "SELECT " + txWithUserColumns +
		" FROM transactions t LEFT JOIN users u ON u.id = t.user_id" + where +
		" ORDER BY t.created_at DESC LIMIT " + limitArg + " OFFSET " + offsetArg
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list admin transactions: %w", err)
	}
	defer rows.Close()
	transactions, err := r.scanTransactionsWithUser(rows)
	if err != nil {
		return nil, 0, err
	}
	return transactions, total, nil
}
