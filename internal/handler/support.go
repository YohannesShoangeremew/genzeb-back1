package handler

import (
	"net/http"
	"strconv"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/middleware"
	"github.com/bingo/backend/internal/usecase"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SupportHandler exposes the player "report a problem" endpoint and the admin
// endpoints that read and resolve those reports.
type SupportHandler struct {
	supportUseCase *usecase.SupportUseCase
}

// NewSupportHandler creates a new support handler.
func NewSupportHandler(supportUseCase *usecase.SupportUseCase) *SupportHandler {
	return &SupportHandler{supportUseCase: supportUseCase}
}

// SubmitReport handles POST /support — a player files a problem report. The
// reporter is the authenticated user (from the JWT), not the request body.
func (h *SupportHandler) SubmitReport(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	var req domain.SubmitReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	report, err := h.supportUseCase.Submit(c.Request.Context(), userID, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, report)
}

// ListReports handles GET /admin/support — reports for the dashboard, optionally
// filtered by ?status=open|resolved, with ?limit and ?offset for paging.
func (h *SupportHandler) ListReports(c *gin.Context) {
	var status *domain.SupportStatus
	if s := c.Query("status"); s != "" {
		st := domain.SupportStatus(s)
		if st != domain.SupportStatusOpen && st != domain.SupportStatusResolved {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
		status = &st
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	search := c.Query("search")
	reports, err := h.supportUseCase.List(c.Request.Context(), status, search, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	count, err := h.supportUseCase.Count(c.Request.Context(), status, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"reports": reports, "count": count})
}

// ResolveReport handles POST /admin/support/:id/resolve — mark a report handled.
func (h *SupportHandler) ResolveReport(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report ID"})
		return
	}
	adminID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	changed, err := h.supportUseCase.Resolve(c.Request.Context(), id, adminID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !changed {
		c.JSON(http.StatusNotFound, gin.H{"error": "report not found or already resolved"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Report resolved"})
}
