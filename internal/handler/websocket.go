package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/usecase"
	redisPkg "github.com/bingo/backend/pkg/redis"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// isValidGameType checks if the game type is valid
func isValidGameType(gameType domain.GameType) bool {
	return gameType.IsValid()
}

// newUpgrader builds an upgrader that accepts browser connections only from
// the configured frontend origins — the same allowlist the CORS middleware
// uses, so the WebSocket cannot be the one door left open when that list is
// tightened. It previously returned true unconditionally, which let any site a
// player happened to visit open a socket to a game.
//
// An absent Origin is allowed: non-browser clients (native apps, curl, uptime
// probes) send none, while browsers always do. Cross-site WebSocket hijacking
// is a browser-only attack, so an empty Origin is not the case this guard
// exists to stop, and rejecting it would break every non-browser consumer.
func newUpgrader(allowed []string) websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			for _, o := range allowed {
				if strings.EqualFold(origin, o) {
					return true
				}
			}
			log.Printf("[WebSocket] rejected connection from disallowed origin %q", origin)
			return false
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
}

type WebSocketHandler struct {
	redisClient *redis.Client
	gameService *redisPkg.GameStateService
	gameUseCase *usecase.GameUseCase                // Add game use case for database checks
	clients     map[string]map[*websocket.Conn]bool // gameID -> connections
	mu          sync.RWMutex
	upgrader    websocket.Upgrader
}

// NewWebSocketHandler creates a new WebSocket handler. allowedOrigins is the
// same list the CORS middleware is configured with; see newUpgrader.
func NewWebSocketHandler(redisClient *redis.Client, gameService *redisPkg.GameStateService, gameUseCase *usecase.GameUseCase, allowedOrigins []string) *WebSocketHandler {
	return &WebSocketHandler{
		redisClient: redisClient,
		gameService: gameService,
		gameUseCase: gameUseCase,
		clients:     make(map[string]map[*websocket.Conn]bool),
		upgrader:    newUpgrader(allowedOrigins),
	}
}

