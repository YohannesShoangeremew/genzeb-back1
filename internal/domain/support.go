package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SupportCategory is the kind of problem a player is reporting. It mirrors the
// category picker in the Mini App and drives triage in the admin dashboard.
type SupportCategory string

const (
	SupportCategoryTransaction SupportCategory = "transaction" // deposit/withdrawal/payment issues
	SupportCategoryGameplay    SupportCategory = "gameplay"    // caller voice, wrong call, lag, etc.
	SupportCategoryOther       SupportCategory = "other"
)

// Valid reports whether c is a category we accept.
func (c SupportCategory) Valid() bool {
	switch c {
	case SupportCategoryTransaction, SupportCategoryGameplay, SupportCategoryOther:
		return true
	default:
		return false
	}
}

// SupportStatus is the triage state of a report.
type SupportStatus string

const (
	SupportStatusOpen     SupportStatus = "open"
	SupportStatusResolved SupportStatus = "resolved"
)

// SupportReport is a player-submitted problem report.
type SupportReport struct {
	ID         uuid.UUID       `json:"id" db:"id"`
	UserID     uuid.UUID       `json:"user_id" db:"user_id"`
	Category   SupportCategory `json:"category" db:"category"`
	Message    string          `json:"message" db:"message"`
	GameID     *uuid.UUID      `json:"game_id,omitempty" db:"game_id"`
	Status     SupportStatus   `json:"status" db:"status"`
	CreatedAt  time.Time       `json:"created_at" db:"created_at"`
	ResolvedAt *time.Time      `json:"resolved_at,omitempty" db:"resolved_at"`
	ResolvedBy *uuid.UUID      `json:"resolved_by,omitempty" db:"resolved_by"`

	// Reporter identity, joined from users for the admin dashboard (populated by
	// List only — zero on the row returned from Create/Submit).
	ReporterFirstName  string  `json:"reporter_first_name,omitempty"`
	ReporterLastName   *string `json:"reporter_last_name,omitempty"`
	ReporterPhone      string  `json:"reporter_phone,omitempty"`
	ReporterTelegramID int64   `json:"reporter_telegram_id,omitempty"`
}

// SubmitReportRequest is the Mini App payload a player POSTs to file a report.
// The reporter is taken from the auth token, not this body.
type SubmitReportRequest struct {
	Category SupportCategory `json:"category" binding:"required"`
	Message  string          `json:"message" binding:"required"`
	GameID   *uuid.UUID      `json:"game_id,omitempty"`
}

// SupportRepository persists and reads player problem reports.
type SupportRepository interface {
	// Create inserts a new report (fills ID, CreatedAt, Status).
	Create(ctx context.Context, report *SupportReport) error
	// List returns reports newest-first, optionally filtered by status, with the
	// reporter's identity joined in for the dashboard.
	List(ctx context.Context, status *SupportStatus, search string, limit, offset int) ([]*SupportReport, error)
	// CountByStatus counts reports, optionally filtered by status (nil = all).
	CountByStatus(ctx context.Context, status *SupportStatus, search string) (int, error)
	// Resolve marks a report resolved by the given admin. Returns true only if a
	// still-open report actually transitioned (idempotent: false if already
	// resolved or missing).
	Resolve(ctx context.Context, id, adminID uuid.UUID) (bool, error)
}
