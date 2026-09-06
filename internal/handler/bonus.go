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

// BonusHandler exposes the play-only bonus wallet: admin controls plus the
// player's own balance.
type BonusHandler struct {
	bonusUseCase *usecase.BonusUseCase
}

func NewBonusHandler(bonusUseCase *usecase.BonusUseCase) *BonusHandler {
	return &BonusHandler{bonusUseCase: bonusUseCase}
}

// GetMyBonus handles GET /me/bonus — the player's spendable bonus, when the
// soonest grant expires, and the operator's current announcement.
//
// The announcement rides along here rather than on a separate endpoint so the
// client shows the promotion text and the balance it refers to together, and
// can never display one without the other.
func (h *BonusHandler) GetMyBonus(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	balance, err := h.bonusUseCase.Balance(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// A failed config read must not hide the player's balance — show the money
	// and drop the announcement rather than erroring the whole response.
	announcement := ""
	if cfg, cerr := h.bonusUseCase.GetConfig(c.Request.Context()); cerr == nil {
		announcement = cfg.Announcement
	}

	c.JSON(http.StatusOK, gin.H{
		"bonus":        balance,
		"announcement": announcement,
	})
}

// GetConfig handles GET /admin/bonus/config.
func (h *BonusHandler) GetConfig(c *gin.Context) {
	cfg, err := h.bonusUseCase.GetConfig(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// UpdateConfig handles PUT /admin/bonus/config — enable/disable granting, set
// how long new grants last, and edit the player-facing announcement.
func (h *BonusHandler) UpdateConfig(c *gin.Context) {
	var req domain.UpdateBonusConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg, err := h.bonusUseCase.UpdateConfig(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// GrantBonus handles POST /admin/bonus/grant — award bonus to one player.
func (h *BonusHandler) GrantBonus(c *gin.Context) {
	var req domain.GrantBonusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	grant, err := h.bonusUseCase.Grant(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"grant": grant})
}

// GrantBonusBulk handles POST /admin/bonus/grant-bulk — award the same bonus to
// several players, which is how a "random day" campaign is actually run.
//
// Reports per-user failures rather than aborting: with a list of ids, one bad
// entry should not silently cost every other player their bonus, and the admin
// needs to know exactly who missed out.
func (h *BonusHandler) GrantBonusBulk(c *gin.Context) {
	var req struct {
		UserIDs []uuid.UUID `json:"user_ids" binding:"required,min=1"`
		Amount  float64     `json:"amount" binding:"required,gt=0"`
		Reason  string      `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	granted, failures := h.bonusUseCase.GrantMany(c.Request.Context(), req.UserIDs, req.Amount, req.Reason)
	failed := make(map[string]string, len(failures))
	for id, msg := range failures {
		failed[id.String()] = msg
	}
	c.JSON(http.StatusOK, gin.H{
		"granted":   granted,
		"failed":    failed,
		"attempted": len(req.UserIDs),
	})
}

// ListUserGrants handles GET /admin/users/:user_id/bonus — one player's bonus
// history.
func (h *BonusHandler) ListUserGrants(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}
	limit := 50
	if v := c.Query("limit"); v != "" {
		if parsed, perr := strconv.Atoi(v); perr == nil && parsed > 0 {
			limit = parsed
		}
	}

	grants, err := h.bonusUseCase.ListGrants(c.Request.Context(), userID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	balance, err := h.bonusUseCase.Balance(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"grants": grants, "balance": balance})
}

// GetOutstanding handles GET /admin/bonus/outstanding — the house's live bonus
// liability, i.e. bonus players could still stake. Expired grants are excluded
// because they cost nothing.
func (h *BonusHandler) GetOutstanding(c *gin.Context) {
	total, err := h.bonusUseCase.TotalOutstanding(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"outstanding_bonus": total})
}
