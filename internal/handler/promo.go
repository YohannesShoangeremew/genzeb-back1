package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/gin-gonic/gin"
)

// PromoHandler exposes admin management of promo codes (creation, listing,
// on/off). Redemption itself happens through the Telegram bot.
type PromoHandler struct {
	promoRepo domain.PromoRepository
}

func NewPromoHandler(promoRepo domain.PromoRepository) *PromoHandler {
	return &PromoHandler{promoRepo: promoRepo}
}

// Create handles POST /admin/promo-codes.
func (h *PromoHandler) Create(c *gin.Context) {
	var req struct {
		Code           string  `json:"code" binding:"required"`
		BonusAmount    float64 `json:"bonus_amount" binding:"required,gt=0"`
		MaxRedemptions *int    `json:"max_redemptions"`
		// RFC3339, optional — e.g. "2026-08-01T00:00:00Z"
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if len(code) < 3 || len(code) > 32 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code must be 3-32 characters"})
		return
	}
	if req.MaxRedemptions != nil && *req.MaxRedemptions <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_redemptions must be positive"})
		return
	}

	promo := &domain.PromoCode{
		Code:           code,
		BonusAmount:    req.BonusAmount,
		MaxRedemptions: req.MaxRedemptions,
		ExpiresAt:      req.ExpiresAt,
	}
	if err := h.promoRepo.Create(c.Request.Context(), promo); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "already exists") {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"promo": promo})
}

// List handles GET /admin/promo-codes.
func (h *PromoHandler) List(c *gin.Context) {
	promos, err := h.promoRepo.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list promo codes"})
		return
	}
	if promos == nil {
		promos = []*domain.PromoCode{}
	}
	c.JSON(http.StatusOK, gin.H{"promos": promos})
}

// SetActive handles POST /admin/promo-codes/:code/activate and /deactivate.
func (h *PromoHandler) SetActive(active bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		code := c.Param("code")
		if err := h.promoRepo.SetActive(c.Request.Context(), code, active); err != nil {
			status := http.StatusInternalServerError
			if err == domain.ErrPromoNotFound {
				status = http.StatusNotFound
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": strings.ToUpper(code), "active": active})
	}
}
