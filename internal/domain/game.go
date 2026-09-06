package domain

import (
	"time"

	"github.com/google/uuid"
)

// GameType represents the type of game.
// There are two tiers: a standard game and a VIP game.
type GameType string

const (
	GameTypeRegular GameType = "REGULAR" // Bet: 10
	GameTypeVIP     GameType = "VIP"     // Bet: 50
)

// GetBetAmount returns the bet amount for a game type.
// Returns 0 for unknown game types (see IsValid).
func (gt GameType) GetBetAmount() float64 {
	switch gt {
	case GameTypeRegular:
		return BetAmountRegular
	case GameTypeVIP:
		return BetAmountVIP
	default:
		return 0
	}
}

// IsValid reports whether the game type is one of the supported tiers.
func (gt GameType) IsValid() bool {
	switch gt {
	case GameTypeRegular, GameTypeVIP:
		return true
	default:
		return false
	}
}

// GameState represents the state of a game
type GameState string

const (
	GameStateWaiting   GameState = "WAITING"
	GameStateCountdown GameState = "COUNTDOWN"
	GameStateDrawing   GameState = "DRAWING"
	GameStateFinished  GameState = "FINISHED"
	GameStateClosed    GameState = "CLOSED"
	GameStateCancelled GameState = "CANCELLED"
)

// BingoLetter represents the column letter in BINGO
type BingoLetter string

const (
	BingoLetterB BingoLetter = "B"
	BingoLetterI BingoLetter = "I"
	BingoLetterN BingoLetter = "N"
	BingoLetterG BingoLetter = "G"
	BingoLetterO BingoLetter = "O"
)

