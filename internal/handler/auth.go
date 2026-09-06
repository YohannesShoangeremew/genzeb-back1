package handler

import (
	"net/http"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/usecase"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authUseCase *usecase.AuthUseCase
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(authUseCase *usecase.AuthUseCase) *AuthHandler {
	return &AuthHandler{
		authUseCase: authUseCase,
	}
}

// Login handles the POST /auth/login endpoint
func (h *AuthHandler) Login(c *gin.Context) {
	var req domain.LoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	response, err := h.authUseCase.Login(c.Request.Context(), req)
	if err != nil {
		statusCode := http.StatusUnauthorized
		if err.Error() == "admin access required" {
			statusCode = http.StatusForbidden
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// TelegramLogin handles POST /auth/telegram
// Accepts a Telegram Mini App initData string, verifies it, and returns a JWT.
func (h *AuthHandler) TelegramLogin(c *gin.Context) {
	var req domain.TelegramAuthRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	response, err := h.authUseCase.TelegramLogin(c.Request.Context(), req.InitData)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, response)
}

// CreateAdmin handles the POST /auth/create-admin endpoint
func (h *AuthHandler) CreateAdmin(c *gin.Context) {
	var req domain.CreateAdminRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	user, err := h.authUseCase.CreateAdmin(c.Request.Context(), req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		switch err.Error() {
		case "user not found":
			statusCode = http.StatusNotFound
		case "invalid secret code":
			statusCode = http.StatusForbidden
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Admin user created successfully",
		"user":    user,
	})
}
