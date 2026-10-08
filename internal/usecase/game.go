package usecase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/bingo/backend/pkg/bingo"
	redisGame "github.com/bingo/backend/pkg/redis"
	utils "github.com/bingo/backend/pkg/utils"
	"github.com/google/uuid"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type GameUseCase struct {
	gameRepo        domain.GameRepository
	walletRepo      domain.WalletRepository
	transactionRepo domain.TransactionRepository
	userRepo        domain.UserRepository
	bonusRepo       domain.BonusRepository
	botRepo         domain.BotRepository
	adminEventLogs  domain.AdminEventLogRepository
	db              *sql.DB
	redisService    *redisGame.GameStateService
}

// NewGameUseCase creates a new game use case
func NewGameUseCase(
	gameRepo domain.GameRepository,
	walletRepo domain.WalletRepository,
	transactionRepo domain.TransactionRepository,
	userRepo domain.UserRepository,
	bonusRepo domain.BonusRepository,
	botRepo domain.BotRepository,
	adminEventLogs domain.AdminEventLogRepository,
	db *sql.DB,
	redisService *redisGame.GameStateService,
) *GameUseCase {
	return &GameUseCase{
		gameRepo:        gameRepo,
		walletRepo:      walletRepo,
		transactionRepo: transactionRepo,
		userRepo:        userRepo,
		bonusRepo:       bonusRepo,
		botRepo:         botRepo,
		adminEventLogs:  adminEventLogs,
		db:              db,
		redisService:    redisService,
	}
}

// refundCardStake returns one card's stake to wherever it came from.
//
// A card bought with play-only bonus MUST come back as bonus. Crediting it to
// the cash balance would convert bonus into withdrawable money simply by
// joining a game and leaving again — the one leak the free-bet model exists to
// prevent, and the reason game_players carries paid_from_bonus at all.
//
// The bonus is reinstated under its ORIGINAL deadline, so the round trip
// cannot be used to keep an expiring bonus alive forever. A deadline already
// past is dropped by the repository rather than resurrected.
func (uc *GameUseCase) refundCardStake(ctx context.Context, tx *sql.Tx, p *domain.GamePlayer, amount float64, ref *string) error {
	if p.PaidFromBonus && uc.bonusRepo != nil {
		// A bonus-funded card always carries its deadline (enforced by a CHECK
		// constraint). The fallback exists only so a legacy row cannot silently
		// take the cash branch, which is the branch that leaks.
		expiry := time.Now().Add(24 * time.Hour)
		if p.BonusExpiresAt != nil {
			expiry = *p.BonusExpiresAt
		}
		return uc.bonusRepo.Restore(ctx, tx, p.UserID, amount, expiry, "game refund")
	}

	if _, err := uc.walletRepo.LockForUpdate(ctx, tx, p.UserID); err != nil {
		return fmt.Errorf("failed to lock wallet for refund: %w", err)
	}
	if err := uc.walletRepo.UpdateBalance(ctx, tx, p.UserID, amount); err != nil {
		return fmt.Errorf("failed to refund stake: %w", err)
	}
	refundTx := &domain.Transaction{
		UserID:    p.UserID,
		Type:      domain.TransactionTypeDeposit, // Refund is treated as a deposit
		Category:  domain.TransactionCategoryRefund,
		Amount:    amount,
		Status:    domain.TransactionStatusCompleted,
		Reference: ref, // GAME_REFUND — excluded from cash history
	}
	if err := uc.transactionRepo.Create(ctx, tx, refundTx); err != nil {
		return fmt.Errorf("failed to record refund: %w", err)
	}
	return nil
}

// CleanupEmptyGames cancels WAITING/COUNTDOWN games that have no active players
// and have sat untouched past the grace period, so abandoned and never-joined
// lobby games stop cluttering the active list. Empty games carry no stakes, so
// nothing is refunded. Returns how many were cancelled. Meant to be called on a
// timer by a background sweeper.
func (uc *GameUseCase) CleanupEmptyGames(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-domain.EmptyGameGracePeriod)
	return uc.gameRepo.CancelEmptyStaleGames(ctx, cutoff)
}

// GetAvailableGames gets available games (WAITING or COUNTDOWN state)
func (uc *GameUseCase) GetAvailableGames(ctx context.Context, gameType *domain.GameType) ([]*domain.Game, error) {
	games, err := uc.gameRepo.FindAvailable(ctx, gameType, domain.MaxAvailableGamesLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to find available games: %w", err)
	}

	return games, nil
}

// RecordLobbyActivity marks the given tier(s) as recently browsed by a real
// player, so the filler bots keep running games there (see BotUseCase.sweep and
// domain.LobbyActivityWindow). Called from the lobby fetch handler. When
// gameType is nil (a combined lobby view), both offered tiers are marked. Only
// real players reach this path — bots never call the HTTP handlers — so it is a
// clean "a human is here" signal. Best-effort: Redis errors are swallowed.
func (uc *GameUseCase) RecordLobbyActivity(ctx context.Context, gameType *domain.GameType) {
	if uc.redisService == nil {
		return
	}
	tiers := []domain.GameType{domain.GameTypeRegular, domain.GameTypeVIP}
	if gameType != nil {
		tiers = []domain.GameType{*gameType}
	}
	for _, t := range tiers {
		_ = uc.redisService.MarkTierBrowsed(ctx, string(t), domain.LobbyActivityWindow)
	}
}

// GetInProgressGame returns the game of this type that is currently being drawn
// (DRAWING), or nil if none. Used so the lobby can route a late / non-playing
// user to spectate the live round instead of starting a second game — one game
// per table at a time.
func (uc *GameUseCase) GetInProgressGame(ctx context.Context, gameType domain.GameType) (*domain.Game, error) {
	state := domain.GameStateDrawing
	games, err := uc.gameRepo.FindAll(ctx, &state, &gameType, 1, 0)
	if err != nil {
		return nil, err
	}
	if len(games) == 0 {
		return nil, nil
	}
	return games[0], nil
}

// CreateOrGetGame creates a new game or returns an existing available game
func (uc *GameUseCase) CreateOrGetGame(ctx context.Context, gameType domain.GameType) (*domain.Game, error) {
	// Reject unsupported tiers (only REGULAR and VIP are offered)
	if !gameType.IsValid() {
		return nil, fmt.Errorf("invalid game type %q: must be REGULAR or VIP", gameType)
	}

	// Try to find an available game first
	games, err := uc.gameRepo.FindAvailable(ctx, &gameType, 1)
	if err == nil && len(games) > 0 {
		// Reuse the existing game. Bump its updated_at so the empty-game sweeper
		// can't cancel it out from under a player who is about to join.
		_ = uc.gameRepo.TouchUpdatedAt(ctx, games[0].ID)
		return games[0], nil
	}

	// Create new game
	game := &domain.Game{
		ID:          uuid.New(),
		GameType:    gameType,
		State:       domain.GameStateWaiting,
		BetAmount:   gameType.GetBetAmount(),
		MinPlayers:  domain.MinPlayers,
		PlayerCount: 0,
		PrizePool:   0,
		HouseCut:    domain.HouseCut,
	}

	if err := uc.gameRepo.Create(ctx, game); err != nil {
		return nil, fmt.Errorf("failed to create game: %w", err)
	}

	// Save to Redis (if available)
	if uc.redisService != nil {
		if err := uc.redisService.SaveGameState(ctx, game); err != nil {
			// Log error but don't fail
			fmt.Printf("Warning: failed to save game state to Redis: %v\n", err)
		}
	}

	return game, nil
}

