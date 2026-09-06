package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/middleware"
	"github.com/bingo/backend/internal/usecase"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type GameHandler struct {
	gameUseCase *usecase.GameUseCase
}

// NewGameHandler creates a new game handler
func NewGameHandler(gameUseCase *usecase.GameUseCase) *GameHandler {
	return &GameHandler{
		gameUseCase: gameUseCase,
	}
}

// GetGames handles GET /games?type={betAmount}
// Returns available games. If no games exist for a game type, creates one automatically.
func (h *GameHandler) GetGames(c *gin.Context) {
	var req domain.GetGamesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid query parameters",
			"details": err.Error(),
		})
		return
	}

	// A real player is looking at the lobby — keep this tier "recently browsed"
	// so the filler bots run games here (and idle once nobody's around).
	h.gameUseCase.RecordLobbyActivity(c.Request.Context(), req.GameType)

	// Get available games
	games, err := h.gameUseCase.GetAvailableGames(c.Request.Context(), req.GameType)
	if err != nil {
		// Log the error for debugging
		fmt.Printf("[GetGames] Error getting available games (type: %v): %v\n", req.GameType, err)
		// Ensure we send a proper JSON response even if there's an error
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get available games",
			"details": err.Error(),
		})
		return
	}

	// If filtering by game type and no JOINABLE (WAITING/COUNTDOWN) game exists:
	if req.GameType != nil && len(games) == 0 {
		// One game per table at a time. If a round of this type is already being
		// drawn, return that DRAWING game so the client spectates it — do NOT
		// start a second game. Only when nothing is running do we create the next.
		if inProgress, _ := h.gameUseCase.GetInProgressGame(c.Request.Context(), *req.GameType); inProgress != nil {
			games = []*domain.Game{inProgress}
		} else {
			game, err := h.gameUseCase.CreateOrGetGame(c.Request.Context(), *req.GameType)
			if err != nil {
				fmt.Printf("[GetGames] Error creating/getting game (type: %v): %v\n", *req.GameType, err)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "Failed to create or get game",
					"details": err.Error(),
				})
				return
			}
			games = []*domain.Game{game}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"games": games,
	})
}

// GetRecentWinners handles GET /games/recent-winners?limit=
// Public feed of recent game winners for the lobby (transparency / trust).
func (h *GameHandler) GetRecentWinners(c *gin.Context) {
	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed := parseInt(limitStr); parsed > 0 {
			limit = parsed
		}
	}

	winners, err := h.gameUseCase.GetRecentWinners(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"winners": winners})
}

// JoinGame handles POST /games/:gameId/join
func (h *GameHandler) JoinGame(c *gin.Context) {
	gameIDStr := c.Param("gameId")
	gameID, err := uuid.Parse(gameIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid game ID",
		})
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	var req domain.JoinGameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}
	req.UserID = userID

	player, err := h.gameUseCase.JoinGame(c.Request.Context(), gameID, req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "game is not accepting new players" ||
			err.Error() == "user is already in this game" ||
			err.Error() == "card is already taken" ||
			err.Error() == "insufficient balance" ||
			strings.Contains(err.Error(), "cards per game") {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"player": player,
	})
}

// LeaveGame handles POST /games/:gameId/leave
func (h *GameHandler) LeaveGame(c *gin.Context) {
	gameIDStr := c.Param("gameId")
	gameID, err := uuid.Parse(gameIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid game ID",
		})
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	var req domain.LeaveGameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Leave has no required body fields; ignore empty-body bind errors.
		req = domain.LeaveGameRequest{}
	}
	req.UserID = userID

	if err := h.gameUseCase.LeaveGame(c.Request.Context(), gameID, req); err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "user is not in this game" ||
			err.Error() == "you do not hold that card" ||
			err.Error() == "game is no longer active" {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Successfully left the game",
	})
}

