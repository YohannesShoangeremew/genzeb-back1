package domain

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// UserRepository defines the interface for user data operations
type UserRepository interface {
	Create(ctx context.Context, tx *sql.Tx, user *User) error
	FindByTelegramID(ctx context.Context, telegramID int64) (*User, error)
	FindByPhone(ctx context.Context, phone string) (*User, error)
	FindByReferralCode(ctx context.Context, referralCode string) (*User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
	FindReferredBy(ctx context.Context, userID uuid.UUID) ([]*User, error)
	FindAll(ctx context.Context, limit, offset int) ([]*User, error)
	ListAdmin(ctx context.Context, search, role string, limit, offset int) ([]*UserWithWallet, int, error)
	SetAdminCredentialsByTelegramID(ctx context.Context, telegramID int64, hashedPassword string) error
	Update(ctx context.Context, user *User) error
	UpdateRole(ctx context.Context, id uuid.UUID, role string) error
	SetAdminCredentialsByID(ctx context.Context, id uuid.UUID, hashedPassword string) error
	SetBanned(ctx context.Context, id uuid.UUID, banned bool) error
	Delete(ctx context.Context, id uuid.UUID) error
	CountAll(ctx context.Context) (int, error)
}

// WalletRepository defines the interface for wallet data operations
type WalletRepository interface {
	Create(ctx context.Context, tx *sql.Tx, wallet *Wallet) error
	FindByUserID(ctx context.Context, userID uuid.UUID) (*Wallet, error)
	UpdateBalance(ctx context.Context, tx *sql.Tx, userID uuid.UUID, amount float64) error
	LockForUpdate(ctx context.Context, tx *sql.Tx, userID uuid.UUID) (*Wallet, error)
	GetTotalBalance(ctx context.Context) (float64, error)
}

// TransactionRepository defines the interface for transaction data operations
type TransactionRepository interface {
	Create(ctx context.Context, tx *sql.Tx, transaction *Transaction) error
	FindByID(ctx context.Context, id uuid.UUID) (*Transaction, error)
	FindByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Transaction, error)
	FindByUserIDAndType(ctx context.Context, userID uuid.UUID, transactionType TransactionType, limit int) ([]*Transaction, error)
	FindByUserIDAndTypes(ctx context.Context, userID uuid.UUID, transactionTypes []TransactionType, limit int) ([]*Transaction, error)
	FindByStatusAndType(ctx context.Context, status TransactionStatus, transactionType TransactionType, limit, offset int) ([]*Transaction, error)
	FindByStatus(ctx context.Context, status TransactionStatus, limit, offset int) ([]*Transaction, error)
	FindByTypes(ctx context.Context, transactionTypes []TransactionType, limit, offset int) ([]*Transaction, error)
	FindAll(ctx context.Context, limit, offset int) ([]*Transaction, error)
	ListAdmin(ctx context.Context, filter AdminTransactionFilter, search string, limit, offset int) ([]*Transaction, int, error)
	UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status TransactionStatus) error
	CountByStatusAndType(ctx context.Context, status TransactionStatus, transactionType TransactionType) (int, error)
	ExistsActiveDepositByTransactionID(ctx context.Context, transactionID string) (bool, error)
	CountAll(ctx context.Context) (int, error)
	// RealPlayerGamePnL returns (real-player stakes) − (real-player winnings)
	// over all completed game transactions, excluding bots. Positive = the house
	// is ahead; negative = it has paid real players more than they staked (real
	// cash exposure, e.g. real players winning bot-inflated pools).
	RealPlayerGamePnL(ctx context.Context) (float64, error)
	// FindRealPlayerWinnings lists completed 'winnings' transactions belonging to
	// REAL (non-bot) players, newest first, for the admin winners tab.
	FindRealPlayerWinnings(ctx context.Context, limit, offset int) ([]*Transaction, error)
	// CountRealPlayerWinnings is the total for the winners list (for pagination).
	CountRealPlayerWinnings(ctx context.Context) (int, error)
	// CountByUser is the total number of a user's transactions (for pagination of
	// their history on the admin player-detail view).
	CountByUser(ctx context.Context, userID uuid.UUID) (int, error)
	// FindWithdrawalsByStatus lists genuine withdrawal REQUESTS only (category
	// 'withdrawal'), excluding bets which are also type 'withdraw'.
	FindWithdrawalsByStatus(ctx context.Context, status TransactionStatus, limit, offset int) ([]*Transaction, error)
	CountWithdrawalsByStatus(ctx context.Context, status TransactionStatus) (int, error)
}