// JoinGame allows a user to join a game
func (uc *GameUseCase) JoinGame(ctx context.Context, gameID uuid.UUID, req domain.JoinGameRequest) (*domain.GamePlayer, error) {
	// Validate card ID
	if req.CardID < domain.MinCardID || req.CardID > domain.MaxCardID {
		return nil, fmt.Errorf("card ID must be between %d and %d", domain.MinCardID, domain.MaxCardID)
	}

	// Get game
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("game not found: %w", err)
	}

	// Idempotent reconnect for THIS card: if the user already holds this exact
	// card, return that row instead of charging again. This lets a player who
	// dropped off (closed the app / lost their WebSocket) tap an already-owned
	// card and get back in, even after drawing has started. A *different* card
	// is a new purchase and falls through to the checks below.
	if existingCard, _ := uc.gameRepo.FindPlayerCard(ctx, gameID, req.UserID, req.CardID); existingCard != nil {
		return existingCard, nil
	}

	// A new card can only be bought while the game hasn't started yet.
	if game.State != domain.GameStateWaiting && game.State != domain.GameStateCountdown {
		return nil, errors.New("game is not accepting new players")
	}

	// Reject if this card is already held by someone (active). The DB also has a
	// UNIQUE(game_id, card_id) constraint as a hard safety net against the rare
	// check-then-insert race.
	takenCards, err := uc.gameRepo.GetTakenCards(ctx, gameID)
	if err == nil {
		for _, taken := range takenCards {
			if taken == req.CardID {
				return nil, errors.New("card is already taken")
			}
		}
	}

	// Start transaction
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Serialize concurrent joins to THIS game by locking its row before touching
	// the live counters. Without it, two players joining at once both read the
	// same player_count/prize_pool and one update is lost — the pool ends up
	// undercounted (winner underpaid) and the count can stay below MinPlayers so
	// the game never starts. Lock order is game-then-wallet everywhere.
	game, err = uc.gameRepo.LockForUpdate(ctx, tx, gameID)
	if err != nil {
		return nil, fmt.Errorf("game not found: %w", err)
	}

	// Re-check under the lock: the game may have started since the pre-lock read,
	// and a concurrent join may have just taken this card.
	if game.State != domain.GameStateWaiting && game.State != domain.GameStateCountdown {
		return nil, errors.New("game is not accepting new players")
	}
	if takenCards, terr := uc.gameRepo.GetTakenCards(ctx, gameID); terr == nil {
		for _, taken := range takenCards {
			if taken == req.CardID {
				return nil, errors.New("card is already taken")
			}
		}
	}

	// Lock wallet
	wallet, err := uc.walletRepo.LockForUpdate(ctx, tx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("wallet not found: %w", err)
	}

	// A player may reserve multiple cards in one game, but must not participate
	// in two live games (for example REGULAR and VIP) at the same time. This is
	// checked only after the per-user wallet lock: concurrent cross-tier joins
	// then serialize, and the second transaction sees the first reservation.
	otherGame, err := uc.gameRepo.FindOtherActiveGameForUserTx(ctx, tx, req.UserID, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to check active games: %w", err)
	}
	if otherGame != nil {
		return nil, fmt.Errorf("leave your active %s game before joining %s", otherGame.GameType, game.GameType)
	}

	// Enforce the per-player card cap. Read after locking the wallet so a user's
	// concurrent joins (which serialize on that lock) can't both slip past it.
	// count == 0 means this is the player's first card in the game.
	existingCardCount, err := uc.gameRepo.CountActiveCardsForUser(ctx, gameID, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to count player cards: %w", err)
	}
	if existingCardCount >= domain.MaxCardsPerPlayer {
		return nil, fmt.Errorf("maximum %d cards per game", domain.MaxCardsPerPlayer)
	}

	// Reservation model: the stake is NOT charged now. Picking a card only
	// reserves it during the pre-game window; every reserved card is charged
	// together when the countdown ends and the game actually starts (see
	// commitReservations in startDrawing). We still verify the player can cover
	// ALL of their reserved cards, so nobody reserves cards they can't pay for
	// and blocks them from others. existingCardCount excludes the card being
	// added, so the +1 accounts for it.
	//
	// The check must count play-only bonus as well as cash, because that is
	// what the charge itself does: bonus pays for whole cards first and cash
	// covers the rest. Testing cash alone refused a player holding 8 birr and a
	// 50 birr bonus a 10 birr card — locked out of exactly what the bonus was
	// granted for, with a "top up your wallet" message while their bonus sat
	// unused.
	spendable := wallet.Balance
	if uc.bonusRepo != nil {
		bonusAvailable, berr := uc.bonusRepo.SpendableForUpdate(ctx, tx, req.UserID)
		if berr != nil {
			return nil, fmt.Errorf("failed to read bonus balance: %w", berr)
		}
		spendable += bonusAvailable
	}
	if spendable < float64(existingCardCount+1)*game.BetAmount {
		return nil, errors.New("insufficient balance")
	}

	// Add the card as a pending (unpaid) reservation.
	player := &domain.GamePlayer{
		ID:           uuid.New(),
		GameID:       gameID,
		UserID:       req.UserID,
		CardID:       req.CardID,
		Paid:         false,
		IsEliminated: false,
	}

	if err := uc.gameRepo.AddPlayer(ctx, tx, player); err != nil {
		return nil, fmt.Errorf("failed to add player: %w", err)
	}

	// Recompute the live counters from the authoritative set of active cards in
	// this game (visible within this locked transaction, including the card just
	// added) rather than incrementing in memory — so concurrent joins can't lose
	// an update. player_count is DISTINCT people (the start rule needs 2 real
	// players); the prize pool is (cards × stake), excluding the house cut.
	active, err := uc.gameRepo.GetActivePlayersTx(ctx, tx, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to read players: %w", err)
	}
	distinct := make(map[uuid.UUID]bool, len(active))
	for _, p := range active {
		distinct[p.UserID] = true
	}
	prevCount := game.PlayerCount
	game.PlayerCount = len(distinct)
	game.PrizePool = float64(len(active)) * game.BetAmount * (1 - game.HouseCut)

	if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
		return nil, fmt.Errorf("failed to update game: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Update Redis
	uc.redisService.AddPlayer(ctx, gameID, req.UserID)
	uc.redisService.AddTakenCard(ctx, gameID, req.CardID)
	uc.redisService.SaveGameState(ctx, game)

	// Start the countdown on the join that brings the game up to the minimum
	// distinct players (the WAITING → ready transition), and only from WAITING so
	// extra cards from existing players don't re-trigger it.
	if game.State == domain.GameStateWaiting && prevCount < domain.MinPlayers && game.PlayerCount >= domain.MinPlayers {
		utils.GoSafe("startCountdown", func() { uc.startCountdown(context.Background(), gameID) })
	}

	// Publish event. Carry the live prize_pool and player_count so already-
	// connected clients update — otherwise their pool stays frozen at whatever
	// it was when they first received INITIAL_STATE.
	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventPlayerJoined, map[string]interface{}{
		"user_id":      req.UserID.String(),
		"card_id":      req.CardID,
		"prize_pool":   game.PrizePool,
		"player_count": game.PlayerCount,
	})

	return player, nil
}

// LeaveGame allows a user to leave a game
func (uc *GameUseCase) LeaveGame(ctx context.Context, gameID uuid.UUID, req domain.LeaveGameRequest) error {
	// Get game
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		return fmt.Errorf("game not found: %w", err)
	}

	// Figure out which of the user's cards to drop. req.CardID > 0 drops just
	// that one card; req.CardID == 0 means leave the game entirely (all cards).
	userCards, err := uc.gameRepo.FindPlayersByUser(ctx, gameID, req.UserID)
	if err != nil || len(userCards) == 0 {
		return errors.New("user is not in this game")
	}

	var toDrop []*domain.GamePlayer
	if req.CardID > 0 {
		for _, p := range userCards {
			if p.CardID == req.CardID {
				toDrop = append(toDrop, p)
				break
			}
		}
		if len(toDrop) == 0 {
			return errors.New("you do not hold that card")
		}
	} else {
		toDrop = userCards
	}

	// Start transaction
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock the game row to read the latest state and serialize against concurrent
	// joins/leaves that also recompute the live counters (same lock as JoinGame).
	game, err = uc.gameRepo.LockForUpdate(ctx, tx, gameID)
	if err != nil {
		return fmt.Errorf("game not found: %w", err)
	}

	// Check if game is already finished/cancelled/closed
	if game.State == domain.GameStateFinished || game.State == domain.GameStateClosed || game.State == domain.GameStateCancelled {
		return errors.New("game is no longer active")
	}

	// Once the draw has started, stakes are committed: leaving is refused. A
	// mid-draw leave gives no refund AND would recompute (shrink) the prize pool
	// for the remaining players, so it's pure downside / a griefing vector.
	// Cards play automatically to the end — a player who navigates away stays in.
	if game.State == domain.GameStateDrawing {
		return errors.New("cannot leave after the game has started")
	}

	// Refund only cards that were actually charged. Reserved (unpaid) cards moved
	// no money, so leaving before the countdown-end charge just releases them.
	// (A paid card can still exist in a WAITING game if it reverted after a
	// reserver failed to pay at commit — that one refunds.)
	anyPaid := false
	for _, p := range toDrop {
		if p.Paid {
			anyPaid = true
			break
		}
	}
	if anyPaid {
		// Lock the wallet once for all of this user's refunds.
		if _, err := uc.walletRepo.LockForUpdate(ctx, tx, req.UserID); err != nil {
			return fmt.Errorf("wallet not found: %w", err)
		}
	}

	gameRefundRef := "GAME_REFUND"
	for _, p := range toDrop {
		// Drop this card. toDrop was read before the game-row lock, so a
		// concurrent leave may already have dropped it — RemovePlayerCard then
		// transitions 0 rows and we must NOT refund again (that was the N-refund
		// money-drain bug).
		rows, err := uc.gameRepo.RemovePlayerCard(ctx, tx, gameID, req.UserID, p.CardID)
		if err != nil {
			return fmt.Errorf("failed to remove card: %w", err)
		}
		if rows == 0 {
			continue
		}

		// Refund this card's stake only if it was actually charged, and back to
		// whichever purse paid for it (bonus stays bonus).
		if p.Paid {
			if err := uc.refundCardStake(ctx, tx, p, game.BetAmount, &gameRefundRef); err != nil {
				return err
			}
		}
	}

	// Recompute the live counters from the authoritative remaining active cards
	// (this tx's removals are already visible) instead of decrementing in memory,
	// so a concurrent join/leave can't lose an update. player_count is DISTINCT
	// people; the pool is (remaining cards × stake), excluding the house cut.
	remaining, err := uc.gameRepo.GetActivePlayersTx(ctx, tx, gameID)
	if err != nil {
		return fmt.Errorf("failed to read players: %w", err)
	}
	distinct := make(map[uuid.UUID]bool, len(remaining))
	for _, p := range remaining {
		distinct[p.UserID] = true
	}
	leftEntirely := !distinct[req.UserID]
	game.PlayerCount = len(distinct)
	game.PrizePool = float64(len(remaining)) * game.BetAmount * (1 - game.HouseCut)

	// If distinct players drop below minimum during countdown, revert to WAITING.
	// Remaining players stay; the countdown restarts when a 2nd player returns.
	didRevert := false
	if game.State == domain.GameStateCountdown && game.PlayerCount < domain.MinPlayers {
		game.State = domain.GameStateWaiting
		game.CountdownEnds = nil
		didRevert = true
	}

	if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
		return fmt.Errorf("failed to update game: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Update Redis - only drop the player from the live set if they fully left.
	if leftEntirely {
		uc.redisService.RemovePlayer(ctx, gameID, req.UserID)
	}

	// Free each dropped card if no remaining active player holds it.
	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
	if err == nil {
		held := make(map[int]bool, len(players))
		for _, p := range players {
			held[p.CardID] = true
		}
		for _, p := range toDrop {
			if !held[p.CardID] {
				uc.redisService.RemoveTakenCard(ctx, gameID, p.CardID)
			}
		}
	}

	// If the countdown was reverted to WAITING, clear countdown state and notify.
	if didRevert {
		uc.redisService.ClearCountdown(ctx, gameID)
		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
			"status": string(domain.GameStateWaiting),
		})
	}

	uc.redisService.SaveGameState(ctx, game)

	// Publish a PLAYER_LEFT per dropped card so clients can free the card in the
	// UI. Carry the live prize_pool and player_count so every client updates.
	for _, p := range toDrop {
		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventPlayerLeft, map[string]interface{}{
			"user_id":      req.UserID.String(),
			"card_id":      p.CardID,
			"prize_pool":   game.PrizePool,
			"player_count": game.PlayerCount,
		})
	}

	return nil
}

