package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Server          ServerConfig
	Database        DatabaseConfig
	Redis           RedisConfig
	JWT             JWTConfig
	Admin           AdminConfig
	Telegram        TelegramConfig
	PaymentVerifier PaymentVerifierConfig
	Internal        InternalConfig
	Bots            BotsConfig
	RateLimits      RateLimitsConfig
}

// RateLimitsConfig holds the per-bucket request ceilings. Each is "how many
// requests per how many seconds"; setting a limit to 0 disables that bucket.
//
// The player-facing buckets are keyed on the authenticated user id rather than
// the IP (see middleware.RateLimit), so they can be tight without punishing the
// many subscribers an Ethiopian mobile carrier puts behind one NAT address.
//
// The per-IP buckets are necessarily loose, because measurement against the
// live service showed client addresses here are unreliable in BOTH directions:
//   - many subscribers share one carrier NAT address, so a bucket is shared by
//     strangers and a tight limit locks out bystanders; and
//   - a single client's address ROTATES between requests. Eight consecutive
//     calls from one machine arrived as 196.191.60.129, 152.233.56.193 and
//     152.233.56.194.
//
// So a per-IP limit both over-counts (shared) and under-counts (rotating). It
// is worth having as a blunt cost on automated abuse, but it is not a reliable
// per-actor control and must not be sized as though it were. Anything that
// needs to actually bind to one actor should key on the user id, or on the
// submitted identifier where there is no session yet.
type RateLimitsConfig struct {
	LoginLimit         int // per-IP: admin password login
	LoginWindow        int
	CreateAdminLimit   int // per-IP: secret-code gated admin creation
	CreateAdminWindow  int
	TelegramAuthLimit  int // per-IP: Mini App initData -> JWT
	TelegramAuthWindow int
	DepositLimit       int // per-user: deposit submission/verification
	DepositWindow      int
	WithdrawLimit      int // per-user: withdrawal requests
	WithdrawWindow     int
	TransferLimit      int // per-user: player-to-player transfers
	TransferWindow     int
	WebSocketLimit     int // per-IP: socket connection attempts
	WebSocketWindow    int
}

// BotsConfig holds the runtime knobs for the filler-bot subsystem. The actual
// fill POLICY (enabled, thresholds, target) lives in the DB and is edited from
// the admin dashboard; these are operational defaults set once at deploy time.
type BotsConfig struct {
	Enabled         bool    // run the background auto-filler goroutine at all
	PoolSize        int     // how many bot accounts to seed on boot
	WalletFloat     float64 // house money each bot wallet is topped up to (birr)
	MaxJoinsPerTick int     // bots added per game per sweep (spaces out joins)
	CheckInterval   int     // seconds between auto-fill sweeps
	JoinDelay       int     // seconds to hold bots back after the first real player joins
	WinRate         float64 // probability that bots win co-winner situations (0-1)
}

// InternalConfig gates the server-to-server ("bot-facing") /user, /wallet and
// per-user /games reads. Callers must present APISecret in the
// X-Internal-Api-Secret header. When empty, those endpoints are disabled
// (fail closed) so they can never be reached anonymously from the public net.
type InternalConfig struct {
	APISecret string
}

