package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type bonusCampaignRepository struct {
	db *sql.DB
}

// NewBonusCampaignRepository creates a repository for first-N bonus giveaways.
func NewBonusCampaignRepository(db *sql.DB) domain.BonusCampaignRepository {
	return &bonusCampaignRepository{db: db}
}

func (r *bonusCampaignRepository) exec(tx *sql.Tx) execer {
	if tx != nil {
		return tx
	}
	return r.db
}

const bonusCampaignColumns = `id, total_amount, slots, amount_per_slot, claimed_count,
	announcement, expiry_minutes, status, created_by, created_at, ended_at`

func scanBonusCampaign(row interface{ Scan(...any) error }) (*domain.BonusCampaign, error) {
	var c domain.BonusCampaign
	var createdBy sql.NullString
	var endedAt sql.NullTime
	var expiryMinutes sql.NullInt64
	if err := row.Scan(
		&c.ID, &c.Amount, &c.Slots, &c.AmountPerSlot, &c.ClaimedCount,
		&c.Announcement, &expiryMinutes, &c.Status, &createdBy, &c.CreatedAt, &endedAt,
	); err != nil {
		return nil, err
	}
	if expiryMinutes.Valid {
		m := int(expiryMinutes.Int64)
		c.ExpiryMinutes = &m
	}
	if createdBy.Valid {
		if id, err := uuid.Parse(createdBy.String); err == nil {
			c.CreatedBy = &id
		}
	}
	if endedAt.Valid {
		t := endedAt.Time
		c.EndedAt = &t
	}
	return &c, nil
}

