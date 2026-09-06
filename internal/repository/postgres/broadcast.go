package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

type broadcastRepository struct {
	db *sql.DB
}

func NewBroadcastRepository(db *sql.DB) domain.BroadcastRepository {
	return &broadcastRepository{db: db}
}

func (r *broadcastRepository) Create(ctx context.Context, b *domain.Broadcast) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	query := `
		INSERT INTO broadcasts (id, message, recipients, sent, failed, status, created_by)
		VALUES ($1, $2, $3, 0, 0, $4, $5)
		RETURNING created_at, updated_at
	`
	err := r.db.QueryRowContext(ctx, query, b.ID, b.Message, b.Recipients, b.Status, b.CreatedBy).
		Scan(&b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create broadcast: %w", err)
	}
	return nil
}

// Recipients lists who can actually receive a Telegram message.
//
// Filler bots are excluded twice over — by is_bot and by the telegram_id sign.
// Their synthetic ids are large NEGATIVE numbers (see botTelegramIDBase), so
// sending to them would not merely be pointless: those chat ids do not exist,
// and every one would come back an error and inflate the failure count.
func (r *broadcastRepository) Recipients(ctx context.Context) ([]domain.BroadcastRecipient, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, telegram_id
		FROM users
		WHERE is_bot = false AND telegram_id > 0 AND banned = false
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list broadcast recipients: %w", err)
	}
	defer rows.Close()

	recipients := make([]domain.BroadcastRecipient, 0)
	for rows.Next() {
		var rec domain.BroadcastRecipient
		if err := rows.Scan(&rec.UserID, &rec.TelegramID); err != nil {
			return nil, fmt.Errorf("failed to scan recipient: %w", err)
		}
		recipients = append(recipients, rec)
	}
	return recipients, rows.Err()
}

func (r *broadcastRepository) UpdateProgress(ctx context.Context, id uuid.UUID, sent, failed int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE broadcasts SET sent = $2, failed = $3, updated_at = CURRENT_TIMESTAMP WHERE id = $1
	`, id, sent, failed)
	if err != nil {
		return fmt.Errorf("failed to update broadcast progress: %w", err)
	}
	return nil
}

func (r *broadcastRepository) Finish(ctx context.Context, id uuid.UUID, status domain.BroadcastStatus, sent, failed int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE broadcasts
		SET sent = $2, failed = $3, status = $4,
		    updated_at = CURRENT_TIMESTAMP, finished_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, id, sent, failed, status)
	if err != nil {
		return fmt.Errorf("failed to finish broadcast: %w", err)
	}
	return nil
}

func (r *broadcastRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Broadcast, error) {
	b := &domain.Broadcast{}
	err := r.db.QueryRowContext(ctx, `
		SELECT id, message, recipients, sent, failed, status, created_by, created_at, updated_at, finished_at
		FROM broadcasts WHERE id = $1
	`, id).Scan(&b.ID, &b.Message, &b.Recipients, &b.Sent, &b.Failed, &b.Status,
		&b.CreatedBy, &b.CreatedAt, &b.UpdatedAt, &b.FinishedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("broadcast not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read broadcast: %w", err)
	}
	return b, nil
}

func (r *broadcastRepository) List(ctx context.Context, limit int) ([]*domain.Broadcast, error) {
	if limit <= 0 {
		limit = 25
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, message, recipients, sent, failed, status, created_by, created_at, updated_at, finished_at
		FROM broadcasts ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list broadcasts: %w", err)
	}
	defer rows.Close()

	out := make([]*domain.Broadcast, 0)
	for rows.Next() {
		b := &domain.Broadcast{}
		if err := rows.Scan(&b.ID, &b.Message, &b.Recipients, &b.Sent, &b.Failed, &b.Status,
			&b.CreatedBy, &b.CreatedAt, &b.UpdatedAt, &b.FinishedAt); err != nil {
			return nil, fmt.Errorf("failed to scan broadcast: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