type ServerConfig struct {
	Port         string
	Host         string
	ReadTimeout  int
	WriteTimeout int
	IdleTimeout  int
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

type JWTConfig struct {
	SecretKey       string
	ExpirationHours int
}

type AdminConfig struct {
	SecretCode string
}

type TelegramConfig struct {
	BotToken      string // used to verify Mini App initData AND authenticate Bot API calls
	WebhookSecret string // shared secret Telegram echoes back in X-Telegram-Bot-Api-Secret-Token
	MiniAppURL    string // URL the bot's "Play" button opens (the Vercel Mini App)
	BotUsername   string // @username (no @), used to build in-chat invite deep links
}

type PaymentVerifierConfig struct {
	BaseURL string
	APIKey  string
	// TelebirrAccount, CBEBirrAccount and MpesaAccount are the house numbers that
	// deposits of each method must be paid to. When set, the verifier rejects any
	// receipt credited to a different account, so a valid receipt for money sent
	// elsewhere can't be claimed. While a method's number is blank its deposits
	// are never auto-credited — they queue for manual admin approval. CBE Birr
	// additionally needs its number to even look receipts up (receipts are
	// fetched by receiver phone).
	TelebirrAccount string
	CBEBirrAccount  string
	MpesaAccount    string
	// TelebirrName, CBEBirrName and MpesaName are the house account HOLDER NAMES
	// per method. When set, a receipt whose credited-party name is revealed by the
	// verifier must ALSO match this name (in addition to the account number) before
	// it can auto-credit — defence in depth so a receipt paid to a look-alike
	// number can't be claimed. Blank disables the name cross-check for that method.
	TelebirrName string
	CBEBirrName  string
	MpesaName    string
	// DebugLog, when true, logs the raw verifier response body for every lookup so
	// operators can see exactly which fields each provider returns (sender/receiver
	// names, account numbers, amount keys). Off by default because responses carry
	// PII; enable temporarily via VERIFY_DEBUG_LOG=true.
	DebugLog bool
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if it exists (optional)
	_ = godotenv.Load()

	config := &Config{
		Server: ServerConfig{
			// Railway uses PORT, fallback to SERVER_PORT
			Port:         getEnv("PORT", getEnv("SERVER_PORT", "8080")),
			Host:         getEnv("SERVER_HOST", "0.0.0.0"),
			ReadTimeout:  15,
			WriteTimeout: 15,
			IdleTimeout:  60,
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", getEnv("PGHOST", "localhost")),
			Port:     getEnv("DB_PORT", getEnv("PGPORT", "5432")),
			User:     getEnv("DB_USER", getEnv("PGUSER", "postgres")),
			Password: getEnv("DB_PASSWORD", getEnv("PGPASSWORD", "postgres")),
			DBName:   getEnv("DB_NAME", getEnv("PGDATABASE", "bingo")),
			SSLMode:  getEnv("DB_SSLMODE", getEnv("PGSSLMODE", "disable")),
		},
		Redis: parseRedisConfig(),
		JWT: JWTConfig{
			SecretKey:       getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
			ExpirationHours: getEnvInt("JWT_EXPIRATION_HOURS", 24),
		},
		Admin: AdminConfig{
			SecretCode: getEnv("SECRET_CODE", ""),
		},
		Telegram: TelegramConfig{
			BotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
			WebhookSecret: getEnv("TELEGRAM_WEBHOOK_SECRET", ""),
			MiniAppURL:    getEnv("TELEGRAM_MINIAPP_URL", "https://bingo-miniapp-gold.vercel.app"),
			BotUsername:   getEnv("TELEGRAM_BOT_USERNAME", "EDL_Bingobot"),
		},
		PaymentVerifier: PaymentVerifierConfig{
			BaseURL:         strings.TrimRight(getEnv("VERIFY_API_BASE_URL", "https://verifyapi.leulzenebe.pro"), "/"),
			APIKey:          getEnv("VERIFY_API_KEY", ""),
			TelebirrAccount: getEnv("VERIFY_TELEBIRR_ACCOUNT", ""),
			CBEBirrAccount:  getEnv("VERIFY_CBEBIRR_ACCOUNT", "0990878233"),
			MpesaAccount:    getEnv("VERIFY_MPESA_ACCOUNT", ""),
			TelebirrName:    getEnv("VERIFY_TELEBIRR_NAME", ""),
			CBEBirrName:     getEnv("VERIFY_CBEBIRR_NAME", "Danasa beyene mana"),
			MpesaName:       getEnv("VERIFY_MPESA_NAME", ""),
			DebugLog:        strings.EqualFold(getEnv("VERIFY_DEBUG_LOG", ""), "true"),
		},
		Internal: InternalConfig{
			APISecret: getEnv("INTERNAL_API_SECRET", ""),
		},
		Bots: BotsConfig{
			Enabled:         getEnv("BOTS_ENABLED", "false") == "true",
			PoolSize:        getEnvInt("BOT_POOL_SIZE", 30),
			WalletFloat:     float64(getEnvInt("BOT_WALLET_FLOAT", 1_000_000)),
			MaxJoinsPerTick: getEnvInt("BOT_MAX_JOINS_PER_TICK", 5),
			CheckInterval:   getEnvInt("BOT_CHECK_INTERVAL_SECONDS", 5),
			JoinDelay:       getEnvInt("BOT_JOIN_DELAY_SECONDS", 5),
			WinRate:         parseWinRate(getEnv("BOT_WIN_RATE", "0.8")),
		},
		RateLimits: RateLimitsConfig{
			// Per-IP, so a blunt instrument here (see the type comment): the
			// bucket is shared by everyone behind a carrier NAT, and a single
			// attacker's address rotates anyway. Sized to make scripted
			// credential stuffing costly without locking out the admins who
			// share an address with them. Binding brute-force protection to a
			// specific ACCOUNT wants a per-identifier bucket, not this.
			LoginLimit:  getEnvInt("RL_LOGIN_LIMIT", 50),
			LoginWindow: getEnvInt("RL_LOGIN_WINDOW_SECONDS", 900),
			// Per-IP. Creating an admin is the highest-value action here and
			// legitimately happens a handful of times ever.
			CreateAdminLimit:  getEnvInt("RL_CREATE_ADMIN_LIMIT", 5),
			CreateAdminWindow: getEnvInt("RL_CREATE_ADMIN_WINDOW_SECONDS", 3600),
			// Per-IP, and the one bucket a whole carrier shares — every Mini
			// App open hits it, so it is generous on purpose. It exists to
			// blunt initData forgery attempts, not to pace normal use.
			TelegramAuthLimit:  getEnvInt("RL_TELEGRAM_AUTH_LIMIT", 300),
			TelegramAuthWindow: getEnvInt("RL_TELEGRAM_AUTH_WINDOW_SECONDS", 60),
			// Per-user from here down, so these are about one account's
			// behaviour and NAT is irrelevant.
			DepositLimit:    getEnvInt("RL_DEPOSIT_LIMIT", 10),
			DepositWindow:   getEnvInt("RL_DEPOSIT_WINDOW_SECONDS", 60),
			WithdrawLimit:   getEnvInt("RL_WITHDRAW_LIMIT", 5),
			WithdrawWindow:  getEnvInt("RL_WITHDRAW_WINDOW_SECONDS", 60),
			TransferLimit:   getEnvInt("RL_TRANSFER_LIMIT", 10),
			TransferWindow:  getEnvInt("RL_TRANSFER_WINDOW_SECONDS", 60),
			WebSocketLimit:  getEnvInt("RL_WEBSOCKET_LIMIT", 300),
			WebSocketWindow: getEnvInt("RL_WEBSOCKET_WINDOW_SECONDS", 60),
		},
	}

	if err := validateConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// validateConfig fails startup (rather than silently running with a known-weak
// secret) when a security-critical value is missing or left at its placeholder.
func validateConfig(c *Config) error {
	s := c.JWT.SecretKey
	switch {
	case s == "", s == "your-secret-key-change-in-production", s == "change-me":
		return fmt.Errorf("JWT_SECRET is unset or still the placeholder; set a strong random value (openssl rand -hex 32)")
	case len(s) < 32:
		return fmt.Errorf("JWT_SECRET is too short (%d chars); use at least 32 (openssl rand -hex 32)", len(s))
	}
	return nil
}

// GetDSN returns the PostgreSQL connection string
func (c *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

// GetAddr returns the Redis address
func (c *RedisConfig) GetAddr() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func parseWinRate(value string) float64 {
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		if parsed >= 0 && parsed <= 1 {
			return parsed
		}
	}
	return 0.8
}

// parseRedisConfig parses Redis configuration from REDIS_URL or individual environment variables
func parseRedisConfig() RedisConfig {
	redisURL := getEnv("REDIS_URL", "")

	// If REDIS_URL is provided, parse it
	if redisURL != "" {
		parsedURL, err := url.Parse(redisURL)
		if err == nil {
			config := RedisConfig{
				Host:     parsedURL.Hostname(),
				Port:     parsedURL.Port(),
				Password: "",
				DB:       0,
			}

			// Get password from UserInfo
			if parsedURL.User != nil {
				password, ok := parsedURL.User.Password()
				if ok {
					config.Password = password
				}
			}

			// Get database number from path (e.g., /0, /1, etc.)
			if parsedURL.Path != "" {
				dbStr := strings.TrimPrefix(parsedURL.Path, "/")
				if dbNum, err := strconv.Atoi(dbStr); err == nil {
					config.DB = dbNum
				}
			}

			// Default port if not specified
			if config.Port == "" {
				config.Port = "6379"
			}

			return config
		}
	}

	// Fall back to individual environment variables
	return RedisConfig{
		Host:     getEnv("REDIS_HOST", "localhost"),
		Port:     getEnv("REDIS_PORT", "6379"),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       0,
	}
}
