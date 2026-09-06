package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

type adminEventLogRepository struct {
	db *sql.DB
}

func NewAdminEventLogRepository(db *sql.DB) domain.AdminEventLogRepository {
	return &adminEventLogRepository{db: db}
}

func (r *adminEventLogRepository) Record(ctx context.Context, entry *domain.AdminEventLog) {
	if entry == nil {
		return
	}
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}

	metadata := []byte("{}")
	if entry.Metadata != nil {
		if b, err := json.Marshal(entry.Metadata); err == nil {
			metadata = b
		}
	}

	_, _ = r.db.ExecContext(ctx, `
		INSERT INTO admin_event_logs (id, level, source, message, game_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)
	`, entry.ID, entry.Level, entry.Source, entry.Message, entry.GameID, string(metadata))
}

func (r *adminEventLogRepository) List(ctx context.Context, level, source string, limit, offset int) ([]*domain.AdminEventLog, int, error) {
	level = strings.TrimSpace(level)
	source = strings.TrimSpace(source)

	where := `WHERE ($1 = '' OR level = $1) AND ($2 = '' OR source = $2)`

	var total int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM admin_event_logs `+where, level, source,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count admin event logs: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, level, source, message, game_id, metadata, created_at
		FROM admin_event_logs `+where+`
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, level, source, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list admin event logs: %w", err)
	}
	defer rows.Close()

	logs := make([]*domain.AdminEventLog, 0)
	for rows.Next() {
		entry := &domain.AdminEventLog{}
		var gameID uuid.NullUUID
		var metadata []byte
		if err := rows.Scan(
			&entry.ID,
			&entry.Level,
			&entry.Source,
			&entry.Message,
			&gameID,
			&metadata,
			&entry.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan admin event log: %w", err)
		}
		if gameID.Valid {
			entry.GameID = &gameID.UUID
		}
		if len(metadata) > 0 {
			var m map[string]any
			if err := json.Unmarshal(metadata, &m); err == nil {
				entry.Metadata = m
			}
		}
		logs = append(logs, entry)
	}
	return logs, total, rows.Err()
}