// startCountdown starts the countdown for a game
func (uc *GameUseCase) startCountdown(ctx context.Context, gameID uuid.UUID) {
	// Transition WAITING → COUNTDOWN under the game-row lock and re-read, so this
	// only changes state/timing and never clobbers the player_count/prize_pool
	// that concurrent joins are still recomputing (a stale full-row write here
	// was reverting the live pool back to its 2-player value).
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	game, err := uc.gameRepo.LockForUpdate(ctx, tx, gameID)
	if err != nil || game.State != domain.GameStateWaiting {
		tx.Rollback()
		return
	}

	countdownEnds := time.Now().Add(domain.CountdownDuration)
	game.State = domain.GameStateCountdown
	game.CountdownEnds = &countdownEnds

	if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
		tx.Rollback()
		return
	}
	if err := tx.Commit(); err != nil {
		return
	}

	uc.redisService.SetCountdown(ctx, gameID, countdownEnds)
	uc.redisService.SaveGameState(ctx, game)

	// Publish countdown start. Include prize_pool/player_count so every client
	// (including the player who joined first) shows the final live pool.
	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
		"status":       string(domain.GameStateCountdown),
		"secondsLeft":  int(domain.CountdownDuration.Seconds()),
		"prize_pool":   game.PrizePool,
		"player_count": game.PlayerCount,
	})

	// Run the visible countdown, then charge reservations and start drawing.
	uc.runCountdownThenDraw(ctx, gameID, int(domain.CountdownDuration.Seconds()))
}

// runCountdownThenDraw ticks the countdown down from secondsLeft to 0 (publishing
// each second), then starts the draw. Shared by the initial startCountdown and by
// resumeCountdown after a restart, which passes only the seconds ACTUALLY left on
// the clock so a redeploy mid-countdown doesn't restart the full 40s.
func (uc *GameUseCase) runCountdownThenDraw(ctx context.Context, gameID uuid.UUID, secondsLeft int) {
	ticker := time.NewTicker(domain.CountdownTickerInterval)
	defer ticker.Stop()

	for i := secondsLeft; i > 0; i-- {
		<-ticker.C

		// Stop if the game left COUNTDOWN (e.g. reverted to WAITING as players left).
		game, err := uc.gameRepo.FindByID(ctx, gameID)
		if err != nil || game.State != domain.GameStateCountdown {
			return
		}
		if game.PlayerCount < domain.MinPlayers {
			return
		}

		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventCountdown, map[string]interface{}{
			"secondsLeft": i - 1,
		})
	}

	uc.startDrawing(ctx, gameID)
}

// startDrawing charges all reservations, then transitions COUNTDOWN → DRAWING.
//
// The whole commit runs under the game-row lock: concurrent joins/leaves block
// until we finish and then see DRAWING (join rejected) or the recomputed
// counters, so no reservation slips in mid-charge and no counter update is lost.
func (uc *GameUseCase) startDrawing(ctx context.Context, gameID uuid.UUID) {
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback() // no-op after a successful Commit below

	game, err := uc.gameRepo.LockForUpdate(ctx, tx, gameID)
	if err != nil || game.State != domain.GameStateCountdown {
		return
	}

	active, err := uc.gameRepo.GetActivePlayersTx(ctx, tx, gameID)
	if err != nil {
		return
	}

	// Charge each player for their reserved (unpaid) cards, all at once. A player
	// who can no longer cover their reservations has those cards dropped (they
	// were never charged, so there is nothing to refund).
	unpaidByUser := map[uuid.UUID]int{}
	for _, p := range active {
		if !p.Paid {
			unpaidByUser[p.UserID]++
		}
	}

	gameBetRef := "GAME_BET"
	var droppedCards []int
	for userID, count := range unpaidByUser {
		wallet, err := uc.walletRepo.LockForUpdate(ctx, tx, userID)
		if err != nil {
			return
		}
		// Play-only bonus pays first, in whole cards. Deciding the split BEFORE
		// spending anything matters: consuming bonus and only then finding the
		// player short of cash for the remainder would strand the bonus on a
		// card they never got. The grant rows are locked by this read, so the
		// figure cannot move between here and the consume below.
		bonusCards := 0
		var bonusExpiry *time.Time
		if uc.bonusRepo != nil {
			available, berr := uc.bonusRepo.SpendableForUpdate(ctx, tx, userID)
			if berr != nil {
				return
			}
			bonusCards = int(available / game.BetAmount)
			if bonusCards > count {
				bonusCards = count
			}
		}

		cashCards := count - bonusCards
		cashCost := float64(cashCards) * game.BetAmount
		if wallet.Balance < cashCost {
			// Can't pay: release all of this player's reserved cards. No bonus
			// has been consumed yet, so there is nothing to unwind.
			for _, p := range active {
				if p.UserID == userID && !p.Paid {
					if _, err := uc.gameRepo.RemovePlayerCard(ctx, tx, gameID, userID, p.CardID); err != nil {
						return
					}
					droppedCards = append(droppedCards, p.CardID)
				}
			}
			continue
		}

		if bonusCards > 0 {
			paid, expiry, berr := uc.bonusRepo.ConsumeForStake(ctx, tx, userID, game.BetAmount, bonusCards)
			if berr != nil {
				return
			}
			// Defensive: the locked read above should make these agree. If they
			// ever diverge, charge the difference as cash rather than letting
			// the player hold cards nobody paid for.
			bonusCards = paid
			bonusExpiry = expiry
			cashCards = count - bonusCards
			cashCost = float64(cashCards) * game.BetAmount
			if wallet.Balance < cashCost {
				return
			}
		}

		// Deduct the cash portion and record one bet transaction per card.
		if cashCost > 0 {
			if err := uc.walletRepo.UpdateBalance(ctx, tx, userID, -cashCost); err != nil {
				return
			}
		}
		for i := 0; i < count; i++ {
			// Bonus-funded stakes carry their own category so promotional
			// turnover can be told apart from cash turnover in reporting. The
			// GAME_ reference is unchanged, so both stay out of the player's
			// cash history exactly as before.
			category := domain.TransactionCategoryBet
			if i < bonusCards {
				category = domain.TransactionCategoryBonusStake
			}
			betTx := &domain.Transaction{
				UserID:    userID,
				Type:      domain.TransactionTypeWithdraw,
				Category:  category,
				Amount:    game.BetAmount,
				Status:    domain.TransactionStatusCompleted,
				Reference: &gameBetRef,
			}
			if err := uc.transactionRepo.Create(ctx, tx, betTx); err != nil {
				return
			}
		}
		if _, err := uc.gameRepo.MarkUserCardsPaidTx(ctx, tx, gameID, userID); err != nil {
			return
		}
		// Record which cards the bonus bought, and under whose deadline, so a
		// refund returns them as bonus rather than as withdrawable cash.
		if bonusCards > 0 && bonusExpiry != nil {
			if _, err := uc.gameRepo.MarkCardsBonusFundedTx(ctx, tx, gameID, userID, bonusCards, *bonusExpiry); err != nil {
				return
			}
		}
	}

	// Recompute the live counters from the paid cards that remain.
	paidPlayers, err := uc.gameRepo.GetActivePlayersTx(ctx, tx, gameID)
	if err != nil {
		return
	}
	distinct := make(map[uuid.UUID]bool, len(paidPlayers))
	for _, p := range paidPlayers {
		distinct[p.UserID] = true
	}
	game.PlayerCount = len(distinct)
	game.PrizePool = float64(len(paidPlayers)) * game.BetAmount * (1 - game.HouseCut)

	// Not enough paying players (e.g. a reserver couldn't cover their cards at
	// commit): don't start. Revert to WAITING and keep whoever did pay; the
	// countdown restarts when another player joins. Their paid cards stay charged
	// and play next round — same as a countdown that drops below the minimum.
	if len(distinct) < domain.MinPlayers {
		game.State = domain.GameStateWaiting
		game.CountdownEnds = nil
		if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
			return
		}
		if err := tx.Commit(); err != nil {
			return
		}
		uc.redisService.ClearCountdown(ctx, gameID)
		for _, cardID := range droppedCards {
			uc.redisService.RemoveTakenCard(ctx, gameID, cardID)
		}
		uc.redisService.SaveGameState(ctx, game)
		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
			"status":       string(domain.GameStateWaiting),
			"prize_pool":   game.PrizePool,
			"player_count": game.PlayerCount,
		})
		return
	}

	game.State = domain.GameStateDrawing
	now := time.Now()
	game.StartedAt = &now
	if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
		return
	}
	if err := tx.Commit(); err != nil {
		return
	}

	for _, cardID := range droppedCards {
		uc.redisService.RemoveTakenCard(ctx, gameID, cardID)
	}
	uc.redisService.SaveGameState(ctx, game)
	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
		"status":       string(domain.GameStateDrawing),
		"prize_pool":   game.PrizePool,
		"player_count": game.PlayerCount,
	})

	// Start drawing numbers periodically
	utils.GoSafe("drawNumbers", func() { uc.drawNumbers(ctx, gameID) })
}

