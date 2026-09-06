package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bingo/backend/config"
	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/internal/handler"
	"github.com/bingo/backend/internal/middleware"
	"github.com/bingo/backend/internal/payment"
	"github.com/bingo/backend/internal/repository/postgres"
	"github.com/bingo/backend/internal/usecase"
	"github.com/bingo/backend/pkg/jwt"
	redisPkg "github.com/bingo/backend/pkg/redis"
	"github.com/bingo/backend/pkg/telegram"
	"github.com/bingo/backend/pkg/utils"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

// telegramBroadcastSender adapts the Telegram bot to the narrow interface a
// broadcast needs (plain text, one chat). Keeping the adapter here rather than
// widening the domain interface means `domain` never imports the bot package,
// and the send loop stays testable against a fake.
type telegramBroadcastSender struct{ bot *telegram.Bot }

func (s telegramBroadcastSender) SendMessage(chatID int64, text string) error {
	return s.bot.SendMessage(chatID, text, nil)
}

// SendMessageWithAction attaches a single inline callback button (e.g. a bonus
// announcement's "Claim" button), satisfying domain.ActionBroadcastSender.
func (s telegramBroadcastSender) SendMessageWithAction(chatID int64, text string, action domain.BroadcastAction) error {
	return s.bot.SendMessage(chatID, text, &telegram.ReplyMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{{
			{Text: action.Text, CallbackData: action.CallbackData},
		}},
	})
}

// resolveAllowedOrigins returns the browser origins permitted to reach this
// API. ALLOWED_ORIGINS (comma-separated) overrides the defaults, so moving the
// frontends to a new host is an env change, not a code change.
//
// Both the CORS middleware and the WebSocket upgrader read from this single
// list — the socket used to accept every origin unconditionally, which meant
// tightening CORS still left that door open.
func resolveAllowedOrigins() []string {
	origins := []string{"http://localhost:3000", "http://localhost:3001", "http://localhost:3002", "http://localhost:5174", "https://bingo-frontend-production-7ee9.up.railway.app", "https://winner.up.railway.app", "https://bingo-miniapp-gold.vercel.app", "https://bingo-frontend-azure.vercel.app"}
	if env := os.Getenv("ALLOWED_ORIGINS"); env != "" {
		origins = origins[:0]
		for _, o := range strings.Split(env, ",") {
			if o = strings.TrimSpace(o); o != "" {
				origins = append(origins, o)
			}
		}
	}
	return origins
}