// ClaimBingo handles POST /games/:gameId/bingo
func (h *GameHandler) ClaimBingo(c *gin.Context) {
	gameIDStr := c.Param("gameId")
	gameID, err := uuid.Parse(gameIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid game ID",
		})
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	var req domain.ClaimBingoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request data",
			"details": err.Error(),
		})
		return
	}
	req.UserID = userID

	isWinner, err := h.gameUseCase.ClaimBingo(c.Request.Context(), gameID, req)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "game is not in drawing phase" ||
			err.Error() == "you do not hold that card in this game" ||
			err.Error() == "this card is already eliminated" {
			statusCode = http.StatusBadRequest
		} else if err.Error() == "game already has a winner" {
			// Lost the race to another simultaneous valid claim.
			statusCode = http.StatusConflict
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	if isWinner {
		c.JSON(http.StatusOK, gin.H{
			"winner":  true,
			"message": "Congratulations! You won!",
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"winner":  false,
			"message": "Invalid bingo claim. This card has been eliminated.",
		})
	}
}

// GetGameState handles GET /games/:gameId/state
func (h *GameHandler) GetGameState(c *gin.Context) {
	gameIDStr := c.Param("gameId")
	gameID, err := uuid.Parse(gameIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid game ID",
		})
		return
	}

	game, drawnNumbers, takenCards, err := h.gameUseCase.GetGameState(c.Request.Context(), gameID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"game":         game,
		"drawnNumbers": drawnNumbers,
		"takenCards":   takenCards,
	})
}

// GetPlayerInGame handles GET /games/:gameId/players/:userId
// Returns the player data if the user is in the game, or 404 if not found
func (h *GameHandler) GetPlayerInGame(c *gin.Context) {
	gameIDStr := c.Param("gameId")
	gameID, err := uuid.Parse(gameIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid game ID",
		})
		return
	}

	userIDStr := c.Param("userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	player, err := h.gameUseCase.GetPlayerInGame(c.Request.Context(), gameID, userID)
	if err != nil {
		if err.Error() == "player not found" {
			c.JSON(http.StatusNotFound, gin.H{
				"player": nil,
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"player": player,
	})
}

// GetCardData handles GET /cards/:cardId
// Returns the bingo card data (5x5 grid) for a given card ID
func (h *GameHandler) GetCardData(c *gin.Context) {
	cardIDStr := c.Param("cardId")

	// Parse card ID
	var cardID int
	if _, err := fmt.Sscanf(cardIDStr, "%d", &cardID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid card ID",
		})
		return
	}

	card, err := h.gameUseCase.GetCardData(c.Request.Context(), cardID)
	if err != nil {
		statusCode := http.StatusBadRequest
		if err.Error() != fmt.Sprintf("card ID must be between %d and %d", domain.MinCardID, domain.MaxCardID) {
			statusCode = http.StatusInternalServerError
		}

		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"card": card,
	})
}

// GetMyWinnings handles GET /me/winnings — the authenticated user's summed
// prize money, today (Ethiopian time) and all time. Backs the WIN stat on the
// card picker.
func (h *GameHandler) GetMyWinnings(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	today, total, err := h.gameUseCase.GetUserWinnings(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load winnings"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"today": today, "total": total})
}

// GetMyGameHistory handles GET /me/games — the authenticated user's game history.
func (h *GameHandler) GetMyGameHistory(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	limit := 10
	offset := 0
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit := parseInt(limitStr); parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset := parseInt(offsetStr); parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	history, err := h.gameUseCase.GetGameHistory(c.Request.Context(), userID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch game history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"games":  history,
		"count":  len(history),
		"limit":  limit,
		"offset": offset,
	})
}

// GetUserGamesAdmin handles GET /admin/users/:user_id/games — a player's game
// history (with game ids to link to), for the admin profile view.
func (h *GameHandler) GetUserGamesAdmin(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	limit, offset := 20, 0
	if v := parseInt(c.Query("limit")); v > 0 {
		limit = v
	}
	if v := parseInt(c.Query("offset")); v > 0 {
		offset = v
	}
	history, total, err := h.gameUseCase.GetGameHistoryPage(c.Request.Context(), userID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch game history"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"games": history, "count": len(history), "total": total,
		"limit": limit, "offset": offset,
	})
}

// GetUserGameStats handles GET /admin/users/:user_id/game-stats — a player's
// lifetime play record (games played/won, total won/staked), so an admin can
// tell whether a pending withdrawal belongs to a genuine winner.
func (h *GameHandler) GetUserGameStats(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}
	stats, err := h.gameUseCase.GetUserGameStats(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch game stats"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stats": stats})
}

