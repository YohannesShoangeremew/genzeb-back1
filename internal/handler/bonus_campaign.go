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

// BonusCampaignHandler exposes "first N players" bonus giveaways: the player's
// claim button and the admin's controls.
type BonusCampaignHandler struct {
	useCase *usecase.BonusCampaignUseCase
}

func NewBonusCampaignHandler(useCase *usecase.BonusCampaignUseCase) *BonusCampaignHandler {
	return &BonusCampaignHandler{useCase: useCase}
}

// GetMyCampaign handles GET /me/bonus/campaign — the running giveaway and
// whether this player may claim it.
//
// Always 200, even with no campaign running: an empty state is the normal case
// on a day with no promotion, and an error status would have the client render
// a failure for it.
func (h *BonusCampaignHandler) GetMyCampaign(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	status, err := h.useCase.Status(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

// Claim handles POST /me/bonus/claim — take a slot in the running giveaway.
//
// A refusal answers 409 Conflict, not 500: losing the race for the last slot,
// having claimed already, or not being eligible are all normal outcomes the
// client must show as a message. Only a genuine fault is a 500. The `reason`
// field is a stable machine-readable code so the app can pick its own Amharic
// wording rather than displaying an English sentence from the server.
func (h *BonusCampaignHandler) Claim(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	claim, err := h.useCase.Claim(c.Request.Context(), userID)
	if err != nil {
		if usecase.IsClaimRefusal(err) {
			c.JSON(http.StatusConflict, gin.H{
				"error":  err.Error(),
				"reason": domain.ReasonCode(err),
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"claim": claim})
}

// CreateCampaign handles POST /admin/bonus/campaigns — start today's giveaway,
// optionally announcing it to every player.
func (h *BonusCampaignHandler) CreateCampaign(c *gin.Context) {
	var req domain.CreateBonusCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var createdBy *uuid.UUID
	if id, ok := middleware.GetUserID(c); ok {
		createdBy = &id
	}

	campaign, err := h.useCase.Create(c.Request.Context(), req, createdBy)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"campaign": campaign})
}

// ListCampaigns handles GET /admin/bonus/campaigns.
func (h *BonusCampaignHandler) ListCampaigns(c *gin.Context) {
	limit := 50
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	campaigns, err := h.useCase.List(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"campaigns": campaigns})
}

// ListClaims handles GET /admin/bonus/campaigns/:id/claims — who claimed, in
// the order they got in.
func (h *BonusCampaignHandler) ListClaims(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign id"})
		return
	}
	claims, err := h.useCase.ListClaims(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"claims": claims})
}

// EndCampaign handles POST /admin/bonus/campaigns/:id/end — stop it early.
// Slots already claimed keep their money; this only stops further claims.
func (h *BonusCampaignHandler) EndCampaign(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid campaign id"})
		return
	}
	campaign, err := h.useCase.End(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"campaign": campaign})
}