// resolveTrustedProxies lists the networks a reverse proxy may reach this
// service from. TRUSTED_PROXIES (comma-separated CIDRs) overrides it.
//
// The default covers the private and CGNAT ranges a platform edge uses to
// reach containers. If it is wrong the failure is loud rather than silent:
// every caller collapses to the proxy's address, one shared bucket, and the
// per-IP limits bite far too early.
func resolveTrustedProxies() []string {
	proxies := []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", // RFC1918
		"100.64.0.0/10", // CGNAT — Railway reaches containers from 100.64.0.0/10
		"127.0.0.1/32",
		"fd00::/8", "::1/128", // IPv6 ULA + loopback
		// Railway's public edge, which appends the caller to X-Forwarded-For.
		// Without this the right-most hop is untrusted and gin stops there, so
		// ClientIP returns the edge address and every caller collapses into a
		// single bucket. Confirmed in production via /debug/client-ip:
		// RemoteAddr 100.64.0.3, XFF "<caller>, 79.127.178.81".
		// If Railway ever changes this range the symptom is per-IP limits
		// biting far too early — check /debug/client-ip first.
		"79.127.178.0/24",
	}
	if env := os.Getenv("TRUSTED_PROXIES"); env != "" {
		proxies = proxies[:0]
		for _, p := range strings.Split(env, ",") {
			if p = strings.TrimSpace(p); p != "" {
				proxies = append(proxies, p)
			}
		}
	}
	return proxies
}

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	allowOrigins := resolveAllowedOrigins()

	// Initialize database connection
	db, err := sql.Open("postgres", cfg.Database.GetDSN())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Test database connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	// Set connection pool settings for high concurrency
	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)

	// Initialize Redis client
	redisClient, err := redisPkg.NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	// Initialize repositories
	userRepo := postgres.NewUserRepository(db)
	walletRepo := postgres.NewWalletRepository(db)
	transactionRepo := postgres.NewTransactionRepository(db)
	gameRepo := postgres.NewGameRepository(db)
	botRepo := postgres.NewBotRepository(db)
	supportRepo := postgres.NewSupportRepository(db)

	// Initialize Redis services
	gameStateService := redisPkg.NewGameStateService(redisClient.GetClient())

	// Initialize JWT service
	jwtService := jwt.NewService(cfg)

	// Initialize use cases
	// Every verifier lookup is persisted through the log repo for the admin audit
	// view (raw provider response + verdict per receipt).
	verificationLogRepo := postgres.NewVerificationLogRepository(db)
	adminEventLogRepo := postgres.NewAdminEventLogRepository(db)
	paymentVerifier := payment.NewVerifierWithRecorder(cfg.PaymentVerifier, verificationLogRepo)
	bonusRepo := postgres.NewBonusRepository(db)
	// Referral rewards are granted as play-only bonus (see CreateUser), so the
	// user use case needs the bonus repo.
	userUseCase := usecase.NewUserUseCase(userRepo, walletRepo, bonusRepo, db)
	walletUseCase := usecase.NewWalletUseCase(walletRepo, transactionRepo, userRepo, gameRepo, bonusRepo, db, paymentVerifier, verificationLogRepo)
	authUseCase := usecase.NewAuthUseCase(userRepo, jwtService, cfg.Admin.SecretCode, cfg.Telegram.BotToken)
	gameUseCase := usecase.NewGameUseCase(gameRepo, walletRepo, transactionRepo, userRepo, bonusRepo, botRepo, adminEventLogRepo, db, gameStateService)
	botUseCase := usecase.NewBotUseCase(botRepo, userRepo, walletRepo, transactionRepo, gameRepo, gameUseCase, gameStateService, db, usecase.BotSettings{
		PoolSize:        cfg.Bots.PoolSize,
		WalletFloat:     cfg.Bots.WalletFloat,
		MaxJoinsPerTick: cfg.Bots.MaxJoinsPerTick,
		CheckInterval:   time.Duration(cfg.Bots.CheckInterval) * time.Second,
		JoinDelay:       time.Duration(cfg.Bots.JoinDelay) * time.Second,
		WinRate:         cfg.Bots.WinRate,
	})
	supportUseCase := usecase.NewSupportUseCase(supportRepo)
	// One bot client, shared by everything that messages players: the game's
	// own replies, bonus grant notices, and admin broadcasts. They share a
	// token and therefore a rate-limit budget, so they should share a client.
	telegramBot := telegram.NewBot(cfg.Telegram.BotToken)
	// Keep the in-chat menu button as a Mini App launcher (next to the message
	// box). NOTE: the "Open" shortcut in the CHAT LIST is the separate BotFather
	// "Main Mini App" setting and cannot be changed here.
	if err := telegramBot.SetWebAppMenuButton("🎮 Play", handler.VersionedMiniAppURL(cfg.Telegram.MiniAppURL)); err != nil {
		log.Printf("[telegram] could not set chat menu button: %v", err)
	}
	bonusUseCase := usecase.NewBonusUseCase(bonusRepo, userRepo, db, telegramBroadcastSender{bot: telegramBot})
	// The bot exists now, so notify a referrer after the invited player's first real deposit.
	walletUseCase.SetReferralNotifier(telegramBroadcastSender{bot: telegramBot})

	// Initialize handlers
	userHandler := handler.NewUserHandler(userUseCase)
	walletHandler := handler.NewWalletHandler(walletUseCase, verificationLogRepo)
	authHandler := handler.NewAuthHandler(authUseCase)
	gameHandler := handler.NewGameHandler(gameUseCase)
	botHandler := handler.NewBotHandler(botUseCase)
	supportHandler := handler.NewSupportHandler(supportUseCase)
	bonusHandler := handler.NewBonusHandler(bonusUseCase)
	adminLogHandler := handler.NewAdminLogHandler(adminEventLogRepo)
	wsHandler := handler.NewWebSocketHandler(redisClient.GetClient(), gameStateService, gameUseCase, allowOrigins)

	// Promo codes: created by admins, redeemed through the bot menu.
	promoRepo := postgres.NewPromoRepository(db, walletRepo, transactionRepo, bonusRepo)
	promoHandler := handler.NewPromoHandler(promoRepo)

	// Admin broadcasts over the same bot token as the game's own messages.
	broadcastRepo := postgres.NewBroadcastRepository(db)
	broadcastUseCase := usecase.NewBroadcastUseCase(broadcastRepo, telegramBroadcastSender{bot: telegramBot})
	broadcastHandler := handler.NewBroadcastHandler(broadcastUseCase)

	// "First N players" bonus giveaways. Declared after broadcastUseCase
	// because creating a campaign can announce itself to every player.
	bonusCampaignRepo := postgres.NewBonusCampaignRepository(db)
	bonusCampaignUseCase := usecase.NewBonusCampaignUseCase(bonusCampaignRepo, bonusRepo, userRepo, db, broadcastUseCase, telegramBroadcastSender{bot: telegramBot}, redisClient.GetClient())
	bonusCampaignHandler := handler.NewBonusCampaignHandler(bonusCampaignUseCase)

	// Telegram bot: registration gateway + Mini App launcher (webhook-driven),
	// plus in-chat balance/deposit/withdraw and today's-bonus claim. Deposit
	// shows the house account for each method, so the bot needs the same numbers
	// the verifier binds receipts to; the campaign use case powers the in-chat
	// Claim button. Built after the campaign use case it depends on.
	depositAccounts := map[domain.PaymentMethod]string{
		domain.PaymentMethodTelebirr: cfg.PaymentVerifier.TelebirrAccount,
		domain.PaymentMethodCBEBirr:  cfg.PaymentVerifier.CBEBirrAccount,
		domain.PaymentMethodMpesa:    cfg.PaymentVerifier.MpesaAccount,
	}
	telegramHandler := handler.NewTelegramHandler(userUseCase, walletUseCase, bonusUseCase, bonusCampaignUseCase, promoRepo, telegramBot, cfg.Telegram.WebhookSecret, cfg.Telegram.MiniAppURL, cfg.Telegram.BotUsername, depositAccounts)

	// Live admin feed: pushes campaign claims to the dashboard over WebSocket
	// so a stampede fills in real time instead of on a refresh timer.
	adminWSHandler := handler.NewAdminWSHandler(redisClient.GetClient(), jwtService, allowOrigins)

	// Setup router
	router := setupRouter(userHandler, walletHandler, authHandler, gameHandler, botHandler, supportHandler, wsHandler, adminWSHandler, telegramHandler, promoHandler, bonusHandler, bonusCampaignHandler, broadcastHandler, adminLogHandler, jwtService, cfg.Internal.APISecret, redisClient.GetClient(), cfg.RateLimits)

	// Shared background context for the server's housekeeping goroutines,
	// cancelled on shutdown.
	botCtx, botCancel := context.WithCancel(context.Background())
	defer botCancel()

	// Resume any game left mid-flight by the previous process (a deploy or crash).
	// Without this, a redeploy freezes live COUNTDOWN/DRAWING games forever with
	// players' stakes locked. Safe during a rolling deploy: the per-game draw
	// lease means only one instance ever draws a given game. Runs in a goroutine
	// so a slow Redis/DB can't delay startup.
	go func() {
		defer utils.RecoverPanic("resume-active-games")
		gameUseCase.ResumeActiveGames(botCtx)
	}()

	// Empty-game sweeper: periodically cancel abandoned/never-joined WAITING
	// games (0 players) so they drop out of the lobby and admin active list.
	go func() {
		defer utils.RecoverPanic("empty-game-sweeper")
		ticker := time.NewTicker(domain.EmptyGameCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-botCtx.Done():
				return
			case <-ticker.C:
				// Per-iteration recovery so one bad sweep can't kill the loop.
				func() {
					defer utils.RecoverPanic("empty-game-sweep")
					if n, err := gameUseCase.CleanupEmptyGames(botCtx); err != nil {
						log.Printf("Warning: empty-game cleanup failed: %v", err)
					} else if n > 0 {
						log.Printf("Empty-game cleanup: cancelled %d abandoned game(s)", n)
					}
				}()
			}
		}
	}()

	// Filler bots: seed the pool once, then run the background auto-filler.
	// Gated by BOTS_ENABLED; the fill POLICY itself is toggled from the admin
	// dashboard (bot_config), so this only decides whether the machinery runs.
	if cfg.Bots.Enabled {
		go func() {
			defer utils.RecoverPanic("bot-filler")
			// Generous timeout: seeding is sequential (one tx per bot), so a
			// large pool (e.g. 1000) needs minutes, not seconds. Runs in a
			// goroutine so it never blocks server startup, and is idempotent —
			// a partial seed resumes on the next boot.
			seedCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			if err := botUseCase.EnsureBotPool(seedCtx, cfg.Bots.PoolSize); err != nil {
				log.Printf("Warning: failed to seed bot pool: %v", err)
			}
			botUseCase.Run(botCtx)
		}()
		log.Printf("Filler bots enabled (pool=%d, float=%.0f)", cfg.Bots.PoolSize, cfg.Bots.WalletFloat)
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Server starting on %s:%s", cfg.Server.Host, cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func setupRouter(userHandler *handler.UserHandler, walletHandler *handler.WalletHandler, authHandler *handler.AuthHandler, gameHandler *handler.GameHandler, botHandler *handler.BotHandler, supportHandler *handler.SupportHandler, wsHandler *handler.WebSocketHandler, adminWSHandler *handler.AdminWSHandler, telegramHandler *handler.TelegramHandler, promoHandler *handler.PromoHandler, bonusHandler *handler.BonusHandler, bonusCampaignHandler *handler.BonusCampaignHandler, broadcastHandler *handler.BroadcastHandler, adminLogHandler *handler.AdminLogHandler, jwtService *jwt.Service, internalAPISecret string, rdb *redis.Client, rl config.RateLimitsConfig) *gin.Engine {
	// Rate-limit buckets. Auth buckets are per-IP (no user to key on yet);
	// the money buckets sit behind AuthMiddleware and key on the user id.
	secs := func(n int) time.Duration { return time.Duration(n) * time.Second }
	limitLogin := middleware.RateLimit(rdb, "login", middleware.RateLimitRule{Limit: rl.LoginLimit, Window: secs(rl.LoginWindow)})
	limitCreateAdmin := middleware.RateLimit(rdb, "create-admin", middleware.RateLimitRule{Limit: rl.CreateAdminLimit, Window: secs(rl.CreateAdminWindow)})
	limitTelegramAuth := middleware.RateLimit(rdb, "telegram-auth", middleware.RateLimitRule{Limit: rl.TelegramAuthLimit, Window: secs(rl.TelegramAuthWindow)})
	limitDeposit := middleware.RateLimit(rdb, "deposit", middleware.RateLimitRule{Limit: rl.DepositLimit, Window: secs(rl.DepositWindow)})
	limitWithdraw := middleware.RateLimit(rdb, "withdraw", middleware.RateLimitRule{Limit: rl.WithdrawLimit, Window: secs(rl.WithdrawWindow)})
	limitTransfer := middleware.RateLimit(rdb, "transfer", middleware.RateLimitRule{Limit: rl.TransferLimit, Window: secs(rl.TransferWindow)})
	limitWebSocket := middleware.RateLimit(rdb, "websocket", middleware.RateLimitRule{Limit: rl.WebSocketLimit, Window: secs(rl.WebSocketWindow)})

	// Set Gin to release mode in production
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Trust only the platform's reverse proxy when resolving the client IP.
	//
	// Gin's default is to trust EVERY proxy, which makes c.ClientIP() return
	// the left-most X-Forwarded-For entry — a value the caller sets. Any
	// per-IP rate limit would then be bypassed by varying that header per
	// request, so this has to be right for the limiter to mean anything.
	// With a trusted list, gin walks X-Forwarded-For from the right and skips
	// trusted hops, landing on the address the platform's edge actually saw.
	//
	// TRUSTED_PROXIES (comma-separated CIDRs) overrides the default, which
	// covers the private and CGNAT ranges a platform edge reaches containers
	// over. If it is ever wrong the failure is visible rather than silent:
	// every caller collapses to one identity and the buckets throttle early.
	if err := router.SetTrustedProxies(resolveTrustedProxies()); err != nil {
		log.Fatalf("Invalid TRUSTED_PROXIES: %v", err)
	}

	router.Use(cors.New(cors.Config{
		AllowOrigins:     resolveAllowedOrigins(),
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type", "Authorization", "Upgrade", "Connection", "Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Extensions", "Sec-WebSocket-Protocol"},
		ExposeHeaders:    []string{"Content-Length", "Upgrade", "Connection", "Sec-WebSocket-Accept"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// Debug middleware for WebSocket routes
	router.Use(func(c *gin.Context) {
		if c.Request.URL.Path == "/api/v1/ws/game" || strings.HasPrefix(c.Request.URL.Path, "/api/v1/ws/game") {
			log.Printf("[Middleware] WebSocket request - Method: %s, Path: %s, Query: %s, Upgrade: %s, Connection: %s",
				c.Request.Method, c.Request.URL.Path, c.Request.URL.RawQuery,
				c.Request.Header.Get("Upgrade"), c.Request.Header.Get("Connection"))
		}
		c.Next()
	})

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// Echoes how this service resolved the caller's address, and the
	// forwarding headers it based that on. It returns only metadata the caller
	// themselves sent, so it discloses nothing about anyone else.
	//
	// It exists because getting client-IP resolution wrong is both easy and
	// invisible: every per-IP rate limit silently becomes one global bucket
	// shared by every player, which looks like working rate limiting right up
	// until real users start getting 429s. `resolved_client_ip` must equal the
	// caller's real public address; if it shows a platform edge address
	// instead, TRUSTED_PROXIES does not cover that edge.
	router.GET("/debug/client-ip", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"resolved_client_ip":       c.ClientIP(),
			"remote_addr":              c.Request.RemoteAddr,
			"x_forwarded_for":          c.Request.Header.Get("X-Forwarded-For"),
			"x_real_ip":                c.Request.Header.Get("X-Real-Ip"),
			"x_envoy_external_address": c.Request.Header.Get("X-Envoy-External-Address"),
			"trusted_proxies":          resolveTrustedProxies(),
		})
	})

	// Telegram bot webhook (set via the Bot API setWebhook method).
	router.POST("/telegram/webhook", telegramHandler.Webhook)

	// API routes
	api := router.Group("/api/v1")
	{
		// Public auth endpoints
		auth := api.Group("/auth")
		{
			auth.POST("/login", limitLogin, authHandler.Login)                    // admin login (telegram_id + password)
			auth.POST("/create-admin", limitCreateAdmin, authHandler.CreateAdmin) // secret-code gated
			auth.POST("/telegram", limitTelegramAuth, authHandler.TelegramLogin)  // website: verify Mini App initData -> JWT
		}

		// Bot-facing user endpoints (server-to-server; called by the trusted Telegram bot).
		// Gated by the internal API secret — they expose other users' data by ID.
		user := api.Group("/user")
		user.Use(middleware.InternalSecretMiddleware(internalAPISecret))
		{
			user.POST("/register", userHandler.Register)
			user.GET("/telegram/:telegram_id", userHandler.FindByTelegramID)
			user.GET("/phone", userHandler.FindByPhone)
			user.GET("/referral/:referral_code", userHandler.FindByReferralCode)
		}

		// Bot-facing wallet reads (server-to-server). Gated by the internal API
		// secret — they return any user's balance/ledger by ID.
		wallet := api.Group("/wallet")
		wallet.Use(middleware.InternalSecretMiddleware(internalAPISecret))
		{
			wallet.GET("/telegram/:telegram_id", walletHandler.GetWalletByTelegramID)
			wallet.GET("/:user_id", walletHandler.GetWallet)
			wallet.GET("/:user_id/deposits", walletHandler.GetDepositHistory)
			wallet.GET("/:user_id/withdrawals", walletHandler.GetWithdrawalHistory)
			wallet.GET("/:user_id/transfers", walletHandler.GetTransferHistory)
		}

		// Public app status (no auth): the Mini App polls this to decide whether to
		// show the maintenance screen.
		api.GET("/status", walletHandler.GetPublicStatus)

		// Public game reads (spectating / lobby — no auth needed)
		games := api.Group("/games")
		{
			games.GET("", gameHandler.GetGames)
			games.GET("/recent-winners", gameHandler.GetRecentWinners)
			games.GET("/:gameId/state", gameHandler.GetGameState)
		}

		// Bot-facing per-user game reads (server-to-server). These leak another
		// user's game history / card by ID, so they sit behind the internal secret.
		// Player-facing clients must use the JWT-scoped /me/games endpoints instead.
		gamesInternal := api.Group("/games")
		gamesInternal.Use(middleware.InternalSecretMiddleware(internalAPISecret))
		{
			gamesInternal.GET("/user/:user_id/history", gameHandler.GetGameHistory)
			gamesInternal.GET("/:gameId/players/:userId", gameHandler.GetPlayerInGame)
		}

		// Authenticated website endpoints (JWT required; user_id comes from the token)
		authed := api.Group("")
		authed.Use(middleware.AuthMiddleware(jwtService))
		// Maintenance gate: while maintenance mode is on, player mutations (join,
		// deposit, withdraw, transfer, …) are rejected with 503. Reads pass through
		// so the app can load and render the maintenance screen. Admin and
		// bot-facing groups are unaffected.
		authed.Use(middleware.Maintenance(walletHandler.MaintenanceStatus))
		{
			// Profile
			authed.GET("/me", userHandler.GetMe)
			authed.PUT("/me/name", userHandler.UpdateMyName)

			// Wallet (self)
			authed.GET("/me/wallet", walletHandler.GetMyWallet)
			authed.GET("/me/bonus", bonusHandler.GetMyBonus)
			// "First N players" giveaway: what is running, and taking a slot.
			authed.GET("/me/bonus/campaign", bonusCampaignHandler.GetMyCampaign)
			authed.POST("/me/bonus/claim", bonusCampaignHandler.Claim)
			authed.GET("/me/wallet/deposits", walletHandler.GetMyDeposits)
			authed.GET("/me/wallet/withdrawals", walletHandler.GetMyWithdrawals)
			authed.GET("/me/wallet/transfers", walletHandler.GetMyTransfers)
			authed.POST("/wallet/deposit", limitDeposit, walletHandler.Deposit)
			authed.POST("/wallet/withdraw", limitWithdraw, walletHandler.Withdraw)
			authed.POST("/wallet/transfer", limitTransfer, walletHandler.Transfer)

			// Games (self)
			authed.GET("/me/active-game", gameHandler.GetMyActiveGame)
			authed.GET("/me/games", gameHandler.GetMyGameHistory)
			authed.GET("/me/winnings", gameHandler.GetMyWinnings)
			authed.GET("/me/games/:gameId", gameHandler.GetMyPlayerInGame)
			authed.GET("/me/games/:gameId/cards", gameHandler.GetMyCardsInGame)
			authed.POST("/games/:gameId/join", gameHandler.JoinGame)
			authed.POST("/games/:gameId/leave", gameHandler.LeaveGame)
			authed.POST("/games/:gameId/bingo", gameHandler.ClaimBingo)

			// Report a problem (transaction / gameplay / other) — reaches the
			// admin dashboard.
			authed.POST("/support", supportHandler.SubmitReport)
		}

		// Public card endpoints
		cards := api.Group("/cards")
		{
			cards.GET("/:cardId", gameHandler.GetCardData)
		}

		// WebSocket endpoints
		// Connect by game type (public viewing): /ws/game?type=G5
		// Connect by game ID: /ws/game/:gameId
		// Note: Order matters - more specific route first
		// Also handle OPTIONS for CORS preflight
		api.OPTIONS("/ws/game/:gameId", func(c *gin.Context) {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Upgrade, Connection, Sec-WebSocket-Key, Sec-WebSocket-Version")
			c.Status(http.StatusOK)
		})
		api.OPTIONS("/ws/game", func(c *gin.Context) {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Upgrade, Connection, Sec-WebSocket-Key, Sec-WebSocket-Version")
			c.Status(http.StatusOK)
		})
		api.GET("/ws/game/:gameId", limitWebSocket, wsHandler.HandleWebSocket)
		api.GET("/ws/game", limitWebSocket, wsHandler.HandleWebSocket)
		// Live admin bonus-campaign feed. Authenticates via a ?token= query
		// param (a browser cannot set headers on a WebSocket), validated inside
		// the handler, so it sits outside the header-based admin middleware.
		api.GET("/ws/admin/bonus-campaign", limitWebSocket, adminWSHandler.BonusCampaign)

		// Protected admin endpoints
		admin := api.Group("/admin")
		admin.Use(middleware.AuthMiddleware(jwtService))
		admin.Use(middleware.AdminMiddleware())
		{
			// User management
			admin.GET("/users", userHandler.GetAllUsers)
			admin.GET("/users/:user_id", userHandler.GetUserDetail)
			admin.GET("/users/:user_id/game-stats", gameHandler.GetUserGameStats)
			admin.GET("/users/:user_id/transactions", walletHandler.GetUserTransactions)
			admin.GET("/users/:user_id/games", gameHandler.GetUserGamesAdmin)
			admin.GET("/users/:user_id/referrals", userHandler.GetReferredUsers)

			// App settings (minimum deposit, …) — editable from the admin dashboard.
			admin.GET("/settings", walletHandler.GetSettings)
			admin.PUT("/settings", walletHandler.UpdateSettings)
			admin.POST("/users/:user_id/role", userHandler.SetUserRole)
			admin.POST("/users/:user_id/make-admin", userHandler.MakeAdmin)
			admin.POST("/users/:user_id/ban", userHandler.BanUser)
			admin.POST("/users/:user_id/unban", userHandler.UnbanUser)
			admin.POST("/users/:user_id/adjust-balance", walletHandler.AdjustBalance)
			admin.DELETE("/users/:user_id", userHandler.DeleteUser)

			// Dashboard stats
			stats := admin.Group("/stats")
			{
				stats.GET("/dashboard", walletHandler.GetDashboardStats)
			}
			admin.GET("/dashboard/house-cut", walletHandler.GetHouseCutDetail)

			// Structured operational warnings surfaced from backend code paths.
			admin.GET("/logs", adminLogHandler.List)

			// Bonus wallet: policy, grants, and the house's live liability.
			bonus := admin.Group("/bonus")
			{
				bonus.GET("/config", bonusHandler.GetConfig)
				bonus.PUT("/config", bonusHandler.UpdateConfig)
				bonus.POST("/grant", bonusHandler.GrantBonus)
				bonus.POST("/grant-bulk", bonusHandler.GrantBonusBulk)
				bonus.GET("/outstanding", bonusHandler.GetOutstanding)

				// "First N players" giveaways. Creating one can announce it to
				// every player in the same call.
				bonus.POST("/campaigns", bonusCampaignHandler.CreateCampaign)
				bonus.GET("/campaigns", bonusCampaignHandler.ListCampaigns)
				bonus.GET("/campaigns/:id/claims", bonusCampaignHandler.ListClaims)
				bonus.POST("/campaigns/:id/end", bonusCampaignHandler.EndCampaign)
			}
			admin.GET("/users/:user_id/bonus", bonusHandler.ListUserGrants)

			// Telegram broadcasts to every registered player.
			admin.POST("/broadcast", broadcastHandler.Send)
			admin.GET("/broadcast/audience", broadcastHandler.Audience)
			admin.GET("/broadcast/:id", broadcastHandler.Get)
			admin.GET("/broadcasts", broadcastHandler.List)

			// Promo codes (redeemed by players through the bot menu)
			promos := admin.Group("/promo-codes")
			{
				promos.GET("", promoHandler.List)
				promos.POST("", promoHandler.Create)
				promos.POST("/:code/activate", promoHandler.SetActive(true))
				promos.POST("/:code/deactivate", promoHandler.SetActive(false))
			}

			// Game management
			games := admin.Group("/games")
			{
				games.GET("", gameHandler.AdminListGames)                  // list games (?state=&type=&limit=&offset=)
				games.GET("/:gameId", gameHandler.AdminGetGame)            // game detail + players
				games.POST("/:gameId/cancel", gameHandler.AdminCancelGame) // force-cancel + refund stakes
				games.POST("/:gameId/add-bots", botHandler.AddBots)        // manually inject filler bots
			}

			// Filler-bot control (auto-fill policy + pool seeding)
			bots := admin.Group("/bots")
			{
				bots.GET("/config", botHandler.GetConfig)    // read auto-fill policy
				bots.PUT("/config", botHandler.UpdateConfig) // edit auto-fill policy
				bots.POST("/seed", botHandler.SeedPool)      // (re)create + fund the bot pool
			}

			// Transaction queries
			transactions := admin.Group("/transactions")
			{
				transactions.GET("", walletHandler.GetAllTransactions)
				transactions.GET("/pending/deposits", walletHandler.GetPendingDeposits)
				transactions.GET("/pending/withdrawals", walletHandler.GetPendingWithdrawals)
				transactions.GET("/completed/deposits", walletHandler.GetCompletedDeposits)
				transactions.GET("/completed/withdrawals", walletHandler.GetCompletedWithdrawals)
				transactions.GET("/failed", walletHandler.GetFailedTransactions)
				transactions.GET("/transfers", walletHandler.GetTransferTransactions)
				transactions.GET("/winners", walletHandler.GetRealPlayerWinnings)
				// Payment-verifier audit trail (raw provider response + verdict per
				// receipt) for investigating disputed deposits. ?reference=&limit=&offset=
				transactions.GET("/verification-logs", walletHandler.GetVerificationLogs)
			}

			// Deposit operations
			admin.POST("/transactions/:id/approve-deposit", walletHandler.ApproveDeposit)
			admin.POST("/transactions/:id/reject-deposit", walletHandler.RejectDeposit)

			// Withdrawal operations
			admin.POST("/transactions/:id/approve-withdrawal", walletHandler.ApproveWithdrawal)
			admin.POST("/transactions/:id/reject-withdrawal", walletHandler.RejectWithdrawal)
			admin.POST("/transactions/:id/reject-to-bonus", walletHandler.RejectWithdrawalToBonus)

			// General operations
			admin.POST("/transactions/:id/cancel", walletHandler.CancelTransaction)

			// Player problem reports (triage queue)
			support := admin.Group("/support")
			{
				support.GET("", supportHandler.ListReports)                // list (?status=open|resolved&limit=&offset=)
				support.POST("/:id/resolve", supportHandler.ResolveReport) // mark handled
			}
		}
	}

	return router
}