// GetMyActiveGame handles GET /me/active-game — the live game the authenticated
// user is currently in (WAITING/COUNTDOWN/DRAWING and still holding cards), so a
// player who navigated away mid-game can jump back into the draw. Returns
// {"game": null} when the user isn't in any live game.
func (h *GameHandler) GetMyActiveGame(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	game, err := h.gameUseCase.GetActiveGame(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch active game"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"game": game})
}

// GetMyPlayerInGame handles GET /me/games/:gameId — whether the authenticated
// user is in the given game (returns the player record or 404).
func (h *GameHandler) GetMyPlayerInGame(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	gameID, err := uuid.Parse(c.Param("gameId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid game ID"})
		return
	}

	player, err := h.gameUseCase.GetPlayerInGame(c.Request.Context(), gameID, userID)
	if err != nil {
		if err.Error() == "player not found" {
			c.JSON(http.StatusNotFound, gin.H{"player": nil})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"player": player})
}

// GetMyCardsInGame handles GET /me/games/:gameId/cards — all of the
// authenticated user's active cards in the given game (0..MaxCardsPerPlayer).
func (h *GameHandler) GetMyCardsInGame(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	gameID, err := uuid.Parse(c.Param("gameId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid game ID"})
		return
	}

	cards, err := h.gameUseCase.GetMyCardsInGame(c.Request.Context(), gameID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"cards": cards})
}

// AdminListGames handles GET /admin/games
// Lists games for the admin dashboard with optional ?state= and ?type= filters
// and ?limit=/?offset= pagination.
func (h *GameHandler) AdminListGames(c *gin.Context) {
	var filter domain.AdminGameFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid query parameters",
			"details": err.Error(),
		})
		return
	}

	limit := domain.MaxAvailableGamesLimit
	offset := 0
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed := parseInt(limitStr); parsed > 0 {
			limit = parsed
		}
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsed := parseInt(offsetStr); parsed >= 0 {
			offset = parsed
		}
	}
	if limit > 200 {
		limit = 200
	}

	games, total, err := h.gameUseCase.ListGames(c.Request.Context(), filter, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"games":  games,
		"total":  total,
		"count":  len(games),
		"limit":  limit,
		"offset": offset,
	})
}

// AdminGetGame handles GET /admin/games/:gameId
// Returns a game plus its active players (with user info) for the admin view.
func (h *GameHandler) AdminGetGame(c *gin.Context) {
	gameID, err := uuid.Parse(c.Param("gameId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid game ID"})
		return
	}

	detail, err := h.gameUseCase.GetGameDetail(c.Request.Context(), gameID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, detail)
}

// AdminCancelGame handles POST /admin/games/:gameId/cancel
// Force-cancels a game and refunds every active player's stake.
func (h *GameHandler) AdminCancelGame(c *gin.Context) {
	gameID, err := uuid.Parse(c.Param("gameId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid game ID"})
		return
	}

	result, err := h.gameUseCase.CancelGame(c.Request.Context(), gameID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "already resolved") {
			statusCode = http.StatusBadRequest
		} else if strings.Contains(err.Error(), "game not found") {
			statusCode = http.StatusNotFound
		}
		c.JSON(statusCode, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Game cancelled and stakes refunded",
		"game":            result.Game,
		"refunded_count":  result.RefundedCount,
		"refunded_amount": result.RefundedAmount,
	})
}

// GetGameHistory handles GET /games/user/:user_id/history
// Returns the game history for a user
func (h *GameHandler) GetGameHistory(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	// Parse query parameters for pagination
	limit := 10 // default limit
	offset := 0 // default offset

	if limitStr := c.Query("limit"); limitStr != "" {
		if parsedLimit := parseInt(limitStr); parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsedOffset := parseInt(offsetStr); parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	history, err := h.gameUseCase.GetGameHistory(c.Request.Context(), userID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch game history",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"games":  history,
		"count":  len(history),
		"limit":  limit,
		"offset": offset,
	})
}
