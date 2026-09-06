package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

// supportRepository persists player problem reports.
type supportRepository struct {
	db *sql.DB
}

// NewSupportRepository creates a new support-report repository.
func NewSupportRepository(db *sql.DB) domain.SupportRepository {
	return &supportRepository{db: db}
}

// Create inserts a new report and fills the DB-assigned fields back onto it.
func (r *supportRepository) Create(ctx context.Context, report *domain.SupportReport) error {
	query := `
		INSERT INTO support_reports (user_id, category, message, game_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, status, created_at
	`
	err := r.db.QueryRowContext(ctx, query,
		report.UserID, report.Category, report.Message, report.GameID,
	).Scan(&report.ID, &report.Status, &report.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create support report: %w", err)
	}
	return nil
}

// List returns reports newest-first, optionally filtered by status, joining the
// reporter's identity for the dashboard.
func (r *supportRepository) List(ctx context.Context, status *domain.SupportStatus, search string, limit, offset int) ([]*domain.SupportReport, error) {
	query := `
		SELECT sr.id, sr.user_id, sr.category, sr.message, sr.game_id, sr.status,
		       sr.created_at, sr.resolved_at, sr.resolved_by,
		       u.first_name, u.last_name, u.phone_number, u.telegram_id
		FROM support_reports sr
		JOIN users u ON u.id = sr.user_id
		WHERE ($1::text IS NULL OR sr.status = $1)
		  AND ($2 = '' OR concat_ws(' ', sr.id::text, sr.user_id::text, sr.category::text,
		      sr.status::text, sr.message, sr.game_id::text, u.first_name, u.last_name,
		      u.phone_number, u.telegram_id::text) ILIKE '%' || $2 || '%')
		ORDER BY sr.created_at DESC
		LIMIT $3 OFFSET $4
	`
	rows, err := r.db.QueryContext(ctx, query, status, strings.TrimSpace(search), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list support reports: %w", err)
	}
	defer rows.Close()

	reports := make([]*domain.SupportReport, 0)
	for rows.Next() {
		report := &domain.SupportReport{}
		var lastName sql.NullString
		if err := rows.Scan(
			&report.ID,
			&report.UserID,
			&report.Category,
			&report.Message,
			&report.GameID,
			&report.Status,
			&report.CreatedAt,
			&report.ResolvedAt,
			&report.ResolvedBy,
			&report.ReporterFirstName,
			&lastName,
			&report.ReporterPhone,
			&report.ReporterTelegramID,
		); err != nil {
			return nil, fmt.Errorf("failed to scan support report: %w", err)
		}
		if lastName.Valid {
			report.ReporterLastName = &lastName.String
		}
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

// CountByStatus counts reports after applying the same status/search filter.
func (r *supportRepository) CountByStatus(ctx context.Context, status *domain.SupportStatus, search string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM support_reports sr
		JOIN users u ON u.id = sr.user_id
		WHERE ($1::text IS NULL OR sr.status = $1)
		  AND ($2 = '' OR concat_ws(' ', sr.id::text, sr.user_id::text, sr.category::text,
		      sr.status::text, sr.message, sr.game_id::text, u.first_name, u.last_name,
		      u.phone_number, u.telegram_id::text) ILIKE '%' || $2 || '%')
	`
	var count int
	if err := r.db.QueryRowContext(ctx, query, status, strings.TrimSpace(search)).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count support reports: %w", err)
	}
	return count, nil
}

// Resolve marks a still-open report resolved. The status guard makes it
// idempotent: an already-resolved (or missing) report affects no rows.
func (r *supportRepository) Resolve(ctx context.Context, id, adminID uuid.UUID) (bool, error) {
	query := `
		UPDATE support_reports
		SET status = 'resolved', resolved_at = CURRENT_TIMESTAMP, resolved_by = $2
		WHERE id = $1 AND status = 'open'
	`
	res, err := r.db.ExecContext(ctx, query, id, adminID)
	if err != nil {
		return false, fmt.Errorf("failed to resolve support report: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read resolve result: %w", err)
	}
	return n > 0, nil
}
