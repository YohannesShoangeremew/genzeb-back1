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

// BroadcastHandler exposes admin-only Telegram broadcasts.
type BroadcastHandler struct {
	broadcastUseCase *usecase.BroadcastUseCase
}

func NewBroadcastHandler(broadcastUseCase *usecase.BroadcastUseCase) *BroadcastHandler {
	return &BroadcastHandler{broadcastUseCase: broadcastUseCase}
}

// Send handles POST /admin/broadcast.
//
// Returns 202 Accepted, not 200: the message has been queued and the sending
// is still in flight when this responds. The body carries the run id so the
// dashboard can poll progress.
func (h *BroadcastHandler) Send(c *gin.Context) {
	var req domain.SendBroadcastRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var createdBy *uuid.UUID
	if id, ok := middleware.GetUserID(c); ok {
		createdBy = &id
	}

	b, err := h.broadcastUseCase.Send(c.Request.Context(), req.Message, createdBy)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"broadcast": b})
}

// Get handles GET /admin/broadcast/:id — progress for one run.
func (h *BroadcastHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid broadcast id"})
		return
	}
	b, err := h.broadcastUseCase.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"broadcast": b})
}

// List handles GET /admin/broadcasts — recent runs.
func (h *BroadcastHandler) List(c *gin.Context) {
	limit := 25
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	items, err := h.broadcastUseCase.List(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"broadcasts": items})
}

// Audience handles GET /admin/broadcast/audience — how many players a
// broadcast would reach, so the admin sees the blast radius before sending.
func (h *BroadcastHandler) Audience(c *gin.Context) {
	n, err := h.broadcastUseCase.RecipientCount(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"recipients": n})
}
