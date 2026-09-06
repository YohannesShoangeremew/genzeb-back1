package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

// Bonus lifetimes are stored per grant as a minute count, so the day-based
// policy default is converted once here.
const minutesPerDay = 24 * 60

type bonusRepository struct {
	db *sql.DB
}

// NewBonusRepository creates a repository for play-only bonus money.
func NewBonusRepository(db *sql.DB) domain.BonusRepository {
	return &bonusRepository{db: db}
}

// exec runs against the transaction when one is supplied, otherwise the pool.
// Money-moving callers always pass a transaction.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (r *bonusRepository) exec(tx *sql.Tx) execer {
	if tx != nil {
		return tx
	}
	return r.db
}

// Grant awards bonus expiring after the configured number of days.
//
// The deadline is computed by the DATABASE (CURRENT_TIMESTAMP + interval), not
// by the application, so it cannot drift when the app process runs in a
// different timezone than Postgres — the failure mode that silently broke the
// withdrawal day boundary elsewhere in this codebase.
func (r *bonusRepository) Grant(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64, reason string) (*domain.BonusGrant, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("bonus amount must be greater than 0")
	}

	cfg, err := r.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("bonus granting is disabled")
	}

	// The configured day count is the default lifetime, expressed in minutes so
	// a single code path serves both this and the shorter, per-campaign expiry.
	return r.grantMinutes(ctx, tx, userID, amount, reason, cfg.ExpiryDays*minutesPerDay)
}

// GrantWithExpiry awards bonus that lapses after an explicit number of minutes,
// for a campaign that wants urgency (a giveaway good only for the next few
// hours) rather than the general grant lifetime. Enforces the same master
// switch as Grant — a campaign cannot mint bonus while granting is disabled.
func (r *bonusRepository) GrantWithExpiry(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64, reason string, expiresInMinutes int) (*domain.BonusGrant, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("bonus amount must be greater than 0")
	}
	if expiresInMinutes <= 0 {
		return nil, fmt.Errorf("bonus expiry must be greater than 0 minutes")
	}

	cfg, err := r.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("bonus granting is disabled")
	}
	return r.grantMinutes(ctx, tx, userID, amount, reason, expiresInMinutes)
}

// grantMinutes is the shared insert: the deadline is computed by the DATABASE
// (CURRENT_TIMESTAMP + interval), never by the application, so it cannot drift
// when the app process runs in a different timezone than Postgres.
func (r *bonusRepository) grantMinutes(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64, reason string, minutes int) (*domain.BonusGrant, error) {
	var reasonArg any
	if reason != "" {
		reasonArg = reason
	}

	grant := &domain.BonusGrant{ID: uuid.New()}
	query := `
		INSERT INTO bonus_grants (id, user_id, amount, remaining, reason, expires_at)
		VALUES ($1, $2, $3, $3, $4, CURRENT_TIMESTAMP + ($5 || ' minutes')::interval)
		RETURNING id, user_id, amount::float8, remaining::float8, granted_at, expires_at
	`
	err := r.exec(tx).QueryRowContext(ctx, query, grant.ID, userID, amount, reasonArg, minutes).Scan(
		&grant.ID, &grant.UserID, &grant.Amount, &grant.Remaining, &grant.GrantedAt, &grant.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to grant bonus: %w", err)
	}
	if reason != "" {
		grant.Reason = &reason
	}
	return grant, nil
}

// Balance sums the live grants. Expired ones are excluded by the WHERE clause
// rather than by a cleanup job, so the figure is correct the instant a grant
// lapses.
func (r *bonusRepository) Balance(ctx context.Context, userID uuid.UUID) (*domain.BonusBalance, error) {
	query := `
		SELECT COALESCE(SUM(remaining), 0)::float8, MIN(expires_at)
		FROM bonus_grants
		WHERE user_id = $1 AND remaining > 0 AND expires_at > CURRENT_TIMESTAMP
	`
	var amount float64
	var next sql.NullTime
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&amount, &next); err != nil {
		return nil, fmt.Errorf("failed to read bonus balance: %w", err)
	}
	bal := &domain.BonusBalance{Amount: amount}
	if next.Valid {
		bal.NextExpiry = &next.Time
	}
	return bal, nil
}