// acquireDrawLease claims the game's draw lease, retrying long enough to outlast
// a departing instance's lease during a deploy overlap so the survivor takes over
// as soon as the old process is gone. Returns false if the game stops being live
// while we wait, or we never get the lease. In normal play there is no contention,
// so the first attempt succeeds instantly.
func (uc *GameUseCase) acquireDrawLease(ctx context.Context, gameID uuid.UUID, token string) bool {
	deadline := time.Now().Add(domain.DrawLeaseTTL + 10*time.Second)
	for {
		if ok, _ := uc.redisService.AcquireOrRenewDrawLease(ctx, gameID, token, domain.DrawLeaseTTL); ok {
			return true
		}
		// Give up if the game is no longer live (cancelled/finished while we waited).
		if g, err := uc.gameRepo.FindByID(ctx, gameID); err == nil &&
			g.State != domain.GameStateDrawing && g.State != domain.GameStateCountdown {
			return false
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Second)
	}
}

// ResumeActiveGames re-attaches the countdown/draw loops to games that were mid-
// flight when this process started — i.e. games a previous process was running
// when it was replaced by a deploy or died. Without it, a redeploy leaves any
// live game frozen in COUNTDOWN/DRAWING forever with players' stakes locked. Safe
// to run alongside a still-draining old instance: the draw lease guarantees a
// single drawer, and startDrawing's row lock guarantees a single transition.
func (uc *GameUseCase) ResumeActiveGames(ctx context.Context) {
	drawing := domain.GameStateDrawing
	if games, err := uc.gameRepo.FindAll(ctx, &drawing, nil, domain.MaxAvailableGamesLimit, 0); err == nil {
		for _, g := range games {
			id := g.ID
			utils.GoSafe("resumeDraw", func() { uc.drawNumbers(context.Background(), id) })
		}
	}
	countdown := domain.GameStateCountdown
	if games, err := uc.gameRepo.FindAll(ctx, &countdown, nil, domain.MaxAvailableGamesLimit, 0); err == nil {
		for _, g := range games {
			id := g.ID
			utils.GoSafe("resumeCountdown", func() { uc.resumeCountdown(context.Background(), id) })
		}
	}
	// A WAITING game already at the minimum has no countdown running (the goroutine
	// that would have started it died with the old process). Kick it off — start-
	// Countdown's row lock makes this a no-op if one is somehow already underway.
	waiting := domain.GameStateWaiting
	if games, err := uc.gameRepo.FindAll(ctx, &waiting, nil, domain.MaxAvailableGamesLimit, 0); err == nil {
		for _, g := range games {
			if g.PlayerCount < domain.MinPlayers {
				continue
			}
			id := g.ID
			utils.GoSafe("resumeStartCountdown", func() { uc.startCountdown(context.Background(), id) })
		}
	}
}

// resumeCountdown continues a game left in COUNTDOWN, honouring the ORIGINAL end
// time (from the Redis countdown, stored as a timezone-safe epoch) rather than
// restarting a fresh 40s. If the clock has already run out, it goes straight to
// drawing.
func (uc *GameUseCase) resumeCountdown(ctx context.Context, gameID uuid.UUID) {
	end, err := uc.redisService.GetCountdown(ctx, gameID)
	if err != nil {
		// No countdown record survived — the clock is effectively up.
		uc.startDrawing(ctx, gameID)
		return
	}
	remaining := int(time.Until(end).Seconds())
	if remaining <= 0 {
		uc.startDrawing(ctx, gameID)
		return
	}
	uc.runCountdownThenDraw(ctx, gameID, remaining)
}

// drawNumbers draws numbers periodically.
//
// Only the process that holds this game's Redis lease draws it, so during a
// rolling deploy (old + new instance overlap) the game is never double-drawn:
// the new instance blocks in acquireDrawLease until the old one's lease expires,
// then picks up exactly where it left off (drawn numbers live in Redis). If we
// ever lose the lease mid-draw, we stop immediately so the new owner is alone.

func (uc *GameUseCase) totalActivePlayers(ctx context.Context, gameID uuid.UUID) int {
	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
	if err != nil {
		return 0
	}
	count := 0
	for _, p := range players {
		if !p.IsEliminated {
			count++
		}
	}
	return count
}

func biasedDrawTarget(totalPlayers int) int {
	return 12 + rand.Intn(9)
}

func (uc *GameUseCase) drawNumbers(ctx context.Context, gameID uuid.UUID) {
	token := uuid.NewString()
	if !uc.acquireDrawLease(ctx, gameID, token) {
		return // another live instance owns the draw — leave it to them
	}
	defer uc.redisService.ReleaseDrawLease(context.Background(), gameID, token)

	// Compute the target draw count from the current player count before the
	// first number is drawn.
	totalPlayers := uc.totalActivePlayers(ctx, gameID)
	targetDrawCount := biasedDrawTarget(totalPlayers)

	// Grace before the first number so players redirecting from the picker
	// have their game socket connected and hear the first call live (see
	// domain.FirstDrawDelay).
	time.Sleep(domain.FirstDrawDelay)

	ticker := time.NewTicker(domain.DrawInterval)
	defer ticker.Stop()

	for {
		<-ticker.C

		// Keep our ownership fresh; if we lost it, another instance has taken over.
		if ok, _ := uc.redisService.AcquireOrRenewDrawLease(ctx, gameID, token, domain.DrawLeaseTTL); !ok {
			return
		}

		// Check if game is still in drawing state
		game, err := uc.gameRepo.FindByID(ctx, gameID)
		if err != nil || game.State != domain.GameStateDrawing {
			return
		}

		// Get drawn numbers
		drawnNumbers, _ := uc.redisService.GetDrawnNumbers(ctx, gameID)
		numbers := make([]int, len(drawnNumbers))
		for i, dn := range drawnNumbers {
			numbers[i] = dn.Number
		}

		forceComplete := len(drawnNumbers)+1 >= targetDrawCount && uc.botRepo != nil
		letter, number, err := uc.biasedDraw(ctx, gameID, numbers, targetDrawCount, forceComplete)
		if err != nil || number == 0 {
			// Configuration/repository failures degrade to an ordinary fair draw.
			// Exhaustion is the only condition that cancels and refunds the game.
			letter, number, err = bingo.DrawNextNumber(numbers)
			if err != nil || number == 0 {
				if _, _, _, cerr := uc.cancelGameAndRefund(ctx, gameID, "all numbers drawn, no winner"); cerr != nil {
					fmt.Printf("Warning: failed to auto-cancel exhausted game %s: %v\n", gameID, cerr)
				}
				go func() {
					defer utils.RecoverPanic("spawnNextGame")
					if newGame, err := uc.CreateOrGetGame(context.Background(), game.GameType); err == nil && newGame != nil {
						uc.redisService.PublishEvent(context.Background(), gameID, domain.WebSocketEventNewGameAvailable, map[string]interface{}{
							"gameId":   newGame.ID.String(),
							"gameType": string(newGame.GameType),
						})
					}
				}()
				return
			}
		}

		// Save drawn number
		drawnAt := time.Now()
		drawnNumber := domain.DrawnNumber{
			Letter:  domain.BingoLetter(letter),
			Number:  number,
			DrawnAt: drawnAt,
		}

		uc.redisService.AddDrawnNumber(ctx, gameID, drawnNumber)
		uc.gameRepo.SaveDrawnNumber(ctx, gameID, letter, number)

		// Publish event
		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventNumberDrawn, map[string]interface{}{
			"letter":   letter,
			"number":   number,
			"drawn_at": drawnAt.Format(time.RFC3339),
		})

		// Auto-declare a winner the instant a card completes a pattern with the
		// number just drawn — players no longer need to press a Bingo button.
		// Only enforce this after the minimum target draw count so the round
		// lasts at least the configured number of balls.
		if len(drawnNumbers)+1 >= targetDrawCount && uc.checkAutoBingo(ctx, gameID, game) {
			return
		}
	}
}

// gameDrawPolicy is the per-room runtime policy. The admin setting is
// preserved in storage, but it is automatically suspended in rooms at or above
// the configured real-player minimum and whenever no bot is actually present.
type gameDrawPolicy struct {
	mode       domain.BiasedDrawMode
	bonusGuard bool
}

