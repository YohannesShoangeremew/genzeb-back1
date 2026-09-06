package handler

import (
	"net/http"
	"strings"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/middleware"
	"github.com/bingo/backend/internal/usecase"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type UserHandler struct {
	userUseCase *usecase.UserUseCase
}

// NewUserHandler creates a new user handler
func NewUserHandler(userUseCase *usecase.UserUseCase) *UserHandler {
	return &UserHandler{
		userUseCase: userUseCase,
	}
}

// Register handles the POST /user/register endpoint
func (h *UserHandler) Register(c *gin.Context) {
	var req domain.CreateUserRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	user, wallet, err := h.userUseCase.CreateUser(c.Request.Context(), req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "user with this telegram ID already exists" ||
			err.Error() == "user with this phone number already exists" {
			statusCode = http.StatusConflict
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "User and wallet created successfully",
		"user":    user,
		"wallet":  wallet,
	})
}

// FindByTelegramID handles the GET /user/telegram/:telegram_id endpoint
func (h *UserHandler) FindByTelegramID(c *gin.Context) {
	var uri struct {
		TelegramID int64 `uri:"telegram_id" binding:"required"`
	}

	if err := c.ShouldBindUri(&uri); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid telegram ID",
		})
		return
	}

	user, err := h.userUseCase.FindUserByTelegramID(c.Request.Context(), uri.TelegramID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}

// FindByPhone handles the GET /user/phone endpoint
func (h *UserHandler) FindByPhone(c *gin.Context) {
	var req struct {
		Phone string `form:"phone" binding:"required"`
	}

	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "phone parameter is required",
		})
		return
	}

	user, err := h.userUseCase.FindUserByPhone(c.Request.Context(), req.Phone)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}

// FindByReferralCode handles the GET /user/referral/:referral_code endpoint
func (h *UserHandler) FindByReferralCode(c *gin.Context) {
	referralCode := c.Param("referral_code")
	if referralCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "referral_code is required",
		})
		return
	}

	user, err := h.userUseCase.FindUserByReferralCode(c.Request.Context(), referralCode)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}

// GetMe handles GET /me — the authenticated user's profile.
func (h *UserHandler) GetMe(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	user, err := h.userUseCase.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}

// UpdateMyName handles PUT /me/name — update the authenticated user's name.
func (h *UserHandler) UpdateMyName(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	var req domain.UpdateUserNameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	user, err := h.userUseCase.UpdateUserName(c.Request.Context(), userID, req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "user not found" {
			statusCode = http.StatusNotFound
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User name updated successfully",
		"user":    user,
	})
}

// GetAllUsers handles the GET /admin/users endpoint
func (h *UserHandler) GetAllUsers(c *gin.Context) {
	limit, offset := getPaginationParams(c)
	search := strings.TrimSpace(c.Query("search"))
	role := strings.TrimSpace(c.Query("role"))
	if role != "" && role != "user" && role != "admin" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid role filter"})
		return
	}

	usersWithWallets, totalCount, err := h.userUseCase.GetAllUsersWithWallets(c.Request.Context(), search, role, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch users",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users":  usersWithWallets,
		"count":  totalCount,
		"limit":  limit,
		"offset": offset,
	})
}

// UpdateUserName handles the PUT /user/:user_id/name endpoint
func (h *UserHandler) UpdateUserName(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	var req domain.UpdateUserNameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}

	user, err := h.userUseCase.UpdateUserName(c.Request.Context(), userID, req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "user not found" {
			statusCode = http.StatusNotFound
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "User name updated successfully",
		"user":    user,
	})
}

// GetUserDetail handles GET /admin/users/:user_id — user + wallet.
func (h *UserHandler) GetUserDetail(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	detail, err := h.userUseCase.GetUserDetail(c.Request.Context(), userID)
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "user not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": detail})
}

// GetReferredUsers handles GET /admin/users/:user_id/referrals — everyone this
// player invited, so an admin can open each invitee's profile.
func (h *UserHandler) GetReferredUsers(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	users, err := h.userUseCase.GetReferredUsers(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch referred users"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users, "count": len(users)})
}

// SetUserRole handles POST /admin/users/:user_id/role — promote/demote.
func (h *UserHandler) SetUserRole(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req domain.SetRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data", "details": err.Error()})
		return
	}

	if err := h.userUseCase.SetUserRole(c.Request.Context(), userID, req.Role); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "user not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User role updated", "role": req.Role})
}

// MakeAdmin handles POST /admin/users/:user_id/make-admin — promote to admin
// and set a dashboard password.
func (h *UserHandler) MakeAdmin(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	var req domain.MakeAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data", "details": err.Error()})
		return
	}

	if err := h.userUseCase.MakeAdmin(c.Request.Context(), userID, req.Password); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "user not found" {
			status = http.StatusNotFound
		} else if err.Error() == "password must be at least 8 characters" {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User is now an admin with a password set"})
}

// BanUser handles POST /admin/users/:user_id/ban.
func (h *UserHandler) BanUser(c *gin.Context) { h.setBanned(c, true) }

// UnbanUser handles POST /admin/users/:user_id/unban.
func (h *UserHandler) UnbanUser(c *gin.Context) { h.setBanned(c, false) }

func (h *UserHandler) setBanned(c *gin.Context, banned bool) {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	if err := h.userUseCase.SetUserBanned(c.Request.Context(), userID, banned); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "user not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	msg := "User banned"
	if !banned {
		msg = "User unbanned"
	}
	c.JSON(http.StatusOK, gin.H{"message": msg, "banned": banned})
}

// DeleteUser handles DELETE /admin/users/:user_id — permanently remove a user
// and their attached wallet/transactions/game rows (via FK cascade).
func (h *UserHandler) DeleteUser(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Guard against an admin deleting their own account.
	if actingID, ok := middleware.GetUserID(c); ok && actingID == userID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "You cannot delete your own account"})
		return
	}

	if err := h.userUseCase.DeleteUser(c.Request.Context(), userID); err != nil {
		status := http.StatusInternalServerError
		switch err.Error() {
		case "user not found":
			status = http.StatusNotFound
		case "cannot delete an admin account; demote it first":
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted"})
}
