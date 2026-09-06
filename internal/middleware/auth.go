package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/bingo/backend/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	UserIDKey = "user_id"
	RoleKey   = "role"
)

// GetUserID returns the authenticated user's ID from the Gin context.
// It is only populated after AuthMiddleware has run.
func GetUserID(c *gin.Context) (uuid.UUID, bool) {
	v, exists := c.Get(UserIDKey)
	if !exists {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

// AuthMiddleware validates JWT token and extracts user information
func AuthMiddleware(jwtService *jwt.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header required",
			})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid authorization header format. Expected: Bearer <token>",
			})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// Validate token
		claims, err := jwtService.ValidateToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid or expired token",
			})
			c.Abort()
			return
		}

		// Store user info in context
		c.Set(UserIDKey, claims.UserID)
		c.Set(RoleKey, claims.Role)

		c.Next()
	}
}

// InternalSecretMiddleware guards server-to-server ("bot-facing") endpoints that
// return another user's data by ID. It requires the X-Internal-Api-Secret header
// to match the configured secret. If no secret is configured it FAILS CLOSED —
// these endpoints expose other users' balances/PII and must never be reachable
// anonymously from the public internet.
func InternalSecretMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "internal endpoints are disabled (INTERNAL_API_SECRET not configured)",
			})
			c.Abort()
			return
		}
		got := c.GetHeader("X-Internal-Api-Secret")
		if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid or missing internal API secret",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// AdminMiddleware checks if the user has admin role
func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get(RoleKey)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "User role not found",
			})
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok || roleStr != "admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Admin access required",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