func effectiveGameDrawPolicy(cfg domain.BotConfig, players []*domain.GamePlayer) gameDrawPolicy {
	mode := cfg.EffectiveBiasedDrawMode()
	activeBots := 0
	realUsers := make(map[uuid.UUID]bool)
	hasBonusFundedCard := false
	for _, p := range players {
		if p == nil || p.IsEliminated || p.LeftAt != nil {
			continue
		}
		if p.IsBot {
			activeBots++
			continue
		}
		realUsers[p.UserID] = true
		if p.Paid && p.PaidFromBonus {
			hasBonusFundedCard = true
		}
	}

	// A room without bots must always be genuinely fair, even when the saved
	// admin policy is Legacy or Protected.
	if activeBots == 0 {
		return gameDrawPolicy{mode: domain.BiasedDrawModeDisabled}
	}

	minimum := cfg.MinimumRoomPlayers
	if minimum < domain.MinPlayers {
		minimum = 20
	}
	if len(realUsers) >= minimum {
		// High-population rooms suspend general bot advantage. When the saved
		// policy is enabled, only actually bonus-funded cards are ineligible;
		// wallet-funded humans and bots remain equal competitors.
		if mode != domain.BiasedDrawModeDisabled && hasBonusFundedCard {
			return gameDrawPolicy{mode: domain.BiasedDrawModeDisabled, bonusGuard: true}
		}
		return gameDrawPolicy{mode: domain.BiasedDrawModeDisabled}
	}

	return gameDrawPolicy{mode: mode}
}

// winnerCard is one card that has completed a valid bingo, together with the
// auto-daubbed marks that prove it.
type winnerCard struct {
	UserID uuid.UUID
	CardID int
	Marked []int
}

// checkAutoBingo scans every active card after a number is drawn and finalizes
// the game the moment one or more cards complete a valid pattern. Because
// auto-daub marks every drawn number on every card, a card wins exactly when its
// marks (the free center plus every drawn number on the card) form a bingo over
// the current draw set — no manual claim required. When several cards complete
// on the same draw, they are all co-winners and split the prize pool.
//
// Returns true once the game has been resolved, so the draw loop can stop.
func (uc *GameUseCase) checkAutoBingo(ctx context.Context, gameID uuid.UUID, game *domain.Game) bool {
	drawnSet, err := uc.drawnNumberSet(ctx, gameID)
	if err != nil {
		return false
	}

	winners, err := uc.collectWinners(ctx, gameID, drawnSet)
	if err != nil || len(winners) == 0 {
		return false
	}

	if _, err := uc.finalizeWinners(ctx, game, winners); err != nil {
		fmt.Printf("Warning: auto-bingo finalize failed for game %s: %v\n", gameID, err)
		return false
	}
	// The game is resolved (won here, or already claimed elsewhere) — stop drawing.
	return true
}

// drawnNumberSet returns the set of numbers drawn so far for quick membership tests.
func (uc *GameUseCase) drawnNumberSet(ctx context.Context, gameID uuid.UUID) (map[int]bool, error) {
	drawnNumbers, err := uc.redisService.GetDrawnNumbers(ctx, gameID)
	if err != nil {
		return nil, err
	}
	drawnSet := make(map[int]bool, len(drawnNumbers))
	for _, dn := range drawnNumbers {
		drawnSet[dn.Number] = true
	}
	return drawnSet, nil
}

func winnerEligible(policy gameDrawPolicy, player *domain.GamePlayer) bool {
	if player == nil || player.IsEliminated || player.LeftAt != nil {
		return false
	}
	return !policy.bonusGuard || player.IsBot || !player.PaidFromBonus
}

// collectWinners returns every eligible active card that currently forms a
// valid bingo. Low-population rooms use the saved admin policy. High-population
// bonus-guard rooms discard only bonus-funded winning cards; wallet-funded
// humans and bots remain equal co-winners and split the pot normally.
func (uc *GameUseCase) collectWinners(ctx context.Context, gameID uuid.UUID, drawnSet map[int]bool) ([]winnerCard, error) {
	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
	if err != nil {
		return nil, err
	}

	sort.Slice(players, func(i, j int) bool {
		if players[i].JoinedAt.Equal(players[j].JoinedAt) {
			return players[i].CardID < players[j].CardID
		}
		return players[i].JoinedAt.Before(players[j].JoinedAt)
	})

	policy := gameDrawPolicy{mode: domain.BiasedDrawModeDisabled}
	if cfg, err := uc.botRepo.GetConfig(ctx); err == nil {
		policy = effectiveGameDrawPolicy(*cfg, players)
	}

	allWinners := make([]winnerCard, 0)
	botWinners := make([]winnerCard, 0)
	humanWinners := make([]winnerCard, 0)

	for _, p := range players {
		if !winnerEligible(policy, p) {
			continue
		}
		card := bingo.GenerateCard(p.CardID)
		if card == nil {
			continue
		}
		marked := autoDaubMarks(card, drawnSet)
		if !bingo.ValidateBingo(card, marked) {
			continue
		}

		wc := winnerCard{UserID: p.UserID, CardID: p.CardID, Marked: marked}
		allWinners = append(allWinners, wc)
		if p.IsBot {
			botWinners = append(botWinners, wc)
		} else {
			humanWinners = append(humanWinners, wc)
		}
	}

	if len(botWinners) > 0 && len(humanWinners) > 0 {
		return selectMixedWinners(allWinners, botWinners, policy.mode != domain.BiasedDrawModeDisabled), nil
	}
	if len(botWinners) > 0 {
		return botWinners, nil
	}
	return allWinners, nil
}

// selectMixedWinners preserves the bot-only result while a biased mode is on.
// When it is off, all simultaneously winning cards share the prize.
func selectMixedWinners(allWinners, botWinners []winnerCard, botAlwaysWin bool) []winnerCard {
	if botAlwaysWin {
		return botWinners
	}
	return allWinners
}

// appendDangerNumbers adds every undrawn number on card that would complete at
// least one valid bingo pattern. It deliberately mirrors ValidateBingo,
// including four corners.
func appendDangerNumbers(card *bingo.BingoCard, drawnSet map[int]bool, dangerMap map[int]bool) {
	marked := make(map[int]bool, len(drawnSet)+1)
	for n := range drawnSet {
		marked[n] = true
	}
	marked[0] = true

	rows := func() {
		for row := 0; row < 5; row++ {
			unmarked := make([]int, 0)
			for col := 0; col < 5; col++ {
				n := card.Numbers[row][col]
				if !marked[n] {
					unmarked = append(unmarked, n)
				}
			}
			if len(unmarked) == 1 {
				dangerMap[unmarked[0]] = true
			}
		}
	}
	cols := func() {
		for col := 0; col < 5; col++ {
			unmarked := make([]int, 0)
			for row := 0; row < 5; row++ {
				n := card.Numbers[row][col]
				if !marked[n] {
					unmarked = append(unmarked, n)
				}
			}
			if len(unmarked) == 1 {
				dangerMap[unmarked[0]] = true
			}
		}
	}
	diags := func() {
		unmarked := make([]int, 0)
		for i := 0; i < 5; i++ {
			n := card.Numbers[i][i]
			if !marked[n] {
				unmarked = append(unmarked, n)
			}
		}
		if len(unmarked) == 1 {
			dangerMap[unmarked[0]] = true
		}
		unmarked = make([]int, 0)
		for i := 0; i < 5; i++ {
			n := card.Numbers[i][4-i]
			if !marked[n] {
				unmarked = append(unmarked, n)
			}
		}
		if len(unmarked) == 1 {
			dangerMap[unmarked[0]] = true
		}
	}
	corners := func() {
		positions := [][2]int{{0, 0}, {0, 4}, {4, 0}, {4, 4}}
		unmarked := make([]int, 0, len(positions))
		for _, pos := range positions {
			n := card.Numbers[pos[0]][pos[1]]
			if !marked[n] {
				unmarked = append(unmarked, n)
			}
		}
		if len(unmarked) == 1 {
			dangerMap[unmarked[0]] = true
		}
	}

	rows()
	cols()
	diags()
	corners()
}

// isLegacyBiasTarget reproduces the pre-protected policy: only active, paid,
// bonus-funded human cards contribute to humanDanger.
func isLegacyBiasTarget(p *domain.GamePlayer) bool {
	return p != nil && !p.IsBot && !p.IsEliminated && p.LeftAt == nil && p.Paid && p.PaidFromBonus
}

// legacySafeDrawCandidates reproduces the implementation that preceded the
// protected mode: draw from the full 1-75 hopper while temporarily excluding
// numbers that complete a paid bonus-funded human card.
func legacySafeDrawCandidates(players []*domain.GamePlayer, drawnSet map[int]bool) []int {
	humanDanger := make(map[int]bool)
	for _, p := range players {
		if !isLegacyBiasTarget(p) {
			continue
		}
		card := bingo.GenerateCard(p.CardID)
		if card != nil {
			appendDangerNumbers(card, drawnSet, humanDanger)
		}
	}

	allUndrawn := make([]int, 0, domain.BingoNumberMaxO-domain.BingoNumberMinB+1)
	safe := make([]int, 0, cap(allUndrawn))
	for n := domain.BingoNumberMinB; n <= domain.BingoNumberMaxO; n++ {
		if drawnSet[n] {
			continue
		}
		allUndrawn = append(allUndrawn, n)
		if !humanDanger[n] {
			safe = append(safe, n)
		}
	}

	if len(safe) > 0 {
		return safe
	}
	return allUndrawn
}

