package handler

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/pkg/jwt"
	redisPkg "github.com/bingo/backend/pkg/redis"
	"github.com/bingo/backend/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// AdminWSHandler streams live admin events (currently: bonus-campaign claims)
// over WebSocket, so the dashboard shows a stampede fill up in real time
// instead of on a timer.
type AdminWSHandler struct {
	redisClient *redis.Client
	jwt         *jwt.Service
	upgrader    websocket.Upgrader
}

func NewAdminWSHandler(redisClient *redis.Client, jwtService *jwt.Service, allowedOrigins []string) *AdminWSHandler {
	return &AdminWSHandler{
		redisClient: redisClient,
		jwt:         jwtService,
		upgrader:    newUpgrader(allowedOrigins),
	}
}

// BonusCampaign handles GET /ws/admin/bonus-campaign.
//
// Authenticated by a `token` QUERY parameter, not the Authorization header: a
// browser cannot set headers on a WebSocket handshake, so the usual admin
// middleware cannot guard this route. The token is validated here and the role
// checked, giving the same guarantee the middleware would — an admin JWT or no
// connection.
func (h *AdminWSHandler) BonusCampaign(c *gin.Context) {
	// Auth BEFORE upgrading, so a rejection is an ordinary HTTP 401/403 the
	// client can read, not a mid-handshake socket close.
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token query parameter required"})
		return
	}
	claims, err := h.jwt.ValidateToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
		return
	}
	if claims.Role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
		return
	}

	if h.redisClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "realtime updates unavailable"})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[admin-ws] upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pubsub := h.redisClient.Subscribe(ctx, redisPkg.BonusCampaignChannel)
	defer pubsub.Close()

	// Reader pump: a WebSocket needs its incoming frames drained for pong
	// handling and close detection, even though this endpoint is push-only.
	readDone := make(chan struct{})
	go func() {
		defer utils.RecoverPanic("admin-ws.read")
		defer close(readDone)
		conn.SetReadDeadline(time.Now().Add(domain.WebSocketReadDeadline))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(domain.WebSocketReadDeadline))
			return nil
		})
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	msgs := pubsub.Channel()
	ticker := time.NewTicker(domain.WebSocketPingInterval)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
				return
			}
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-readDone:
			return
		case <-ctx.Done():
			return
		}
	}
}
