package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AdminEventLog is a structured operational event shown only in the admin
// dashboard. It is for warnings/operators, not player-facing game history.
type AdminEventLog struct {
	ID        uuid.UUID      `json:"id" db:"id"`
	Level     string         `json:"level" db:"level"`
	Source    string         `json:"source" db:"source"`
	Message   string         `json:"message" db:"message"`
	GameID    *uuid.UUID     `json:"game_id,omitempty" db:"game_id"`
	Metadata  map[string]any `json:"metadata,omitempty" db:"metadata"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
}

// AdminEventLogRepository persists and reads admin-visible operational logs.
// Record is best-effort and must never break the action that produced the log.
type AdminEventLogRepository interface {
	Record(ctx context.Context, entry *AdminEventLog)
	List(ctx context.Context, level, source string, limit, offset int) ([]*AdminEventLog, int, error)
}
