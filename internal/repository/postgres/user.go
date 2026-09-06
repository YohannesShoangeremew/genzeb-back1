package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type userRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new PostgreSQL user repository
func NewUserRepository(db *sql.DB) domain.UserRepository {
	return &userRepository{db: db}
}

// Create inserts a new user into the database
func (r *userRepository) Create(ctx context.Context, tx *sql.Tx, user *domain.User) error {
	query := `
		INSERT INTO users (id, telegram_id, first_name, last_name, phone_number, referal_code, role, password, is_bot, referred_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}

	// Set default role if not provided
	if user.Role == "" {
		user.Role = "user"
	}

	var referredBy any
	if user.ReferredBy != nil {
		referredBy = *user.ReferredBy
	}

	args := []any{
		user.ID,
		user.TelegramID,
		user.FirstName,
		user.LastName,
		user.PhoneNumber,
		user.ReferalCode,
		user.Role,
		user.Password,
		user.IsBot,
		referredBy,
		user.CreatedAt,
		user.UpdatedAt,
	}

	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, args...)
	} else {
		_, err = r.db.ExecContext(ctx, query, args...)
	}

	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// FindByTelegramID finds a user by their Telegram ID
func (r *userRepository) FindByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	query := `
		SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, banned, password, created_at, updated_at
		FROM users
		WHERE telegram_id = $1
	`

	user := &domain.User{}
	var lastName sql.NullString
	var password sql.NullString

	err := r.db.QueryRowContext(ctx, query, telegramID).Scan(
		&user.ID,
		&user.TelegramID,
		&user.FirstName,
		&lastName,
		&user.PhoneNumber,
		&user.ReferalCode,
		&user.Role,
		&user.Banned,
		&password,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by telegram_id: %w", err)
	}

	if lastName.Valid {
		user.LastName = &lastName.String
	}
	if password.Valid {
		user.Password = &password.String
	}

	return user, nil
}

// FindByPhone finds a user by their phone number
func (r *userRepository) FindByPhone(ctx context.Context, phone string) (*domain.User, error) {
	query := `
		SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, password, created_at, updated_at
		FROM users
		WHERE phone_number = $1
	`

	user := &domain.User{}
	var lastName sql.NullString
	var password sql.NullString

	err := r.db.QueryRowContext(ctx, query, phone).Scan(
		&user.ID,
		&user.TelegramID,
		&user.FirstName,
		&lastName,
		&user.PhoneNumber,
		&user.ReferalCode,
		&user.Role,
		&password,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by phone: %w", err)
	}

	if lastName.Valid {
		user.LastName = &lastName.String
	}
	if password.Valid {
		user.Password = &password.String
	}

	return user, nil
}

// FindByReferralCode finds a user by their referral code
func (r *userRepository) FindByReferralCode(ctx context.Context, referralCode string) (*domain.User, error) {
	query := `
		SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, password, created_at, updated_at
		FROM users
		WHERE referal_code = $1
	`

	user := &domain.User{}
	var lastName sql.NullString
	var password sql.NullString

	err := r.db.QueryRowContext(ctx, query, referralCode).Scan(
		&user.ID,
		&user.TelegramID,
		&user.FirstName,
		&lastName,
		&user.PhoneNumber,
		&user.ReferalCode,
		&user.Role,
		&password,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by referral_code: %w", err)
	}

	if lastName.Valid {
		user.LastName = &lastName.String
	}
	if password.Valid {
		user.Password = &password.String
	}

	return user, nil
}

// FindReferredBy returns everyone this user invited (referred_by = userID),
// newest first — for the "invited players" section on the admin profile.
func (r *userRepository) FindReferredBy(ctx context.Context, userID uuid.UUID) ([]*domain.User, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, created_at
		FROM users WHERE referred_by = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to find referred users: %w", err)
	}
	defer rows.Close()

	var out []*domain.User
	for rows.Next() {
		u := &domain.User{}
		var lastName sql.NullString
		if err := rows.Scan(&u.ID, &u.TelegramID, &u.FirstName, &lastName, &u.PhoneNumber, &u.ReferalCode, &u.Role, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan referred user: %w", err)
		}
		if lastName.Valid {
			u.LastName = &lastName.String
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// FindByID finds a user by their ID
func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	query := `
		SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, banned, password, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	user := &domain.User{}
	var lastName sql.NullString
	var password sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.TelegramID,
		&user.FirstName,
		&lastName,
		&user.PhoneNumber,
		&user.ReferalCode,
		&user.Role,
		&user.Banned,
		&password,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by id: %w", err)
	}

	if lastName.Valid {
		user.LastName = &lastName.String
	}
	if password.Valid {
		user.Password = &password.String
	}

	return user, nil
}