// GameHistoryEntry represents a game with user's participation details.
// A player may hold several cards in one game; CardsHeld is how many, and
// TotalStake is what they actually spent (CardsHeld × bet_amount). CardID is
// just the representative (most recently joined) card.
type GameHistoryEntry struct {
	Game         *Game      `json:"game"`
	CardID       int        `json:"card_id"`
	CardsHeld    int        `json:"cards_held"`
	TotalStake   float64    `json:"total_stake"`
	IsEliminated bool       `json:"is_eliminated"`
	JoinedAt     time.Time  `json:"joined_at"`
	LeftAt       *time.Time `json:"left_at,omitempty"`
	IsWinner     bool       `json:"is_winner"`
	// WinAmount is the total this user was paid in the game across all their
	// winning cards (0 if they didn't win). With pot splitting this can be less
	// than the full prize pool.
	WinAmount float64 `json:"win_amount"`
}

// GameRepository defines the interface for game data operations
type GameRepository interface {
	Create(ctx context.Context, game *Game) error
	FindByID(ctx context.Context, id uuid.UUID) (*Game, error)
	FindAvailable(ctx context.Context, gameType *GameType, limit int) ([]*Game, error)
	// FindAll returns games filtered by optional state and type, newest first.
	FindAll(ctx context.Context, state *GameState, gameType *GameType, limit, offset int) ([]*Game, error)
	// CountAll counts games matching the optional state and type filters.
	CountAll(ctx context.Context, state *GameState, gameType *GameType) (int, error)
	FindAdmin(ctx context.Context, filter AdminGameFilter, limit, offset int) ([]*Game, error)
	CountAdmin(ctx context.Context, filter AdminGameFilter) (int, error)
	Update(ctx context.Context, game *Game) error
	// LockForUpdate locks a game row FOR UPDATE inside a transaction. Used to
	// serialize force-cancel against winner claims and double-cancels.
	LockForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*Game, error)
	// UpdateTx updates a game row inside an existing transaction.
	UpdateTx(ctx context.Context, tx *sql.Tx, game *Game) error
	// GetActivePlayersTx returns players still in the game (left_at IS NULL),
	// read inside an existing transaction.
	GetActivePlayersTx(ctx context.Context, tx *sql.Tx, gameID uuid.UUID) ([]*GamePlayer, error)
	// ClaimWinner atomically marks the game FINISHED with the winner, but only if
	// it is still DRAWING. Returns true only for the single claim that succeeds.
	ClaimWinner(ctx context.Context, tx *sql.Tx, gameID, winnerID uuid.UUID) (bool, error)
	// MarkCardWinner flags one card (game_id, user_id, card_id) as a winner and
	// records the prize share it was paid. Used when splitting the pot across
	// every card that completed on the same drawn number.
	MarkCardWinner(ctx context.Context, tx *sql.Tx, gameID, userID uuid.UUID, cardID int, prize float64) error
	// FindWinningCards returns every winning card of a game (is_winner = TRUE)
	// with its owner's name and prize share, ordered deterministically (earliest
	// joiner, then card ID). Empty for games with no winner.
	FindWinningCards(ctx context.Context, gameID uuid.UUID) ([]*GameWinner, error)
	// GetUserWinnings returns how much the user has won today (Ethiopian time)
	// and in total, summed across their winning cards. Backs the WIN stat on
	// the card picker.
	GetUserWinnings(ctx context.Context, userID uuid.UUID) (today float64, total float64, err error)
	// GetUserGameStats returns a player's lifetime play record (games played/won,
	// total won/staked) so an admin can verify a withdrawal is from a real winner.
	GetUserGameStats(ctx context.Context, userID uuid.UUID) (*UserGameStats, error)
	AddPlayer(ctx context.Context, tx *sql.Tx, player *GamePlayer) error
	// MarkUserCardsPaidTx flips a user's reserved (unpaid) active cards to paid
	// when the countdown ends and their stake is charged. Returns rows changed.
	MarkUserCardsPaidTx(ctx context.Context, tx *sql.Tx, gameID, userID uuid.UUID) (int64, error)
	// MarkCardsBonusFundedTx flags n of a user's cards as bought with bonus,
	// stamping the consumed grant's expiry so a refund can honour it.
	MarkCardsBonusFundedTx(ctx context.Context, tx *sql.Tx, gameID, userID uuid.UUID, n int, expiresAt time.Time) (int64, error)
	// RemovePlayerCard marks one specific card (game_id, user_id, card_id) as
	// left. A player may hold several cards, so leaving is per-card. It returns
	// the number of rows actually transitioned (0 if the card was already left),
	// so callers only refund a stake that genuinely moved out of the game.
	RemovePlayerCard(ctx context.Context, tx *sql.Tx, gameID, userID uuid.UUID, cardID int) (int64, error)
	// FindPlayer returns any one active card row for the user (used to test
	// membership / reconnect). Prefer FindPlayersByUser when all cards matter.
	FindPlayer(ctx context.Context, gameID, userID uuid.UUID) (*GamePlayer, error)
	// FindPlayersByUser returns all of a user's active card rows in a game.
	FindPlayersByUser(ctx context.Context, gameID, userID uuid.UUID) ([]*GamePlayer, error)
	// FindPlayerCard returns the user's row for one specific card, if active.
	FindPlayerCard(ctx context.Context, gameID, userID uuid.UUID, cardID int) (*GamePlayer, error)
	// CountActiveCardsForUser counts a user's active cards in a game (cap check).
	CountActiveCardsForUser(ctx context.Context, gameID, userID uuid.UUID) (int, error)
	// FindOtherActiveGameForUserTx returns another live game in which the user
	// still has an active card. It is called after locking the user's wallet, so
	// concurrent joins to different tiers cannot both create reservations.
	FindOtherActiveGameForUserTx(ctx context.Context, tx *sql.Tx, userID, gameID uuid.UUID) (*Game, error)
	// CountDistinctPlayers counts distinct active users in a game (start rule).
	CountDistinctPlayers(ctx context.Context, gameID uuid.UUID) (int, error)
	GetPlayers(ctx context.Context, gameID uuid.UUID) ([]*GamePlayer, error)
	// EliminatePlayerCard eliminates one specific card after a wrong claim; the
	// player's other cards keep playing.
	EliminatePlayerCard(ctx context.Context, tx *sql.Tx, gameID, userID uuid.UUID, cardID int) error
	GetTakenCards(ctx context.Context, gameID uuid.UUID) ([]int, error)
	SaveDrawnNumber(ctx context.Context, gameID uuid.UUID, letter string, number int) error
	FindGamesByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*GameHistoryEntry, error)
	CountGamesByUserID(ctx context.Context, userID uuid.UUID) (int, error)
	// FindActiveGameByUserID returns the live game (WAITING/COUNTDOWN/DRAWING)
	// the user still holds cards in, or nil if none. Powers the "return to live
	// game" reconnect after the player navigates away mid-game.
	FindActiveGameByUserID(ctx context.Context, userID uuid.UUID) (*GameHistoryEntry, error)
	CountGamesByType(ctx context.Context) (map[GameType]int, error)
	GetTotalHouseCut(ctx context.Context) (float64, error)
	// HouseCutByTier / HouseCutByDay break the house cut down for the dashboard
	// drill-down (real-player games only, matching GetTotalHouseCut).
	HouseCutByTier(ctx context.Context) ([]HouseCutTier, error)
	HouseCutByDay(ctx context.Context, days int) ([]HouseCutDay, error)
	// FindRecentWinners returns the most recently finished games that had a
	// winner, with the winner's display name and prize, for the lobby feed.
	FindRecentWinners(ctx context.Context, limit int) ([]*RecentWinner, error)
	// CancelEmptyStaleGames cancels every WAITING/COUNTDOWN game that has no
	// active players and hasn't been touched since `olderThan`. Empty games
	// carry no stakes, so this is safe (no refunds) and just clears the lobby of
	// abandoned/auto-spawned games. Returns how many were cancelled.
	CancelEmptyStaleGames(ctx context.Context, olderThan time.Time) (int64, error)
	// TouchUpdatedAt bumps a game's updated_at so a just-served lobby game is
	// protected from the empty-game sweeper during the join window.
	TouchUpdatedAt(ctx context.Context, id uuid.UUID) error
}