// HandleWebSocket handles WebSocket connections for game updates
// Public viewing - anyone can connect by game type (e.g., ?type=G5) or game ID
func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	// Log request details for debugging. Deliberately NOT the whole header map:
	// that carries Authorization bearer tokens, cookies and any internal
	// forwarding headers straight into the logs. Origin is the only header
	// worth having here — it is what the upgrade decision turns on.
	log.Printf("[WebSocket] Request received - Method: %s, Path: %s, Query: %s, Origin: %s",
		c.Request.Method, c.Request.URL.Path, c.Request.URL.RawQuery, c.Request.Header.Get("Origin"))
	var gameID uuid.UUID
	var err error
	var errorReason string

	// Check if connecting by game type (public viewing) or game ID
	gameTypeStr := c.Query("type")
	gameIDParam := c.Param("gameId")

	log.Printf("[WebSocket] ===== Connection attempt ===== Path: %s, Query: %s, gameId param: '%s'",
		c.Request.URL.Path, c.Request.URL.RawQuery, gameIDParam)

	if gameTypeStr != "" {
		// Connect by game type - find or create an available game
		gameType := domain.GameType(gameTypeStr)
		if !isValidGameType(gameType) {
			errorReason = fmt.Sprintf("Invalid game type '%s'. Must be one of: REGULAR, VIP", gameTypeStr)
			log.Printf("[WebSocket] ERROR: %s", errorReason)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":  errorReason,
				"reason": errorReason,
			})
			return
		}

		// Find or create a game of this type
		if h.gameUseCase == nil {
			errorReason = "Game use case service is not initialized (nil pointer)"
			log.Printf("[WebSocket] ERROR: %s", errorReason)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  errorReason,
				"reason": errorReason,
			})
			return
		}

		log.Printf("[WebSocket] Creating or getting game of type %s", gameType)
		game, err := h.gameUseCase.CreateOrGetGame(c.Request.Context(), gameType)
		if err != nil {
			errorReason = fmt.Sprintf("Failed to create or get game of type %s: %v", gameType, err)
			log.Printf("[WebSocket] ERROR: %s", errorReason)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  errorReason,
				"reason": errorReason,
			})
			return
		}
		if game == nil {
			errorReason = fmt.Sprintf("Game service returned nil game for type %s", gameType)
			log.Printf("[WebSocket] ERROR: %s", errorReason)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  errorReason,
				"reason": errorReason,
			})
			return
		}
		gameID = game.ID
		log.Printf("[WebSocket] ✓ Found/created game %s (type: %s, state: %s, players: %d)",
			gameID, gameType, game.State, game.PlayerCount)
	} else if gameIDParam != "" {
		// Connect by game ID (from path parameter)
		gameID, err = uuid.Parse(gameIDParam)
		if err != nil {
			errorReason = fmt.Sprintf("Invalid game ID format '%s': %v", gameIDParam, err)
			log.Printf("[WebSocket] ERROR: %s", errorReason)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":  errorReason,
				"reason": errorReason,
			})
			return
		}
		log.Printf("[WebSocket] ✓ Parsed game ID: %s", gameID)
	} else {
		errorReason = "No game type or game ID provided. Use ?type=REGULAR, ?type=VIP, or /ws/game/:gameId"
		log.Printf("[WebSocket] ERROR: %s", errorReason)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  errorReason,
			"reason": errorReason,
		})
		return
	}

	// Check Redis availability first (before upgrading connection)
	if h.redisClient == nil {
		errorReason = "Redis client is nil. WebSocket requires Redis for real-time updates."
		log.Printf("[WebSocket] ERROR: %s", errorReason)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":  errorReason,
			"reason": errorReason,
		})
		return
	}

	// Verify Redis connection
	ctx := c.Request.Context()
	log.Printf("[WebSocket] Testing Redis connection...")
	if err := h.redisClient.Ping(ctx).Err(); err != nil {
		errorReason = fmt.Sprintf("Redis ping failed: %v. Check Redis connection and credentials.", err)
		log.Printf("[WebSocket] ERROR: %s", errorReason)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":  errorReason,
			"reason": errorReason,
		})
		return
	}
	log.Printf("[WebSocket] ✓ Redis connection verified")

	gameIDStr := gameID.String()
	log.Printf("[WebSocket] All pre-checks passed. Attempting WebSocket upgrade for game %s", gameIDStr)

	// Upgrade connection
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		errorReason = fmt.Sprintf("WebSocket upgrade failed: %v. Check if connection is already upgraded or headers are correct.", err)
		log.Printf("[WebSocket] ERROR: %s", errorReason)
		// Try to send error response if connection not yet upgraded
		if !c.Writer.Written() {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":  errorReason,
				"reason": errorReason,
			})
		}
		return
	}
	log.Printf("[WebSocket] ✓ Connection upgraded successfully for game %s", gameIDStr)
	defer conn.Close()

	// Add client
	h.mu.Lock()
	if h.clients[gameIDStr] == nil {
		h.clients[gameIDStr] = make(map[*websocket.Conn]bool)
	}
	h.clients[gameIDStr][conn] = true
	h.mu.Unlock()

	// Remove client on disconnect
	defer func() {
		h.mu.Lock()
		delete(h.clients[gameIDStr], conn)
		if len(h.clients[gameIDStr]) == 0 {
			delete(h.clients, gameIDStr)
		}
		h.mu.Unlock()
	}()

	// Subscribe to Redis pub/sub (Redis is already verified above)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	channel := redisPkg.GameChannel(gameIDStr)
	log.Printf("[WebSocket] Subscribing to Redis channel: %s", channel)
	pubsub := h.redisClient.Subscribe(ctx, channel)
	defer pubsub.Close()

	// Verify subscription
	_, err = pubsub.ReceiveTimeout(ctx, 2*time.Second)
	if err != nil {
		log.Printf("[WebSocket] Warning: Subscription verification failed: %v", err)
		// Continue anyway, subscription might still work
	} else {
		log.Printf("[WebSocket] Successfully subscribed to Redis channel")
	}

	// Send initial game state
	log.Printf("[WebSocket] Sending initial game state for game %s", gameIDStr)
	if err := h.sendInitialState(conn, gameID); err != nil {
		log.Printf("[WebSocket] ERROR: Failed to send initial state for game %s: %v", gameIDStr, err)
		return
	}
	log.Printf("[WebSocket] ✓ Initial state sent. Starting message loop for game %s", gameIDStr)

	// Channel for messages from Redis (increased buffer to prevent blocking)
	redisMessages := make(chan *redis.Message, 100)

	// Channel to signal Redis receiver error
	redisError := make(chan error, 1)

	// Goroutine to receive Redis messages
	go func() {
		defer close(redisError)
		if pubsub == nil {
			log.Printf("[WebSocket] ERROR: PubSub is nil, cannot receive messages for game %s", gameIDStr)
			redisError <- fmt.Errorf("pubsub is nil")
			return
		}
		log.Printf("[WebSocket] Starting Redis message receiver for game %s", gameIDStr)
		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				if err == context.Canceled {
					log.Printf("[WebSocket] Redis subscription context canceled for game %s", gameIDStr)
				} else {
					log.Printf("[WebSocket] Redis receive error for game %s: %v", gameIDStr, err)
					// Signal error so WebSocket can be closed
					select {
					case redisError <- err:
					default:
					}
				}
				return
			}
			log.Printf("[WebSocket] Received Redis message for game %s: %s", gameIDStr, msg.Payload)
			select {
			case redisMessages <- msg:
				// Message sent successfully
			case <-ctx.Done():
				log.Printf("[WebSocket] Context done, stopping Redis receiver for game %s", gameIDStr)
				return
			default:
				// Channel is full - log warning but don't block
				log.Printf("[WebSocket] WARNING: Redis message channel full for game %s, dropping message", gameIDStr)
			}
		}
	}()

	// Set read deadline and pong handler
	conn.SetReadDeadline(time.Now().Add(domain.WebSocketReadDeadline))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(domain.WebSocketReadDeadline))
		return nil
	})

	// Channel to signal connection close from read goroutine
	readDone := make(chan struct{})

	// Separate goroutine to read client messages (handles pong responses)
	go func() {
		defer func() {
			// Recover from any panic (e.g., "repeated read on failed websocket connection")
			if r := recover(); r != nil {
				log.Printf("[WebSocket] Recovered from panic in read goroutine for game %s: %v", gameIDStr, r)
			}
			close(readDone)
		}()
		for {
			// Set a reasonable read deadline
			conn.SetReadDeadline(time.Now().Add(domain.WebSocketReadDeadline))
			_, _, err := conn.ReadMessage()
			if err != nil {
				// Check if it's a timeout/deadline error (expected - means no message)
				if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
					// Expected timeout, continue reading
					continue
				}
				// Check if it's a WebSocket close error
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("[WebSocket] Unexpected close error in read goroutine for game %s: %v", gameIDStr, err)
					return
				}
				// Check if connection was closed normally
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Printf("[WebSocket] Connection closed normally in read goroutine for game %s", gameIDStr)
					return
				}
				// Check if it's a "use of closed network connection" error (connection already closed)
				errStr := err.Error()
				if errStr == "use of closed network connection" ||
					errStr == "repeated read on failed websocket connection" ||
					errStr == "websocket: close 1006 (abnormal closure): unexpected EOF" {
					log.Printf("[WebSocket] Connection already closed for game %s: %v", gameIDStr, err)
					return
				}
				// Other errors - log and return (connection likely failed)
				// Don't continue reading after any error to avoid panic
				log.Printf("[WebSocket] Read error in read goroutine for game %s: %v", gameIDStr, err)
				return
			}
		}
	}()

	// Main write loop
	ticker := time.NewTicker(domain.WebSocketPingInterval)
	defer ticker.Stop()

	for {
		select {
		case msg := <-redisMessages:
			// Forward Redis message to WebSocket
			if err := conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
				log.Printf("[WebSocket] Error writing Redis message to client for game %s: %v", gameIDStr, err)
				return
			}
			log.Printf("[WebSocket] ✓ Forwarded Redis message to client for game %s", gameIDStr)

		case err := <-redisError:
			// Redis subscription error - close connection so client can reconnect
			log.Printf("[WebSocket] Redis subscription error for game %s: %v. Closing connection.", gameIDStr, err)
			return

		case <-ticker.C:
			// Send ping
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("[WebSocket] Error sending ping for game %s: %v", gameIDStr, err)
				return
			}

		case <-readDone:
			// Read goroutine detected connection close
			log.Printf("[WebSocket] Read goroutine signaled connection close for game %s", gameIDStr)
			return

		case <-ctx.Done():
			// Context canceled (e.g., Redis subscription closed)
			log.Printf("[WebSocket] Context done for game %s", gameIDStr)
			return
		}
	}
}