// botSafeDrawCandidates builds humanDanger from every active real player's card
// and restricts the draw pool to undrawn numbers present on active bot cards.
//
// Before the target draw count, a candidate must complete neither a human nor a
// bot bingo. At/after the target, a human-safe bot-completing number is
// preferred so the round can finish; if none exists, another human-safe
// bot-card number keeps the round moving.
func botSafeDrawCandidates(players []*domain.GamePlayer, drawnSet map[int]bool, forceComplete bool) []int {
	humanDanger := make(map[int]bool)
	botDanger := make(map[int]bool)
	botNumbers := make([]int, 0)
	seenBotNumbers := make(map[int]bool)

	for _, p := range players {
		if p == nil || p.IsEliminated || p.LeftAt != nil {
			continue
		}
		card := bingo.GenerateCard(p.CardID)
		if card == nil {
			continue
		}

		if !p.IsBot {
			appendDangerNumbers(card, drawnSet, humanDanger)
			continue
		}

		appendDangerNumbers(card, drawnSet, botDanger)
		for row := 0; row < domain.CardGridSize; row++ {
			for col := 0; col < domain.CardGridSize; col++ {
				n := card.Numbers[row][col]
				if n == domain.CardCenterValue || drawnSet[n] || seenBotNumbers[n] {
					continue
				}
				seenBotNumbers[n] = true
				botNumbers = append(botNumbers, n)
			}
		}
	}

	if forceComplete {
		completing := make([]int, 0)
		for _, n := range botNumbers {
			if botDanger[n] && !humanDanger[n] {
				completing = append(completing, n)
			}
		}
		if len(completing) > 0 {
			return completing
		}
	}

	safe := make([]int, 0, len(botNumbers))
	for _, n := range botNumbers {
		if humanDanger[n] || (!forceComplete && botDanger[n]) {
			continue
		}
		safe = append(safe, n)
	}

	return safe
}

// fullHopperSafeDrawCandidates provides a non-cancelling fallback when the
// bot-card pool has no candidate. It considers all 1-75 undrawn numbers,
// always excludes humanDanger, and prefers numbers outside botDanger.
func fullHopperSafeDrawCandidates(players []*domain.GamePlayer, drawnSet map[int]bool) []int {
	humanDanger := make(map[int]bool)
	botDanger := make(map[int]bool)
	for _, p := range players {
		if p == nil || p.IsEliminated || p.LeftAt != nil {
			continue
		}
		card := bingo.GenerateCard(p.CardID)
		if card == nil {
			continue
		}
		if p.IsBot {
			appendDangerNumbers(card, drawnSet, botDanger)
		} else {
			appendDangerNumbers(card, drawnSet, humanDanger)
		}
	}

	strictSafe := make([]int, 0, domain.BingoNumberMaxO-domain.BingoNumberMinB+1)
	humanSafe := make([]int, 0, cap(strictSafe))
	for n := domain.BingoNumberMinB; n <= domain.BingoNumberMaxO; n++ {
		if drawnSet[n] || humanDanger[n] {
			continue
		}
		humanSafe = append(humanSafe, n)
		if !botDanger[n] {
			strictSafe = append(strictSafe, n)
		}
	}

	if len(strictSafe) > 0 {
		return strictSafe
	}
	return humanSafe
}

// biasedDraw selects the effective per-room policy. Fair and bonus-guard rooms
// use the ordinary hopper; the bonus guard is enforced only when winners are
// selected so wallet-funded humans and bots see the same number stream.
func (uc *GameUseCase) biasedDraw(ctx context.Context, gameID uuid.UUID, drawnNumbers []int, _ int, forceComplete bool) (string, int, error) {
	cfg, err := uc.botRepo.GetConfig(ctx)
	if err != nil {
		return "", 0, err
	}
	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
	if err != nil {
		return "", 0, err
	}

	policy := effectiveGameDrawPolicy(*cfg, players)
	if policy.mode == domain.BiasedDrawModeDisabled {
		return bingo.DrawNextNumber(drawnNumbers)
	}

	drawnSet := make(map[int]bool, len(drawnNumbers))
	for _, n := range drawnNumbers {
		drawnSet[n] = true
	}

	var candidates []int
	if policy.mode == domain.BiasedDrawModeLegacy {
		candidates = legacySafeDrawCandidates(players, drawnSet)
	} else {
		candidates = botSafeDrawCandidates(players, drawnSet, forceComplete)
		if len(candidates) == 0 {
			// Preserve the protected non-cancelling fallback: leave the bot-card
			// pool, protect every human card, and prefer avoiding early bot bingo.
			candidates = fullHopperSafeDrawCandidates(players, drawnSet)
		}
	}
	if len(candidates) == 0 {
		return bingo.DrawNextNumber(drawnNumbers)
	}
	pick := candidates[rand.Intn(len(candidates))]
	return bingo.GetLetterForNumber(pick), pick, nil
}

// autoDaubMarks returns the marked numbers of a card under auto-daub: the free
// center plus every drawn number present on the card.
func autoDaubMarks(card *bingo.BingoCard, drawnSet map[int]bool) []int {
	marked := make([]int, 0, domain.CardTotalPositions)
	for row := 0; row < domain.CardGridSize; row++ {
		for col := 0; col < domain.CardGridSize; col++ {
			n := card.Numbers[row][col]
			if n == domain.CardCenterValue || drawnSet[n] {
				marked = append(marked, n)
			}
		}
	}
	return marked
}

// round2 rounds a birr amount to the santim (2 decimal places).
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// splitPot divides a prize pool into n even shares rounded to the santim. Any
// rounding remainder is folded into the first share so the shares always sum to
// exactly the pool (never paying out more than was staked).
func splitPot(pool float64, n int) []float64 {
	shares := make([]float64, n)
	if n <= 0 {
		return shares
	}
	share := round2(pool / float64(n))
	for i := range shares {
		shares[i] = share
	}
	remainder := round2(pool - share*float64(n))
	shares[0] = round2(share + remainder)
	return shares
}

// winnerDisplayName resolves a user's display name for winner events.
func (uc *GameUseCase) winnerDisplayName(ctx context.Context, userID uuid.UUID) string {
	name := "Unknown"
	if u, err := uc.userRepo.FindByID(ctx, userID); err == nil && u != nil {
		if u.LastName != nil && *u.LastName != "" {
			name = fmt.Sprintf("%s %s", u.FirstName, *u.LastName)
		} else {
			name = u.FirstName
		}
	}
	return name
}

// GetUserWinnings returns the user's summed prize money — today (Ethiopian
// time) and all time. Backs the WIN stat on the card picker.
func (uc *GameUseCase) GetUserWinnings(ctx context.Context, userID uuid.UUID) (today float64, total float64, err error) {
	return uc.gameRepo.GetUserWinnings(ctx, userID)
}

// GetUserGameStats returns a player's lifetime play record for admin review.
func (uc *GameUseCase) GetUserGameStats(ctx context.Context, userID uuid.UUID) (*domain.UserGameStats, error) {
	return uc.gameRepo.GetUserGameStats(ctx, userID)
}

// finalizeWinners closes the game and splits the prize pool evenly across every
// winning card. It is the single winner-resolution path shared by manual bingo
// claims and automatic bingo detection.
//
// The conditional ClaimWinner UPDATE is an atomic single-resolution guard: if
// two resolutions race (a manual claim racing the auto-check), only the first
// transitions DRAWING→FINISHED and pays out; the rest observe the game is
// already claimed and return (false, nil) — no error, no double payout. The
// winners slice must be pre-sorted deterministically; winners[0] becomes the
// primary winner (games.winner_id) and absorbs any rounding remainder so the
// credited total exactly equals the prize pool.
func (uc *GameUseCase) finalizeWinners(ctx context.Context, game *domain.Game, winners []winnerCard) (bool, error) {
	if len(winners) == 0 {
		return false, nil
	}

	gameID := game.ID
	primary := winners[0].UserID

	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Atomic single-resolution guard: claim the game only if it is still DRAWING.
	claimed, err := uc.gameRepo.ClaimWinner(ctx, tx, gameID, primary)
	if err != nil {
		return false, fmt.Errorf("failed to claim win: %w", err)
	}
	if !claimed {
		// The game was already resolved elsewhere — nothing to do.
		return false, nil
	}

	// Split the pot evenly across every winning card (santim-rounded, remainder
	// to the first winner) so the credited total is exactly the prize pool.
	n := len(winners)
	amounts := splitPot(game.PrizePool, n)

	// Reflect the claimed state in memory for the events/Redis updates below.
	game.State = domain.GameStateFinished
	game.WinnerID = &primary
	now := time.Now()
	game.FinishedAt = &now

	gamePrizeRef := "GAME_PRIZE"
	for i, w := range winners {
		amount := amounts[i]

		// Lock and credit the winner's wallet. Multiple cards may belong to the
		// same user; each card's share is credited separately to that wallet.
		if _, err := uc.walletRepo.LockForUpdate(ctx, tx, w.UserID); err != nil {
			return false, fmt.Errorf("wallet not found: %w", err)
		}
		if err := uc.walletRepo.UpdateBalance(ctx, tx, w.UserID, amount); err != nil {
			return false, fmt.Errorf("failed to add prize: %w", err)
		}

		// Record the prize as a completed transaction (GAME_PRIZE excludes it
		// from deposit history).
		if err := uc.transactionRepo.Create(ctx, tx, &domain.Transaction{
			UserID:    w.UserID,
			Type:      domain.TransactionTypeDeposit, // Prize is treated as deposit
			Category:  domain.TransactionCategoryWinnings,
			Amount:    amount,
			Status:    domain.TransactionStatusCompleted,
			Reference: &gamePrizeRef,
		}); err != nil {
			return false, fmt.Errorf("failed to create prize transaction: %w", err)
		}

		// Flag the winning card and record its share.
		if err := uc.gameRepo.MarkCardWinner(ctx, tx, gameID, w.UserID, w.CardID, amount); err != nil {
			return false, fmt.Errorf("failed to mark winning card: %w", err)
		}
	}

	// Commit transaction (ClaimWinner already persisted the FINISHED state).
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Update Redis
	uc.redisService.SaveGameState(ctx, game)

	// Build the winners payload (each card's owner, share, and proving marks).
	winnersPayload := make([]map[string]interface{}, n)
	for i, w := range winners {
		winnersPayload[i] = map[string]interface{}{
			"user_id":        w.UserID.String(),
			"winner_name":    uc.winnerDisplayName(ctx, w.UserID),
			"prize":          amounts[i],
			"card_id":        w.CardID,
			"marked_numbers": w.Marked,
		}
	}

	// Publish the winner event. Top-level fields describe the primary winner for
	// backward compatibility with older clients; `winners` carries every
	// co-winner and their individual share, and `prize_pool` is the full pot.
	primaryPayload := winnersPayload[0]
	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventWinner, map[string]interface{}{
		"user_id":        primaryPayload["user_id"],
		"winner_name":    primaryPayload["winner_name"],
		"prize":          primaryPayload["prize"],
		"card_id":        primaryPayload["card_id"],
		"marked_numbers": primaryPayload["marked_numbers"],
		"prize_pool":     game.PrizePool,
		"split":          n > 1,
		"winners":        winnersPayload,
	})

	// Publish game finished status
	uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
		"status": string(domain.GameStateFinished),
	})

	// Create a new game of the same type and notify clients
	go func() {
		defer utils.RecoverPanic("spawnNextGame")
		newGame, err := uc.CreateOrGetGame(context.Background(), game.GameType)
		if err == nil && newGame != nil {
			// Publish new game available event to the same channel so clients can
			// reconnect to this new game.
			uc.redisService.PublishEvent(context.Background(), gameID, domain.WebSocketEventNewGameAvailable, map[string]interface{}{
				"gameId":   newGame.ID.String(),
				"gameType": string(newGame.GameType),
			})
		}
	}()

	return true, nil
}