// FindAll finds all users with pagination
func (r *userRepository) FindAll(ctx context.Context, limit, offset int) ([]*domain.User, error) {
	query := `
		SELECT id, telegram_id, first_name, last_name, phone_number, referal_code, role, banned, password, created_at, updated_at
		FROM users
		WHERE is_bot = false
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find users: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		user := &domain.User{}
		var lastName sql.NullString
		var password sql.NullString

		err := rows.Scan(
			&user.ID,
			&user.TelegramID,
			&user.FirstName,
			&lastName,
			&user.PhoneNumber,
			&user.ReferalCode,
			&user.Role,
			&user.Banned,
			&password,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		if lastName.Valid {
			user.LastName = &lastName.String
		}
		// Never expose password in list responses
		user.Password = nil

		users = append(users, user)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate users: %w", err)
	}

	return users, nil
}

// SetAdminCredentialsByTelegramID promotes a user to admin and stores password hash.
func (r *userRepository) SetAdminCredentialsByTelegramID(ctx context.Context, telegramID int64, hashedPassword string) error {
	query := `
		UPDATE users
		SET role = 'admin', password = $2, updated_at = $3
		WHERE telegram_id = $1
	`

	result, err := r.db.ExecContext(ctx, query, telegramID, hashedPassword, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set admin credentials: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// Update updates an existing user
func (r *userRepository) Update(ctx context.Context, user *domain.User) error {
	query := `
		UPDATE users
		SET first_name = $2, last_name = $3, phone_number = $4, updated_at = $5
		WHERE id = $1
	`

	user.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(
		ctx,
		query,
		user.ID,
		user.FirstName,
		user.LastName,
		user.PhoneNumber,
		user.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// UpdateRole sets a user's role (e.g. promote to admin / demote to user).
func (r *userRepository) UpdateRole(ctx context.Context, id uuid.UUID, role string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET role = $2, updated_at = $3 WHERE id = $1`,
		id, role, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// SetAdminCredentialsByID promotes a user to admin and sets their password hash
// (by user ID). Used by the admin dashboard's "make admin" action.
func (r *userRepository) SetAdminCredentialsByID(ctx context.Context, id uuid.UUID, hashedPassword string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET role = 'admin', password = $2, updated_at = $3 WHERE id = $1`,
		id, hashedPassword, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set admin credentials: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// SetBanned bans or unbans a user.
func (r *userRepository) SetBanned(ctx context.Context, id uuid.UUID, banned bool) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE users SET banned = $2, updated_at = $3 WHERE id = $1`,
		id, banned, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update banned: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// Delete permanently removes a user. Foreign keys with ON DELETE CASCADE
// (wallets, transactions, game_players) remove the user's attached rows, and
// games.winner_id is set NULL — so no orphan rows are left behind.
func (r *userRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// CountAll counts all real users (filler bots are excluded so dashboard counts
// reflect actual accounts, not house-controlled players).
func (r *userRepository) CountAll(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM users WHERE is_bot = false`

	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count users: %w", err)
	}

	return count, nil
}

// ListAdmin returns one filtered page of real users with their wallet in a
// single joined query. This replaces the old N+1 wallet lookup performed once
// for every user row in the admin table.
func (r *userRepository) ListAdmin(ctx context.Context, search, role string, limit, offset int) ([]*domain.UserWithWallet, int, error) {
	where := `
		WHERE u.is_bot = false
		  AND ($1 = '' OR concat_ws(' ', u.first_name, u.last_name, u.phone_number,
		      u.telegram_id::text, u.referal_code, u.id::text) ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR u.role = $2)
	`

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users u `+where, search, role).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count admin users: %w", err)
	}

	query := `
		SELECT u.id, u.telegram_id, u.first_name, u.last_name, u.phone_number,
		       u.referal_code, u.referred_by, u.role, u.banned, u.created_at, u.updated_at,
		       w.user_id, w.balance::float8, w.demo_balance::float8, w.updated_at
		FROM users u
		LEFT JOIN wallets w ON w.user_id = u.id
	` + where + `
		ORDER BY u.created_at DESC
		LIMIT $3 OFFSET $4
	`
	rows, err := r.db.QueryContext(ctx, query, search, role, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list admin users: %w", err)
	}
	defer rows.Close()

	out := make([]*domain.UserWithWallet, 0, limit)
	for rows.Next() {
		u := &domain.User{}
		var lastName sql.NullString
		var referredBy uuid.NullUUID
		var walletUser uuid.NullUUID
		var walletBalance, demoBalance sql.NullFloat64
		var walletUpdated sql.NullTime
		if err := rows.Scan(
			&u.ID, &u.TelegramID, &u.FirstName, &lastName, &u.PhoneNumber,
			&u.ReferalCode, &referredBy, &u.Role, &u.Banned, &u.CreatedAt, &u.UpdatedAt,
			&walletUser, &walletBalance, &demoBalance, &walletUpdated,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan admin user: %w", err)
		}
		if lastName.Valid {
			u.LastName = &lastName.String
		}
		if referredBy.Valid {
			id := referredBy.UUID
			u.ReferredBy = &id
		}
		entry := &domain.UserWithWallet{User: u}
		if walletUser.Valid {
			entry.Wallet = &domain.Wallet{
				UserID:      walletUser.UUID,
				Balance:     walletBalance.Float64,
				DemoBalance: demoBalance.Float64,
				UpdatedAt:   walletUpdated.Time,
			}
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate admin users: %w", err)
	}
	return out, total, nil
}