// sendInitialState sends the initial game state to the client
// Returns an error if the message cannot be sent
func (h *WebSocketHandler) sendInitialState(conn *websocket.Conn, gameID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(context.Background(), domain.WebSocketInitialStateTimeout)
	defer cancel()

	// Try Redis first, fallback to database
	var game *domain.Game
	var err error

	if h.gameService != nil && h.redisClient != nil {
		game, err = h.gameService.GetGameState(ctx, gameID)
	}

	// Fallback to database if Redis fails or not available
	if game == nil && h.gameUseCase != nil {
		game, _, _, err = h.gameUseCase.GetGameState(ctx, gameID)
	}

	if err != nil || game == nil {
		log.Printf("Warning: Could not get game state for %s: %v", gameID, err)
		// Send minimal state
		initialState := map[string]interface{}{
			"event": domain.WebSocketEventInitialState,
			"data": map[string]interface{}{
				"game":         nil,
				"drawnNumbers": []interface{}{},
				"takenCards":   []int{},
				"playerCount":  0,
				"secondsLeft":  0,
			},
		}
		data, err := json.Marshal(initialState)
		if err != nil {
			log.Printf("[WebSocket] ERROR: Failed to marshal minimal initial state for game %s: %v", gameID, err)
			return err
		}
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[WebSocket] ERROR: Failed to write minimal initial state message for game %s: %v", gameID, err)
			return err
		}
		log.Printf("[WebSocket] ✓ Sent minimal initial state for game %s", gameID)
		return nil
	}

	var drawnNumbers []domain.DrawnNumber
	var takenCards []int
	var playerCount int64

	if h.gameService != nil && h.redisClient != nil {
		drawnNumbers, _ = h.gameService.GetDrawnNumbers(ctx, gameID)
		takenCards, _ = h.gameService.GetTakenCards(ctx, gameID)
		playerCount, _ = h.gameService.GetPlayerCount(ctx, gameID)
	} else if h.gameUseCase != nil {
		// Fallback to database
		_, drawnNumbers, takenCards, _ = h.gameUseCase.GetGameState(ctx, gameID)
		// Player count from game (game is already checked to be non-nil above)
		playerCount = int64(game.PlayerCount)
	}

	// Get countdown if in countdown state
	var secondsLeft int
	if game.State == domain.GameStateCountdown {
		if h.gameService != nil && h.redisClient != nil {
			countdownEnds, err := h.gameService.GetCountdown(ctx, gameID)
			if err == nil {
				secondsLeft = int(time.Until(countdownEnds).Seconds())
				if secondsLeft < 0 {
					secondsLeft = 0
				}
			}
		} else if game.CountdownEnds != nil {
			// Fallback to database
			secondsLeft = int(time.Until(*game.CountdownEnds).Seconds())
			if secondsLeft < 0 {
				secondsLeft = 0
			}
		}
	}

	stateData := map[string]interface{}{
		"game":         game,
		"drawnNumbers": drawnNumbers,
		"takenCards":   takenCards,
		"playerCount":  playerCount,
		"secondsLeft":  secondsLeft,
	}

	// For a finished game, include the winning card(s) so a client connecting
	// after the transient winner event (a reconnect, or a spectator opening the
	// finished game) can still render the post-game winner screen. There may be
	// several co-winners who split the pot.
	if game.State == domain.GameStateFinished && h.gameUseCase != nil {
		if winners, err := h.gameUseCase.GetGameWinners(ctx, gameID); err == nil {
			stateData["winners"] = winners
		}
	}

	initialState := map[string]interface{}{
		"event": domain.WebSocketEventInitialState,
		"data":  stateData,
	}

	data, err := json.Marshal(initialState)
	if err != nil {
		log.Printf("[WebSocket] ERROR: Failed to marshal initial state for game %s: %v", gameID, err)
		return err
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("[WebSocket] ERROR: Failed to write initial state message for game %s: %v", gameID, err)
		return err
	}
	log.Printf("[WebSocket] ✓ Initial state message sent successfully for game %s", gameID)
	return nil
}
