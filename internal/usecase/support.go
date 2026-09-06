package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

// maxSupportMessageLen bounds a report body so a single submission can't store
// an unbounded blob. Generous enough for a full description.
const maxSupportMessageLen = 2000

// SupportUseCase handles player problem reports.
type SupportUseCase struct {
	repo domain.SupportRepository
}

// NewSupportUseCase wires the support use case.
func NewSupportUseCase(repo domain.SupportRepository) *SupportUseCase {
	return &SupportUseCase{repo: repo}
}

// Submit files a new report for the authenticated user. The reporter identity
// comes from userID (the JWT), never the request body.
func (u *SupportUseCase) Submit(ctx context.Context, userID uuid.UUID, req domain.SubmitReportRequest) (*domain.SupportReport, error) {
	if !req.Category.Valid() {
		return nil, errors.New("invalid category")
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		return nil, errors.New("message is required")
	}
	if len(msg) > maxSupportMessageLen {
		msg = msg[:maxSupportMessageLen]
	}

	report := &domain.SupportReport{
		UserID:   userID,
		Category: req.Category,
		Message:  msg,
		GameID:   req.GameID,
	}
	if err := u.repo.Create(ctx, report); err != nil {
		return nil, err
	}
	return report, nil
}

// List returns reports for the dashboard, optionally filtered by status.
func (u *SupportUseCase) List(ctx context.Context, status *domain.SupportStatus, search string, limit, offset int) ([]*domain.SupportReport, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return u.repo.List(ctx, status, search, limit, offset)
}

// Count returns how many reports match the optional status filter.
func (u *SupportUseCase) Count(ctx context.Context, status *domain.SupportStatus, search string) (int, error) {
	return u.repo.CountByStatus(ctx, status, search)
}

// Resolve marks a report resolved by an admin. Returns true only if a still-open
// report actually transitioned.
func (u *SupportUseCase) Resolve(ctx context.Context, id, adminID uuid.UUID) (bool, error) {
	return u.repo.Resolve(ctx, id, adminID)
}