// SpendableForUpdate sums live bonus with the grant rows locked, so the caller
// can plan a purchase against a figure that cannot change underneath it.
func (r *bonusRepository) SpendableForUpdate(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (float64, error) {
	if tx == nil {
		return 0, fmt.Errorf("SpendableForUpdate requires a transaction")
	}
	// Lock the individual rows first; an aggregate cannot be locked directly.
	rows, err := tx.QueryContext(ctx, `
		SELECT remaining::float8
		FROM bonus_grants
		WHERE user_id = $1 AND remaining > 0 AND expires_at > CURRENT_TIMESTAMP
		FOR UPDATE
	`, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to lock bonus grants: %w", err)
	}
	defer rows.Close()

	var total float64
	for rows.Next() {
		var remaining float64
		if err := rows.Scan(&remaining); err != nil {
			return 0, fmt.Errorf("failed to scan bonus grant: %w", err)
		}
		total += remaining
	}
	return total, rows.Err()
}

// ConsumeForStake spends live bonus soonest-expiring-first.
//
// Soonest-first matters: spending the grant with the most time left would
// strand the one about to lapse, so a player would lose bonus they could have
// used. The rows are locked FOR UPDATE, so two concurrent stakes serialize and
// the second sees the first's deduction — without that, both could read the
// same remaining balance and overspend a grant.
func (r *bonusRepository) ConsumeForStake(ctx context.Context, tx *sql.Tx, userID uuid.UUID, unitPrice float64, maxUnits int) (int, *time.Time, error) {
	if tx == nil {
		return 0, nil, fmt.Errorf("ConsumeForStake requires a transaction")
	}
	if unitPrice <= 0 || maxUnits <= 0 {
		return 0, nil, nil
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, remaining::float8, expires_at
		FROM bonus_grants
		WHERE user_id = $1 AND remaining > 0 AND expires_at > CURRENT_TIMESTAMP
		ORDER BY expires_at ASC, granted_at ASC
		FOR UPDATE
	`, userID)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to lock bonus grants: %w", err)
	}

	type liveGrant struct {
		id        uuid.UUID
		remaining float64
		expiresAt time.Time
	}
	var grants []liveGrant
	for rows.Next() {
		var g liveGrant
		if err := rows.Scan(&g.id, &g.remaining, &g.expiresAt); err != nil {
			rows.Close()
			return 0, nil, fmt.Errorf("failed to scan bonus grant: %w", err)
		}
		grants = append(grants, g)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, nil, fmt.Errorf("failed to read bonus grants: %w", err)
	}
	rows.Close()

	// How many whole cards the live bonus can cover. Summing first, then
	// spending, is what keeps the spend to whole units — a grant remainder too
	// small for a card stays put rather than being stranded mid-purchase.
	var available float64
	for _, g := range grants {
		available += g.remaining
	}
	units := int(available / unitPrice)
	if units > maxUnits {
		units = maxUnits
	}
	if units == 0 {
		return 0, nil, nil
	}

	target := float64(units) * unitPrice
	var consumed float64
	var earliest *time.Time
	for _, g := range grants {
		if consumed >= target {
			break
		}
		take := target - consumed
		if take > g.remaining {
			take = g.remaining
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE bonus_grants SET remaining = remaining - $2 WHERE id = $1`, g.id, take,
		); err != nil {
			return 0, nil, fmt.Errorf("failed to consume bonus grant: %w", err)
		}
		consumed += take
		if earliest == nil {
			exp := g.expiresAt
			earliest = &exp
		}
	}
	return units, earliest, nil
}

// Restore returns refunded bonus under its ORIGINAL deadline.
//
// Reinstating it with a fresh deadline would let a player park bonus in a game
// and leave, over and over, to keep it alive indefinitely. A deadline that has
// already passed is dropped rather than resurrected — the player would not
// have been able to spend that bonus anyway.
func (r *bonusRepository) Restore(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64, expiresAt time.Time, reason string) error {
	if tx == nil {
		return fmt.Errorf("Restore requires a transaction")
	}
	if amount <= 0 {
		return nil
	}

	var reasonArg any
	if reason != "" {
		reasonArg = reason
	}
	// The WHERE guard drops refunds whose deadline has already passed.
	//
	// $5 is cast explicitly because it appears both as an inserted value and in
	// a comparison; without the cast Postgres cannot deduce one type for it and
	// rejects the statement outright ("inconsistent types deduced"), which
	// would fail the enclosing cancel or leave rather than just the refund.
	_, err := tx.ExecContext(ctx, `
		INSERT INTO bonus_grants (id, user_id, amount, remaining, reason, expires_at)
		SELECT $1, $2, $3, $3, $4, $5::timestamp
		WHERE $5::timestamp > CURRENT_TIMESTAMP
	`, uuid.New(), userID, amount, reasonArg, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to restore bonus: %w", err)
	}
	return nil
}

func (r *bonusRepository) ListGrants(ctx context.Context, userID uuid.UUID, limit int) ([]*domain.BonusGrant, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, amount::float8, remaining::float8, reason, granted_at, expires_at
		FROM bonus_grants
		WHERE user_id = $1
		ORDER BY granted_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list bonus grants: %w", err)
	}
	defer rows.Close()

	grants := make([]*domain.BonusGrant, 0)
	for rows.Next() {
		g := &domain.BonusGrant{}
		var reason sql.NullString
		if err := rows.Scan(&g.ID, &g.UserID, &g.Amount, &g.Remaining, &reason, &g.GrantedAt, &g.ExpiresAt); err != nil {
			return nil, fmt.Errorf("failed to scan bonus grant: %w", err)
		}
		if reason.Valid {
			g.Reason = &reason.String
		}
		grants = append(grants, g)
	}
	return grants, rows.Err()
}

