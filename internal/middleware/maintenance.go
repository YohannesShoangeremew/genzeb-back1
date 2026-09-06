package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Maintenance blocks player state-changing actions while maintenance mode is on.
//
// It is applied to the authenticated player (Mini App) route group only — the
// admin group and the bot-facing internal-secret groups never get it, so the
// dashboard and the bot stay fully usable during maintenance.
//
// Only mutations are blocked: GET reads pass through so the app can still load
// its shell and show the maintenance screen (which the frontend drives off the
// public /status endpoint). Everything else — join game, deposit, withdraw,
// transfer, claim bingo, edit profile — is rejected with 503 so a player cannot
// act even by calling the API directly.
//
// isOn is injected (rather than importing the usecase) to keep middleware free of
// business-layer dependencies. It should be cheap; a single-row settings read.
func Maintenance(isOn func(ctx context.Context) (bool, string)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		on, msg := isOn(c.Request.Context())
		if on {
			if msg == "" {
				msg = "The app is under maintenance. Please try again shortly."
			}
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error":       "maintenance",
				"maintenance": true,
				"message":     msg,
			})
			return
		}
		c.Next()
	}
}