func (r *bonusCampaignRepository) Create(ctx context.Context, c *domain.BonusCampaign) error {
	var createdBy any
	if c.CreatedBy != nil {
		createdBy = *c.CreatedBy
	}
	// created_at is RETURNED rather than assumed: the database writes it (see
	// the clocks note in migration 032), so without reading it back the caller
	// gets Go's zero time and the admin screen shows "started year 1" until
	// something else refetches.
	var expiryMinutes any
	if c.ExpiryMinutes != nil {
		expiryMinutes = *c.ExpiryMinutes
	}
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO bonus_campaigns
			(id, total_amount, slots, amount_per_slot, announcement, expiry_minutes, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, 'active', $7)
		RETURNING created_at`,
		c.ID, c.Amount, c.Slots, c.AmountPerSlot, c.Announcement, expiryMinutes, createdBy,
	).Scan(&c.CreatedAt)
	if err != nil {
		// The partial unique index on status='active' is what refuses a second
		// live campaign. Translated here so the admin sees the actual problem
		// rather than a raw constraint name.
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return fmt.Errorf("a bonus campaign is already running — end it before starting another")
		}
		return fmt.Errorf("failed to create bonus campaign: %w", err)
	}
	c.Status = domain.BonusCampaignStatusActive
	return nil
}

func (r *bonusCampaignRepository) Active(ctx context.Context) (*domain.BonusCampaign, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+bonusCampaignColumns+` FROM bonus_campaigns WHERE status = 'active'`)
	c, err := scanBonusCampaign(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read active campaign: %w", err)
	}
	return c, nil
}

// ActiveForUpdate locks the campaign row for the duration of the caller's
// transaction. Every claimer queues on this lock, which is what turns a
// stampede into an exact first-N ordering.
func (r *bonusCampaignRepository) ActiveForUpdate(ctx context.Context, tx *sql.Tx) (*domain.BonusCampaign, error) {
	if tx == nil {
		return nil, fmt.Errorf("ActiveForUpdate requires a transaction")
	}
	row := tx.QueryRowContext(ctx,
		`SELECT `+bonusCampaignColumns+` FROM bonus_campaigns WHERE status = 'active' FOR UPDATE`)
	c, err := scanBonusCampaign(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to lock active campaign: %w", err)
	}
	return c, nil
}

func (r *bonusCampaignRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.BonusCampaign, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+bonusCampaignColumns+` FROM bonus_campaigns WHERE id = $1`, id)
	c, err := scanBonusCampaign(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read campaign: %w", err)
	}
	return c, nil
}

func (r *bonusCampaignRepository) List(ctx context.Context, limit int) ([]*domain.BonusCampaign, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+bonusCampaignColumns+` FROM bonus_campaigns ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list campaigns: %w", err)
	}
	defer rows.Close()

	campaigns := make([]*domain.BonusCampaign, 0, limit)
	for rows.Next() {
		c, err := scanBonusCampaign(rows)
		if err != nil {
			return nil, err
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, rows.Err()
}

// RecordClaim takes a slot and writes the claim.
//
// The UPDATE carries `claimed_count < slots` in its WHERE clause, so the cap
// holds even if a caller reaches here without the row lock: no matching row
// means the slots are gone. The INSERT's primary key catches a double claim.
// Both are database-enforced, so neither depends on the checks the usecase
// makes first — those exist to produce a clean message, not to be the guard.
func (r *bonusCampaignRepository) RecordClaim(ctx context.Context, tx *sql.Tx, campaignID, userID uuid.UUID, amount float64, grantID uuid.UUID) (int, error) {
	if tx == nil {
		return 0, fmt.Errorf("RecordClaim requires a transaction")
	}

	var position int
	err := tx.QueryRowContext(ctx, `
		UPDATE bonus_campaigns
		   SET claimed_count = claimed_count + 1
		 WHERE id = $1 AND status = 'active' AND claimed_count < slots
		RETURNING claimed_count`, campaignID).Scan(&position)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, domain.ErrCampaignExhausted
	}
	if err != nil {
		return 0, fmt.Errorf("failed to take a bonus slot: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO bonus_campaign_claims
			(campaign_id, user_id, grant_id, amount, position)
		VALUES ($1, $2, $3, $4, $5)`,
		campaignID, userID, grantID, amount, position,
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return 0, domain.ErrCampaignAlreadyClaimed
		}
		return 0, fmt.Errorf("failed to record bonus claim: %w", err)
	}
	return position, nil
}

func (r *bonusCampaignRepository) FindClaim(ctx context.Context, campaignID, userID uuid.UUID) (*domain.BonusCampaignClaim, error) {
	var cl domain.BonusCampaignClaim
	var grantID sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT campaign_id, user_id, grant_id, amount, position, claimed_at
		  FROM bonus_campaign_claims
		 WHERE campaign_id = $1 AND user_id = $2`, campaignID, userID,
	).Scan(&cl.CampaignID, &cl.UserID, &grantID, &cl.Amount, &cl.Position, &cl.ClaimedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read claim: %w", err)
	}
	if grantID.Valid {
		if id, perr := uuid.Parse(grantID.String); perr == nil {
			cl.GrantID = &id
		}
	}
	return &cl, nil
}

func (r *bonusCampaignRepository) ListClaims(ctx context.Context, campaignID uuid.UUID) ([]*domain.BonusCampaignClaim, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.campaign_id, c.user_id, c.grant_id, c.amount, c.position, c.claimed_at,
		       u.first_name, u.phone_number
		  FROM bonus_campaign_claims c
		  JOIN users u ON u.id = c.user_id
		 WHERE c.campaign_id = $1
		 ORDER BY c.position ASC`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("failed to list claims: %w", err)
	}
	defer rows.Close()

	claims := make([]*domain.BonusCampaignClaim, 0)
	for rows.Next() {
		var cl domain.BonusCampaignClaim
		var grantID sql.NullString
		if err := rows.Scan(
			&cl.CampaignID, &cl.UserID, &grantID, &cl.Amount, &cl.Position, &cl.ClaimedAt,
			&cl.Name, &cl.Phone,
		); err != nil {
			return nil, err
		}
		if grantID.Valid {
			if id, perr := uuid.Parse(grantID.String); perr == nil {
				cl.GrantID = &id
			}
		}
		claims = append(claims, &cl)
	}
	return claims, rows.Err()
}

// End closes a campaign, and is a no-op on one already ended — the last claim
// exhausting the slots and an admin pressing Stop can legitimately race, and
// neither should see an error for losing.
func (r *bonusCampaignRepository) End(ctx context.Context, tx *sql.Tx, id uuid.UUID) error {
	_, err := r.exec(tx).ExecContext(ctx, `
		UPDATE bonus_campaigns
		   SET status = 'ended', ended_at = CURRENT_TIMESTAMP
		 WHERE id = $1 AND status = 'active'`, id)
	if err != nil {
		return fmt.Errorf("failed to end campaign: %w", err)
	}
	return nil
}

// HasCompletedDeposit reports whether this player has ever completed a real
// money-in transaction.
//
// Deliberately category = 'deposit' and not merely type = 'deposit': type is
// shared by prizes, refunds and admin credits (see the comment in
// domain/transaction.go), so keying on it would let a player bootstrap
// eligibility from a bonus they were given, which is the loop this check
// exists to prevent.
func (r *bonusCampaignRepository) HasCompletedDeposit(ctx context.Context, userID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM transactions
			 WHERE user_id = $1 AND category = 'deposit' AND status = 'completed'
		)`, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check deposit history: %w", err)
	}
	return exists, nil
}