// ClaimBingo validates and processes a manual bingo claim. With automatic bingo
// detection now resolving games as soon as a card completes (see checkAutoBingo),
// this endpoint is a fallback/no-op in most games, but is kept working so an
// explicit claim still pays out correctly — splitting the pot with any co-winners
// through the shared finalizeWinners path.
func (uc *GameUseCase) ClaimBingo(ctx context.Context, gameID uuid.UUID, req domain.ClaimBingoRequest) (bool, error) {
	// Get game
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		return false, fmt.Errorf("game not found: %w", err)
	}

	// Check game state
	if game.State != domain.GameStateDrawing {
		return false, errors.New("game is not in drawing phase")
	}

	// Resolve the specific card being claimed. It must belong to this user and
	// still be active in the game.
	player, err := uc.gameRepo.FindPlayerCard(ctx, gameID, req.UserID, req.CardID)
	if err != nil {
		return false, errors.New("you do not hold that card in this game")
	}

	if player.IsEliminated {
		return false, errors.New("this card is already eliminated")
	}

	// Get drawn numbers
	drawnNumbers, err := uc.redisService.GetDrawnNumbers(ctx, gameID)
	if err != nil {
		return false, fmt.Errorf("failed to get drawn numbers: %w", err)
	}

	// Convert to set for quick lookup
	drawnSet := make(map[int]bool)
	for _, dn := range drawnNumbers {
		drawnSet[dn.Number] = true
	}

	// Generate card
	card := bingo.GenerateCard(player.CardID)
	if card == nil {
		return false, fmt.Errorf("invalid card ID")
	}

	// Convert marked positions (0-24) to actual card numbers
	markedNumbers := make([]int, 0, len(req.MarkedNumbers))
	for _, pos := range req.MarkedNumbers {
		if pos < 0 || pos >= domain.CardTotalPositions {
			return false, fmt.Errorf("invalid position: %d (must be 0-%d)", pos, domain.CardTotalPositions-1)
		}

		// Convert position to row/col: position = row * CardGridSize + col
		row := pos / domain.CardGridSize
		col := pos % domain.CardGridSize
		cardNumber := card.Numbers[row][col]

		// Verify this number was actually drawn
		if cardNumber != domain.CardCenterValue && !drawnSet[cardNumber] {
			return false, fmt.Errorf("number %d at position %d was not drawn", cardNumber, pos)
		}

		// Add to marked numbers (include CardCenterValue for center cell)
		markedNumbers = append(markedNumbers, cardNumber)
	}

	// Validate bingo with the marked numbers
	isValid := bingo.ValidateBingo(card, markedNumbers)

	// Start transaction
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if isValid {
		// Valid claim — resolve the game through the shared finalizeWinners path,
		// splitting the pot with any other cards that completed on the same draw.
		// Auto-daub marks are a superset of the client's marked positions, so the
		// claimant's card is guaranteed to be among the collected winners. The
		// atomic guard inside finalizeWinners prevents a double payout if a manual
		// claim and the auto-check race.
		winners, werr := uc.collectWinners(ctx, gameID, drawnSet)
		if werr != nil {
			return false, fmt.Errorf("failed to collect winners: %w", werr)
		}
		return uc.finalizeWinners(ctx, game, winners)
	} else {
		// Invalid claim - eliminate only this card; the player's other cards live on.
		if err := uc.gameRepo.EliminatePlayerCard(ctx, tx, gameID, req.UserID, req.CardID); err != nil {
			return false, fmt.Errorf("failed to eliminate card: %w", err)
		}

		// Count cards still in play. GetPlayers reads committed state, which does
		// not yet reflect this transaction's elimination, so skip the just-killed
		// card explicitly.
		// The error here used to be discarded, which was the worse half of the
		// bug below: a failed read yields no players, so activeCards lands on 0
		// and the game commits as CANCELLED having refunded nobody.
		players, err := uc.gameRepo.GetPlayers(ctx, gameID)
		if err != nil {
			return false, fmt.Errorf("failed to read players for elimination check: %w", err)
		}
		activeCards := 0
		for _, p := range players {
			if p.IsEliminated {
				continue
			}
			if p.UserID == req.UserID && p.CardID == req.CardID {
				continue // eliminated in this tx
			}
			activeCards++
		}

		if activeCards == 0 {
			// Every card is out - cancel the game and refund each card's stake
			game.State = domain.GameStateCancelled
			gameRefundRef := "GAME_REFUND"
			// Every refund error is fatal to the whole transaction. Previously a
			// failed wallet lock silently skipped that player's refund entirely
			// (`if err == nil`), and UpdateBalance/Create errors were discarded
			// outright — then the game committed as CANCELLED regardless. That
			// left players unrefunded on a cancelled game with nothing logged.
			// Returning rolls back the cancellation with the refunds, so the
			// game stays live and the claim can be retried.
			for _, p := range players {
				// Routes bonus-funded stakes back to bonus rather than cash.
				if err := uc.refundCardStake(ctx, tx, p, game.BetAmount, &gameRefundRef); err != nil {
					return false, err
				}
			}
		}

		if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
			return false, fmt.Errorf("failed to update game: %w", err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("failed to commit transaction: %w", err)
		}

		// Update Redis
		uc.redisService.SaveGameState(ctx, game)

		// Publish elimination event
		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventPlayerEliminated, map[string]interface{}{
			"userId": req.UserID.String(),
		})

		// If game was cancelled, publish status and create new game
		if game.State == domain.GameStateCancelled {
			uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
				"status": string(domain.GameStateCancelled),
			})

			// Create a new game of the same type and notify clients
			go func() {
				defer utils.RecoverPanic("spawnNextGame")
				newGame, err := uc.CreateOrGetGame(context.Background(), game.GameType)
				if err == nil && newGame != nil {
					uc.redisService.PublishEvent(context.Background(), gameID, domain.WebSocketEventNewGameAvailable, map[string]interface{}{
						"gameId":   newGame.ID.String(),
						"gameType": string(newGame.GameType),
					})
				}
			}()
		}

		return false, nil
	}
}

// GetGameState gets the current game state
func (uc *GameUseCase) GetGameState(ctx context.Context, gameID uuid.UUID) (*domain.Game, []domain.DrawnNumber, []int, error) {
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("game not found: %w", err)
	}

	var drawnNumbers []domain.DrawnNumber
	var takenCards []int

	if uc.redisService != nil {
		drawnNumbers, _ = uc.redisService.GetDrawnNumbers(ctx, gameID)
		takenCards, _ = uc.redisService.GetTakenCards(ctx, gameID)
	} else {
		// Fallback to database
		takenCards, _ = uc.gameRepo.GetTakenCards(ctx, gameID)
		// drawnNumbers would need DB implementation
	}

	return game, drawnNumbers, takenCards, nil
}

// GetGameWinners returns the winning card(s) of a finished game — each with its
// prize share and the marked numbers reconstructed from the drawn set via
// auto-daub. There may be several (co-winners who split the pot). Used to render
// the post-game winner card(s), including for clients that connect after the
// transient live winner event. Returns an empty slice for games with no winner.
func (uc *GameUseCase) GetGameWinners(ctx context.Context, gameID uuid.UUID) ([]domain.GameWinner, error) {
	rows, err := uc.gameRepo.FindWinningCards(ctx, gameID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []domain.GameWinner{}, nil
	}

	// Reconstruct each winner's marks from the drawn set (nil-safe if the draw
	// history is no longer cached — the card still renders, just unhighlighted).
	drawnSet, _ := uc.drawnNumberSet(ctx, gameID)

	winners := make([]domain.GameWinner, 0, len(rows))
	for _, w := range rows {
		out := *w
		if card := bingo.GenerateCard(w.CardID); card != nil {
			out.MarkedNumbers = autoDaubMarks(card, drawnSet)
		}
		winners = append(winners, out)
	}
	return winners, nil
}

