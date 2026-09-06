package handler

import (
	"net/http"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/usecase"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// BotHandler exposes admin-only endpoints to control the filler bots.
type BotHandler struct {
	botUseCase *usecase.BotUseCase
}

// NewBotHandler creates a new bot handler.
func NewBotHandler(botUseCase *usecase.BotUseCase) *BotHandler {
	return &BotHandler{botUseCase: botUseCase}
}

// GetConfig handles GET /admin/bots/config — the auto-fill policy.
func (h *BotHandler) GetConfig(c *gin.Context) {
	cfg, err := h.botUseCase.GetConfig(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// UpdateConfig handles PUT /admin/bots/config — edit the auto-fill policy.
func (h *BotHandler) UpdateConfig(c *gin.Context) {
	var req domain.UpdateBotConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg, err := h.botUseCase.UpdateConfig(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// SeedPool handles POST /admin/bots/seed — (re)create + fund the bot pool.
func (h *BotHandler) SeedPool(c *gin.Context) {
	var req struct {
		Count int `json:"count"`
	}
	_ = c.ShouldBindJSON(&req) // body optional; 0 → use configured pool size

	if err := h.botUseCase.SeedPool(c.Request.Context(), req.Count); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Bot pool seeded and funded"})
}

// AddBots handles POST /admin/games/:gameId/add-bots — manually inject bots.
func (h *BotHandler) AddBots(c *gin.Context) {
	gameID, err := uuid.Parse(c.Param("gameId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid game ID"})
		return
	}
	var req domain.AddBotsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.botUseCase.FillGame(c.Request.Context(), gameID, req.Count)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}
