package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/bingo/backend/internal/domain"
	"github.com/gin-gonic/gin"
)

type AdminLogHandler struct {
	logs domain.AdminEventLogRepository
}

func NewAdminLogHandler(logs domain.AdminEventLogRepository) *AdminLogHandler {
	return &AdminLogHandler{logs: logs}
}

func (h *AdminLogHandler) List(c *gin.Context) {
	if h.logs == nil {
		c.JSON(http.StatusOK, gin.H{"logs": []any{}, "total": 0, "count": 0})
		return
	}

	level := strings.TrimSpace(c.Query("level"))
	source := strings.TrimSpace(c.Query("source"))

	limit := 50
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	offset := 0
	if v, err := strconv.Atoi(c.Query("offset")); err == nil && v > 0 {
		offset = v
	}

	logs, total, err := h.logs.List(c.Request.Context(), level, source, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch admin logs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs, "total": total, "count": len(logs)})
}