// GetPlayerInGame checks if a player is in a game (for WebSocket validation)
func (uc *GameUseCase) GetPlayerInGame(ctx context.Context, gameID, userID uuid.UUID) (*domain.GamePlayer, error) {
	return uc.gameRepo.FindPlayer(ctx, gameID, userID)
}

// GetMyCardsInGame returns all of the authenticated user's active cards in a
// game (a player may hold up to MaxCardsPerPlayer).
func (uc *GameUseCase) GetMyCardsInGame(ctx context.Context, gameID, userID uuid.UUID) ([]*domain.GamePlayer, error) {
	return uc.gameRepo.FindPlayersByUser(ctx, gameID, userID)
}

// GetCardData returns the card data for a given card ID
func (uc *GameUseCase) GetCardData(ctx context.Context, cardID int) (*bingo.BingoCard, error) {
	// Validate card ID
	if cardID < domain.MinCardID || cardID > domain.MaxCardID {
		return nil, fmt.Errorf("card ID must be between %d and %d", domain.MinCardID, domain.MaxCardID)
	}

	// Generate card (deterministic - same card_id always generates same card)
	card := bingo.GenerateCard(cardID)
	return card, nil
}

// ListGames returns games for the admin dashboard, filtered by optional state
// and type, with pagination. Returns the games and the total matching count.
func (uc *GameUseCase) ListGames(ctx context.Context, filter domain.AdminGameFilter, limit, offset int) ([]*domain.Game, int, error) {
	if limit <= 0 {
		limit = domain.MaxAvailableGamesLimit
	}
	if offset < 0 {
		offset = 0
	}

	games, err := uc.gameRepo.FindAdmin(ctx, filter, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list games: %w", err)
	}

	total, err := uc.gameRepo.CountAdmin(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count games: %w", err)
	}

	return games, total, nil
}

// GetGameDetail returns a game and its active players enriched with user info,
// for the admin dashboard.
func (uc *GameUseCase) GetGameDetail(ctx context.Context, gameID uuid.UUID) (*domain.AdminGameDetail, error) {
	game, err := uc.gameRepo.FindByID(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("game not found: %w", err)
	}

	players, err := uc.gameRepo.GetPlayers(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get players: %w", err)
	}

	detail := &domain.AdminGameDetail{
		Game:    game,
		Players: make([]*domain.AdminGamePlayer, 0, len(players)),
	}

	for _, p := range players {
		entry := &domain.AdminGamePlayer{
			UserID:       p.UserID,
			CardID:       p.CardID,
			IsEliminated: p.IsEliminated,
			JoinedAt:     p.JoinedAt,
		}
		// Best-effort user enrichment; a missing user shouldn't break the view.
		if user, err := uc.userRepo.FindByID(ctx, p.UserID); err == nil && user != nil {
			entry.FirstName = user.FirstName
			entry.LastName = user.LastName
			entry.PhoneNumber = user.PhoneNumber
			entry.TelegramID = user.TelegramID
		}
		detail.Players = append(detail.Players, entry)
	}

	detail.Funding = summarizeGameFunding(players)

	// Winners of a finished game (who won, which card, how much). Best-effort —
	// an error here shouldn't break the whole detail view.
	if winners, err := uc.gameRepo.FindWinningCards(ctx, gameID); err == nil {
		detail.Winners = winners
	}

	return detail, nil
}

// CancelGame is the admin force-cancel: it cancels the game and refunds every
// active player's stake. Only games that are not yet resolved can be cancelled.
func (uc *GameUseCase) CancelGame(ctx context.Context, gameID uuid.UUID) (*domain.CancelGameResult, error) {
	game, count, amount, err := uc.cancelGameAndRefund(ctx, gameID, "cancelled by admin")
	if err != nil {
		return nil, err
	}
	return &domain.CancelGameResult{
		Game:           game,
		RefundedCount:  count,
		RefundedAmount: amount,
	}, nil
}

// cancelGameAndRefund cancels a game and refunds every active player's stake in
// a single transaction. It locks the game row first to serialize against winner
// claims and concurrent cancels. Returns the updated game, the number of players
// refunded, and the total amount refunded. Shared by the admin force-cancel and
// the automatic "numbers exhausted, no winner" path in drawNumbers.
func (uc *GameUseCase) cancelGameAndRefund(ctx context.Context, gameID uuid.UUID, reason string) (*domain.Game, int, float64, error) {
	tx, err := uc.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock the game row. This blocks a concurrent winner claim's conditional
	// UPDATE (WHERE state='DRAWING') until we commit, after which it affects
	// zero rows — so we can never both refund and pay out a prize.
	game, err := uc.gameRepo.LockForUpdate(ctx, tx, gameID)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("game not found: %w", err)
	}

	// Only games that are still in play can be cancelled.
	if game.State == domain.GameStateFinished ||
		game.State == domain.GameStateCancelled ||
		game.State == domain.GameStateClosed {
		return nil, 0, 0, fmt.Errorf("game is already resolved (state: %s)", game.State)
	}

	players, err := uc.gameRepo.GetActivePlayersTx(ctx, tx, gameID)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to get active players: %w", err)
	}

	refundRef := "GAME_REFUND"
	refundedCount := 0
	var refundedAmount float64

	for _, p := range players {
		// Transition the card out FIRST, and refund only if it actually moved.
		// If it was already left (0 rows), skip so a stake can't be refunded twice.
		rows, err := uc.gameRepo.RemovePlayerCard(ctx, tx, gameID, p.UserID, p.CardID)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("failed to remove player card: %w", err)
		}
		if rows == 0 {
			continue
		}

		// Reserved (unpaid) cards moved no money — release them without a refund.
		if !p.Paid {
			continue
		}

		if err := uc.refundCardStake(ctx, tx, p, game.BetAmount, &refundRef); err != nil {
			return nil, 0, 0, err
		}

		refundedCount++
		refundedAmount += game.BetAmount
	}

	// Mark the game cancelled and zero out its live counters.
	game.State = domain.GameStateCancelled
	game.PlayerCount = 0
	game.PrizePool = 0
	if err := uc.gameRepo.UpdateTx(ctx, tx, game); err != nil {
		return nil, 0, 0, fmt.Errorf("failed to cancel game: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, 0, 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Best-effort live-state sync + notification (outside the money transaction).
	if uc.redisService != nil {
		uc.redisService.SaveGameState(ctx, game)
		uc.redisService.PublishEvent(ctx, gameID, domain.WebSocketEventGameStatus, map[string]interface{}{
			"status": string(domain.GameStateCancelled),
			"reason": reason,
		})
	}

	return game, refundedCount, refundedAmount, nil
}

// GetRecentWinners returns the most recent game winners for the public lobby feed.
func (uc *GameUseCase) GetRecentWinners(ctx context.Context, limit int) ([]*domain.RecentWinner, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	winners, err := uc.gameRepo.FindRecentWinners(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent winners: %w", err)
	}
	return winners, nil
}

// GetActiveGame returns the live game the user is currently in (still holding
// cards, state WAITING/COUNTDOWN/DRAWING), or nil if they're not in one. Lets a
// player who navigated away mid-game find and rejoin it.
func (uc *GameUseCase) GetActiveGame(ctx context.Context, userID uuid.UUID) (*domain.GameHistoryEntry, error) {
	game, err := uc.gameRepo.FindActiveGameByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active game: %w", err)
	}
	return game, nil
}

// GetGameHistory returns the game history for a user
func (uc *GameUseCase) GetGameHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.GameHistoryEntry, error) {
	if limit <= 0 {
		limit = domain.DefaultTransactionHistoryLimit
	}
	if offset < 0 {
		offset = 0
	}

	history, err := uc.gameRepo.FindGamesByUserID(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get game history: %w", err)
	}

	return history, nil
}

// GetGameHistoryPage returns the rows plus the total distinct games so the
// admin user-detail card can paginate without guessing from a short page.
func (uc *GameUseCase) GetGameHistoryPage(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.GameHistoryEntry, int, error) {
	history, err := uc.GetGameHistory(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.gameRepo.CountGamesByUserID(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count game history: %w", err)
	}
	return history, total, nil
}

// summarizeGameFunding counts distinct real players by the source of their paid
// cards. Several cards from the same source count once; a mixed-source player is
// counted in both source totals and explicitly in MixedPlayers.
func summarizeGameFunding(players []*domain.GamePlayer) domain.AdminGameFundingStats {
	type flags struct {
		wallet bool
		bonus  bool
	}
	byUser := make(map[uuid.UUID]flags)
	for _, p := range players {
		if !p.Paid || p.IsBot || p.LeftAt != nil {
			continue
		}
		f := byUser[p.UserID]
		if p.PaidFromBonus {
			f.bonus = true
		} else {
			f.wallet = true
		}
		byUser[p.UserID] = f
	}

	stats := domain.AdminGameFundingStats{TotalPlayers: len(byUser)}
	for _, f := range byUser {
		if f.wallet {
			stats.WalletPlayers++
		}
		if f.bonus {
			stats.BonusPlayers++
		}
		if f.wallet && f.bonus {
			stats.MixedPlayers++
		}
	}
	return stats
}