// Game represents a bingo game instance
type Game struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	GameType      GameType   `json:"game_type" db:"game_type"`
	State         GameState  `json:"state" db:"state"`
	BetAmount     float64    `json:"bet_amount" db:"bet_amount"`
	MinPlayers    int        `json:"min_players" db:"min_players"`
	PlayerCount   int        `json:"player_count" db:"player_count"`
	PrizePool     float64    `json:"prize_pool" db:"prize_pool"`
	HouseCut      float64    `json:"house_cut" db:"house_cut"`
	RoundCode     string     `json:"round_code" db:"round_code"`
	WinnerID      *uuid.UUID `json:"winner_id,omitempty" db:"winner_id"`
	CountdownEnds *time.Time `json:"countdown_ends,omitempty" db:"countdown_ends"`
	StartedAt     *time.Time `json:"started_at,omitempty" db:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty" db:"finished_at"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
}

// GamePlayer represents a player in a game
type GamePlayer struct {
	ID     uuid.UUID `json:"id" db:"id"`
	GameID uuid.UUID `json:"game_id" db:"game_id"`
	UserID uuid.UUID `json:"user_id" db:"user_id"`
	CardID int       `json:"card_id" db:"card_id"` // 1-500
	// Paid is false while a card is only reserved during the pre-game window and
	// flips to true when the countdown ends and the stake is actually charged.
	Paid         bool       `json:"paid" db:"paid"`
	IsEliminated bool       `json:"is_eliminated" db:"is_eliminated"`
	IsWinner     bool       `json:"is_winner" db:"is_winner"`
	PrizeWon     float64    `json:"prize_won" db:"prize_won"`
	JoinedAt     time.Time  `json:"joined_at" db:"joined_at"`
	LeftAt       *time.Time `json:"left_at,omitempty" db:"left_at"`
	// PaidFromBonus marks a card bought with play-only bonus. Refund paths must
	// return such a stake as bonus, not as withdrawable cash — otherwise
	// joining a game and leaving would launder bonus into cash.
	PaidFromBonus bool `json:"paid_from_bonus" db:"paid_from_bonus"`
	// BonusExpiresAt is the deadline of the grant this card consumed, so a
	// refund is reinstated under the original deadline rather than a fresh one.
	BonusExpiresAt *time.Time `json:"bonus_expires_at,omitempty" db:"bonus_expires_at"`
	// IsBot is true when the owning user is a house-controlled filler bot.
	IsBot bool `json:"is_bot" db:"is_bot"`
}

// BingoCard represents a 5x5 bingo card
type BingoCard struct {
	ID      int       `json:"id"`      // 1-500
	Numbers [5][5]int `json:"numbers"` // 5x5 grid
}

// DrawnNumber represents a number drawn in the game
type DrawnNumber struct {
	Letter  BingoLetter `json:"letter"`
	Number  int         `json:"number"`
	DrawnAt time.Time   `json:"drawn_at"`
}

// JoinGameRequest represents the request to join a game.
// UserID is populated from the authenticated JWT, not the request body.
type JoinGameRequest struct {
	UserID uuid.UUID `json:"-"`
	CardID int       `json:"card_id" binding:"required,min=1,max=500"` // min=MinCardID, max=MaxCardID (see constants.go)
}

// LeaveGameRequest represents the request to leave a game.
// UserID is populated from the authenticated JWT, not the request body.
// CardID is optional: when > 0 only that one card is dropped (refunded if the
// game hasn't started); when 0 the player leaves entirely (all their cards).
type LeaveGameRequest struct {
	UserID uuid.UUID `json:"-"`
	CardID int       `json:"card_id"`
}

// ClaimBingoRequest represents the request to claim bingo.
// UserID is populated from the authenticated JWT, not the request body.
// CardID identifies which of the player's cards the claim is for.
type ClaimBingoRequest struct {
	UserID        uuid.UUID `json:"-"`
	CardID        int       `json:"card_id" binding:"required,min=1,max=500"` // which card to claim on
	MarkedNumbers []int     `json:"marked_numbers" binding:"required"`        // marked positions (0-24)
}

// GetGamesRequest represents the request to get available games
type GetGamesRequest struct {
	GameType *GameType `form:"type"` // Optional filter by game type
}

// AdminGamePlayer is a player entry enriched with user info for the admin
// game-detail view.
type AdminGamePlayer struct {
	UserID       uuid.UUID `json:"user_id"`
	FirstName    string    `json:"first_name"`
	LastName     *string   `json:"last_name,omitempty"`
	PhoneNumber  string    `json:"phone_number"`
	TelegramID   int64     `json:"telegram_id"`
	CardID       int       `json:"card_id"`
	IsEliminated bool      `json:"is_eliminated"`
	JoinedAt     time.Time `json:"joined_at"`
}

// AdminGameFundingStats counts distinct real players by how their paid cards
// were funded. WalletPlayers and BonusPlayers overlap when a player used both.
type AdminGameFundingStats struct {
	TotalPlayers  int `json:"total_players"`
	WalletPlayers int `json:"wallet_players"`
	BonusPlayers  int `json:"bonus_players"`
	MixedPlayers  int `json:"mixed_players"`
}

// AdminGameDetail is a game plus its active players and — once finished — who
// won and how much, for the admin dashboard.
type AdminGameDetail struct {
	Game    *Game                 `json:"game"`
	Players []*AdminGamePlayer    `json:"players"`
	Winners []*GameWinner         `json:"winners"`
	Funding AdminGameFundingStats `json:"funding"`
}

// CancelGameResult summarizes the outcome of an admin force-cancel.
type CancelGameResult struct {
	Game           *Game   `json:"game"`
	RefundedCount  int     `json:"refunded_count"`
	RefundedAmount float64 `json:"refunded_amount"`
}

// AdminGameFilter holds optional filters for the admin game list.
type AdminGameFilter struct {
	State    *GameState `form:"state"`
	GameType *GameType  `form:"type"`
	Search   string     `form:"search"`
	Active   bool       `form:"active"`
}

// RecentWinner is a public, lightweight record of a finished game's winner,
// for the lobby's recent-winners feed (transparency / trust).
type RecentWinner struct {
	GameID     uuid.UUID `json:"game_id"`
	GameType   GameType  `json:"game_type"`
	WinnerName string    `json:"winner_name"`
	Prize      float64   `json:"prize"`
	FinishedAt time.Time `json:"finished_at"`
}

// HouseCutTier is the house cut earned from one game tier (real-player games).
type HouseCutTier struct {
	Tier     GameType `json:"tier"`
	Games    int      `json:"games"`
	HouseCut float64  `json:"house_cut"`
}

// HouseCutDay is the house cut earned on one day (Ethiopian date).
type HouseCutDay struct {
	Day      string  `json:"day"`
	Games    int     `json:"games"`
	HouseCut float64 `json:"house_cut"`
}

// HouseCutDetail is the drill-down behind the dashboard house-cut figure, so an
// admin can see where revenue comes from (per tier, per day) and the real-player
// P&L — the actual cash exposure — all in one place.
type HouseCutDetail struct {
	TotalHouseCut float64        `json:"total_house_cut"`
	RealPlayerPnl float64        `json:"real_player_pnl"`
	ByTier        []HouseCutTier `json:"by_tier"`
	ByDay         []HouseCutDay  `json:"by_day"`
}

// UserGameStats is a player's lifetime play + money record, so an admin can tell
// at a glance whether a pending withdrawal belongs to someone who actually plays
// and wins — or to a farmed account whose balance came from bonuses/referrals
// they never earned by playing. The money fields make the SOURCE of the balance
// explicit: deposited (real cash in), bonus (play-only they were given), won
// (from playing), staked, withdrawn (real cash already taken out).
type UserGameStats struct {
	GamesPlayed    int     `json:"games_played"`    // distinct games they paid into
	GamesWon       int     `json:"games_won"`       // distinct games they won
	TotalWon       float64 `json:"total_won"`       // sum of prizes won by playing
	TotalStaked    float64 `json:"total_staked"`    // sum of cash bets placed
	TotalDeposited float64 `json:"total_deposited"` // real money deposited (completed)
	TotalWithdrawn float64 `json:"total_withdrawn"` // real money already withdrawn (completed)
	TotalBonus     float64 `json:"total_bonus"`     // play-only bonus/referral ever granted
	RealBalance    float64 `json:"real_balance"`    // current withdrawable balance
	BonusBalance   float64 `json:"bonus_balance"`   // current spendable play-only bonus
	ReferredCount  int     `json:"referred_count"`  // how many players this user invited
}

// GameWinner is one winning card of a finished game, with the prize share it was
// paid and the marks that prove the win. A game may have several (co-winners who
// completed on the same draw and split the pot). Used to render the winning
// card(s) on the post-game screen — including for clients that reconnect after
// the live winner event, which the transient event alone can't reach.
type GameWinner struct {
	UserID        uuid.UUID `json:"user_id"`
	WinnerName    string    `json:"winner_name"`
	CardID        int       `json:"card_id"`
	Prize         float64   `json:"prize"`
	MarkedNumbers []int     `json:"marked_numbers"`
}