// TotalOutstanding is the house's live bonus liability — what players could
// still stake. Excludes expired grants, which cost nothing.
func (r *bonusRepository) TotalOutstanding(ctx context.Context) (float64, error) {
	var total float64
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(remaining), 0)::float8
		FROM bonus_grants
		WHERE remaining > 0 AND expires_at > CURRENT_TIMESTAMP
	`).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to total outstanding bonus: %w", err)
	}
	return total, nil
}

func (r *bonusRepository) GetConfig(ctx context.Context) (*domain.BonusConfig, error) {
	cfg := &domain.BonusConfig{}
	err := r.db.QueryRowContext(ctx, `
		SELECT enabled, expiry_days, announcement, updated_at FROM bonus_config WHERE id = 1
	`).Scan(&cfg.Enabled, &cfg.ExpiryDays, &cfg.Announcement, &cfg.UpdatedAt)
	if err == sql.ErrNoRows {
		// Row missing (migration not applied yet) — fail closed on granting
		// rather than error, matching how bot_config degrades.
		return &domain.BonusConfig{Enabled: false, ExpiryDays: 7}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read bonus config: %w", err)
	}
	return cfg, nil
}

func (r *bonusRepository) UpdateConfig(ctx context.Context, cfg *domain.BonusConfig) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO bonus_config (id, enabled, expiry_days, announcement, updated_at)
		VALUES (1, $1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			expiry_days = EXCLUDED.expiry_days,
			announcement = EXCLUDED.announcement,
			updated_at = CURRENT_TIMESTAMP
	`, cfg.Enabled, cfg.ExpiryDays, cfg.Announcement)
	if err != nil {
		return fmt.Errorf("failed to update bonus config: %w", err)
	}
	return nil
}

// GrantDepositOnce atomically reserves the deposit-transaction award key and
// creates its bonus grant. If the key already exists, no grant is created and
// false is returned. The deposit-specific Settings switch controls issuance;
// the bonus policy supplies only the expiry period.
func (r *bonusRepository) GrantDepositOnce(ctx context.Context, tx *sql.Tx, transactionID, userID uuid.UUID, amount float64) (bool, error) {
	if tx == nil {
		return false, fmt.Errorf("GrantDepositOnce requires a transaction")
	}
	if amount <= 0 {
		return false, nil
	}

	grantID := uuid.New()
	grant := &domain.BonusGrant{}
	err := tx.QueryRowContext(ctx, `
		WITH cfg AS (
			SELECT COALESCE((SELECT expiry_days FROM bonus_config WHERE id = 1), 7) AS expiry_days
		), award AS (
			INSERT INTO deposit_bonus_awards (deposit_transaction_id, user_id, bonus_grant_id, amount)
			SELECT $1, $2, $3, $4 FROM cfg
			ON CONFLICT (deposit_transaction_id) DO NOTHING
			RETURNING bonus_grant_id
		)
		INSERT INTO bonus_grants (id, user_id, amount, remaining, reason, expires_at)
		SELECT $3, $2, $4, $4, 'deposit reward',
		       CURRENT_TIMESTAMP + ((SELECT expiry_days FROM cfg) * 1440 || ' minutes')::interval
		FROM award
		RETURNING id, user_id, amount::float8, remaining::float8, granted_at, expires_at
	`, transactionID, userID, grantID, amount).Scan(
		&grant.ID, &grant.UserID, &grant.Amount, &grant.Remaining, &grant.GrantedAt, &grant.ExpiresAt,
	)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to grant deposit reward: %w", err)
	}
	return true, nil
}

// GrantReferralOnce marks an invitee's referral as processed and grants the
// referrer play-only bonus in the same transaction. The guarded UPDATE is the
// one-time claim: concurrent first deposits cannot both create a grant.
func (r *bonusRepository) GrantReferralOnce(ctx context.Context, tx *sql.Tx, inviteeID, referrerID uuid.UUID, amount float64) (bool, error) {
	if tx == nil {
		return false, fmt.Errorf("GrantReferralOnce requires a transaction")
	}
	if amount <= 0 {
		return false, nil
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET referral_rewarded = true
		WHERE id = $1 AND referred_by = $2 AND referral_rewarded = false
	`, inviteeID, referrerID)
	if err != nil {
		return false, fmt.Errorf("failed to claim referral reward: %w", err)
	}
	claimed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to inspect referral claim: %w", err)
	}
	if claimed == 0 {
		return false, nil
	}

	grantID := uuid.New()
	grant := &domain.BonusGrant{}
	err = tx.QueryRowContext(ctx, `
		WITH cfg AS (
			SELECT COALESCE((SELECT expiry_days FROM bonus_config WHERE id = 1), 7) AS expiry_days
		)
		INSERT INTO bonus_grants (id, user_id, amount, remaining, reason, expires_at)
		VALUES ($1, $2, $3, $3, 'Referral reward — invited player made first deposit',
		        CURRENT_TIMESTAMP + ((SELECT expiry_days FROM cfg) * 1440 || ' minutes')::interval)
		RETURNING id, user_id, amount::float8, remaining::float8, granted_at, expires_at
	`, grantID, referrerID, amount).Scan(
		&grant.ID, &grant.UserID, &grant.Amount, &grant.Remaining, &grant.GrantedAt, &grant.ExpiresAt,
	)
	if err != nil {
		return false, fmt.Errorf("failed to grant referral reward: %w", err)
	}
	return true, nil
}
