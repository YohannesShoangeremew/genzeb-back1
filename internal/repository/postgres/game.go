package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
)

type gameRepository struct {
	db *sql.DB
}

// NewGameRepository creates a new PostgreSQL game repository
func NewGameRepository(db *sql.DB) domain.GameRepository {
	return &gameRepository{db: db}
}

// eatZone is Ethiopian time (EAT, UTC+3, no DST). Used to decide which calendar
// day a game belongs to for its human-readable round code.
var eatZone = time.FixedZone("EAT", 3*60*60)

// Create creates a new game, assigning a human-readable daily round code
// (e.g. "0714-03") atomically so concurrent creates never collide.
func (r *gameRepository) Create(ctx context.Context, game *domain.Game) error {
	now := time.Now()
	game.CreatedAt = now
	game.UpdatedAt = now

	nowEAT := now.In(eatZone)
	gameDay := nowEAT.Format("2006-01-02") // day bucket; the sequence resets each EAT day

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback()

	// Hand out the next sequence for today atomically.
	var seq int
	err = tx.QueryRowContext(ctx, `
		INSERT INTO daily_game_counter (game_day, last_seq)
		VALUES ($1, 1)
		ON CONFLICT (game_day) DO UPDATE SET last_seq = daily_game_counter.last_seq + 1
		RETURNING last_seq
	`, gameDay).Scan(&seq)
	if err != nil {
		return fmt.Errorf("failed to allocate round code: %w", err)
	}
	game.RoundCode = fmt.Sprintf("%d", seq) // just the daily count, e.g. "3"

	_, err = tx.ExecContext(ctx, `
		INSERT INTO games (id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut, round_code, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`,
		game.ID,
		game.GameType,
		game.State,
		game.BetAmount,
		game.MinPlayers,
		game.PlayerCount,
		game.PrizePool,
		game.HouseCut,
		game.RoundCode,
		game.CreatedAt,
		game.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create game: %w", err)
	}

	return tx.Commit()
}

// FindByID finds a game by ID
func (r *gameRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Game, error) {
	query := `
		SELECT id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut, round_code,
		       winner_id, countdown_ends, started_at, finished_at, created_at, updated_at
		FROM games
		WHERE id = $1
	`

	game := &domain.Game{}
	var roundCode sql.NullString
	var winnerID sql.NullString
	var countdownEnds sql.NullTime
	var startedAt sql.NullTime
	var finishedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&game.ID,
		&game.GameType,
		&game.State,
		&game.BetAmount,
		&game.MinPlayers,
		&game.PlayerCount,
		&game.PrizePool,
		&game.HouseCut,
		&roundCode,
		&winnerID,
		&countdownEnds,
		&startedAt,
		&finishedAt,
		&game.CreatedAt,
		&game.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("game not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find game: %w", err)
	}

	// Handle nullable fields
	game.RoundCode = roundCode.String
	if winnerID.Valid {
		parsedID, err := uuid.Parse(winnerID.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse winner_id: %w", err)
		}
		game.WinnerID = &parsedID
	}
	if countdownEnds.Valid {
		game.CountdownEnds = &countdownEnds.Time
	}
	if startedAt.Valid {
		game.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		game.FinishedAt = &finishedAt.Time
	}

	return game, nil
}

// FindAvailable finds available games (WAITING or COUNTDOWN state)
func (r *gameRepository) FindAvailable(ctx context.Context, gameType *domain.GameType, limit int) ([]*domain.Game, error) {
	query := `
		SELECT id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut, round_code,
		       winner_id, countdown_ends, started_at, finished_at, created_at, updated_at
		FROM games
		WHERE state IN ('WAITING', 'COUNTDOWN')
		  AND (
		        (game_type = 'REGULAR' AND bet_amount = $1)
		     OR (game_type = 'VIP' AND bet_amount = $2)
		  )
	`

	args := []interface{}{domain.BetAmountRegular, domain.BetAmountVIP}
	argPos := 3

	if gameType != nil {
		query += fmt.Sprintf(" AND game_type = $%d", argPos)
		args = append(args, *gameType)
		argPos++
	}

	query += " ORDER BY created_at ASC LIMIT $" + fmt.Sprintf("%d", argPos)
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to find available games: %w", err)
	}
	defer rows.Close()

	games := []*domain.Game{}
	for rows.Next() {
		game := &domain.Game{}
		var roundCode sql.NullString
		var winnerID sql.NullString
		var countdownEnds sql.NullTime
		var startedAt sql.NullTime
		var finishedAt sql.NullTime

		err := rows.Scan(
			&game.ID,
			&game.GameType,
			&game.State,
			&game.BetAmount,
			&game.MinPlayers,
			&game.PlayerCount,
			&game.PrizePool,
			&game.HouseCut,
			&roundCode,
			&winnerID,
			&countdownEnds,
			&startedAt,
			&finishedAt,
			&game.CreatedAt,
			&game.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan game: %w", err)
		}

		// Handle nullable fields
		game.RoundCode = roundCode.String
		if winnerID.Valid {
			parsedID, err := uuid.Parse(winnerID.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse winner_id: %w", err)
			}
			game.WinnerID = &parsedID
		}
		if countdownEnds.Valid {
			game.CountdownEnds = &countdownEnds.Time
		}
		if startedAt.Valid {
			game.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			game.FinishedAt = &finishedAt.Time
		}

		games = append(games, game)
	}

	// Check for errors from iterating over rows
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating games: %w", err)
	}

	return games, nil
}

// scanGame scans a single game row from a *sql.Row-like Scan into a domain.Game,
// handling the nullable columns. The column order must match the SELECT below.
func scanGame(scan func(dest ...interface{}) error) (*domain.Game, error) {
	game := &domain.Game{}
	var roundCode sql.NullString
	var winnerID sql.NullString
	var countdownEnds sql.NullTime
	var startedAt sql.NullTime
	var finishedAt sql.NullTime

	if err := scan(
		&game.ID,
		&game.GameType,
		&game.State,
		&game.BetAmount,
		&game.MinPlayers,
		&game.PlayerCount,
		&game.PrizePool,
		&game.HouseCut,
		&roundCode,
		&winnerID,
		&countdownEnds,
		&startedAt,
		&finishedAt,
		&game.CreatedAt,
		&game.UpdatedAt,
	); err != nil {
		return nil, err
	}

	game.RoundCode = roundCode.String
	if winnerID.Valid {
		parsedID, err := uuid.Parse(winnerID.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse winner_id: %w", err)
		}
		game.WinnerID = &parsedID
	}
	if countdownEnds.Valid {
		game.CountdownEnds = &countdownEnds.Time
	}
	if startedAt.Valid {
		game.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		game.FinishedAt = &finishedAt.Time
	}

	return game, nil
}

// FindAll returns games filtered by optional state and type, newest first.
func (r *gameRepository) FindAll(ctx context.Context, state *domain.GameState, gameType *domain.GameType, limit, offset int) ([]*domain.Game, error) {
	query := `
		SELECT id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut, round_code,
		       winner_id, countdown_ends, started_at, finished_at, created_at, updated_at
		FROM games
		WHERE 1=1
	`

	args := []interface{}{}
	argPos := 1

	if state != nil {
		query += fmt.Sprintf(" AND state = $%d", argPos)
		args = append(args, *state)
		argPos++
	}
	if gameType != nil {
		query += fmt.Sprintf(" AND game_type = $%d", argPos)
		args = append(args, *gameType)
		argPos++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argPos, argPos+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to find games: %w", err)
	}
	defer rows.Close()

	games := []*domain.Game{}
	for rows.Next() {
		game, err := scanGame(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to scan game: %w", err)
		}
		games = append(games, game)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating games: %w", err)
	}

	return games, nil
}

// CountAll counts games matching the optional state and type filters.
func (r *gameRepository) CountAll(ctx context.Context, state *domain.GameState, gameType *domain.GameType) (int, error) {
	query := `SELECT COUNT(*) FROM games WHERE 1=1`

	args := []interface{}{}
	argPos := 1

	if state != nil {
		query += fmt.Sprintf(" AND state = $%d", argPos)
		args = append(args, *state)
		argPos++
	}
	if gameType != nil {
		query += fmt.Sprintf(" AND game_type = $%d", argPos)
		args = append(args, *gameType)
		argPos++
	}

	var count int
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count games: %w", err)
	}
	return count, nil
}

// LockForUpdate locks a game row FOR UPDATE inside a transaction.
func (r *gameRepository) LockForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Game, error) {
	query := `
		SELECT id, game_type, state, bet_amount, min_players, player_count, prize_pool, house_cut, round_code,
		       winner_id, countdown_ends, started_at, finished_at, created_at, updated_at
		FROM games
		WHERE id = $1
		FOR UPDATE
	`

	game, err := scanGame(tx.QueryRowContext(ctx, query, id).Scan)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("game not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to lock game: %w", err)
	}
	return game, nil
}

// FindOtherActiveGameForUserTx finds a different live game in which the user
// still has at least one active, non-eliminated card. JoinGame calls this after
// taking the per-user wallet lock, which makes the check safe against concurrent
// joins to REGULAR and VIP games without blocking same-game multi-card play.
func (r *gameRepository) FindOtherActiveGameForUserTx(ctx context.Context, tx *sql.Tx, userID, gameID uuid.UUID) (*domain.Game, error) {
	query := `
		SELECT g.id, g.game_type, g.state, g.bet_amount, g.min_players, g.player_count, g.prize_pool, g.house_cut, g.round_code,
		       g.winner_id, g.countdown_ends, g.started_at, g.finished_at, g.created_at, g.updated_at
		FROM games g
		WHERE g.id <> $2
		  AND g.state IN ('WAITING', 'COUNTDOWN', 'DRAWING')
		  AND EXISTS (
			  SELECT 1
			  FROM game_players gp
			  WHERE gp.game_id = g.id
			    AND gp.user_id = $1
			    AND gp.left_at IS NULL
			    AND gp.is_eliminated = FALSE
		  )
		ORDER BY g.created_at DESC
		LIMIT 1
	`

	game, err := scanGame(tx.QueryRowContext(ctx, query, userID, gameID).Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find other active game: %w", err)
	}
	return game, nil
}

// UpdateTx updates a game row inside an existing transaction.
func (r *gameRepository) UpdateTx(ctx context.Context, tx *sql.Tx, game *domain.Game) error {
	query := `
		UPDATE games
		SET state = $2, player_count = $3, prize_pool = $4, house_cut = $5,
		    winner_id = $6, countdown_ends = $7, started_at = $8, finished_at = $9, updated_at = $10
		WHERE id = $1
	`

	game.UpdatedAt = time.Now()

	result, err := tx.ExecContext(ctx, query,
		game.ID,
		game.State,
		game.PlayerCount,
		game.PrizePool,
		game.HouseCut,
		game.WinnerID,
		game.CountdownEnds,
		game.StartedAt,
		game.FinishedAt,
		game.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update game: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("game not found")
	}
	return nil
}

// GetActivePlayersTx returns players still in the game (left_at IS NULL), read
// inside an existing transaction.
func (r *gameRepository) GetActivePlayersTx(ctx context.Context, tx *sql.Tx, gameID uuid.UUID) ([]*domain.GamePlayer, error) {
	query := `
		SELECT gp.id, gp.game_id, gp.user_id, gp.card_id, gp.paid, gp.is_eliminated, gp.joined_at, gp.left_at,
		       gp.paid_from_bonus, gp.bonus_expires_at, u.is_bot
		FROM game_players gp
		JOIN users u ON u.id = gp.user_id
		WHERE gp.game_id = $1 AND gp.left_at IS NULL
	`

	rows, err := tx.QueryContext(ctx, query, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active players: %w", err)
	}
	defer rows.Close()

	players := []*domain.GamePlayer{}
	for rows.Next() {
		player := &domain.GamePlayer{}
		if err := rows.Scan(
			&player.ID,
			&player.GameID,
			&player.UserID,
			&player.CardID,
			&player.Paid,
			&player.IsEliminated,
			&player.JoinedAt,
			&player.LeftAt,
			&player.PaidFromBonus,
			&player.BonusExpiresAt,
			&player.IsBot,
		); err != nil {
			return nil, fmt.Errorf("failed to scan player: %w", err)
		}
		players = append(players, player)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating players: %w", err)
	}

	return players, nil
}

// Update updates a game
func (r *gameRepository) Update(ctx context.Context, game *domain.Game) error {
	query := `
		UPDATE games
		SET state = $2, player_count = $3, prize_pool = $4, house_cut = $5,
		    winner_id = $6, countdown_ends = $7, started_at = $8, finished_at = $9, updated_at = $10
		WHERE id = $1
	`

	game.UpdatedAt = time.Now()

	result, err := r.db.ExecContext(ctx, query,
		game.ID,
		game.State,
		game.PlayerCount,
		game.PrizePool,
		game.HouseCut,
		game.WinnerID,
		game.CountdownEnds,
		game.StartedAt,
		game.FinishedAt,
		game.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update game: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("game not found")
	}

	return nil
}

// ClaimWinner atomically marks the game FINISHED with the given winner, but only
// if it is still in DRAWING state. The WHERE clause makes this a single-winner
// guard: with concurrent claims, exactly one UPDATE affects a row (returns true);
// the others affect zero rows (return false), so no double winner or double payout.
func (r *gameRepository) ClaimWinner(ctx context.Context, tx *sql.Tx, gameID, winnerID uuid.UUID) (bool, error) {
	query := `
		UPDATE games
		SET state = 'FINISHED', winner_id = $2, finished_at = $3, updated_at = $3
		WHERE id = $1 AND state = 'DRAWING'
	`

	now := time.Now()

	var result sql.Result
	var err error
	if tx != nil {
		result, err = tx.ExecContext(ctx, query, gameID, winnerID, now)
	} else {
		result, err = r.db.ExecContext(ctx, query, gameID, winnerID, now)
	}
	if err != nil {
		return false, fmt.Errorf("failed to claim winner: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected == 1, nil
}

// AddPlayer adds a card row for a player in a game.
// Keyed on the card (which is UNIQUE per game): if a soft-deleted row already
// exists for this (game, card) — e.g. someone left and the card is being
// re-taken, possibly by a different user — that row is reactivated and
// reassigned. Otherwise a new row is inserted. A player may hold several cards,
// so this never touches the user's other card rows.
func (r *gameRepository) AddPlayer(ctx context.Context, tx *sql.Tx, player *domain.GamePlayer) error {
	player.JoinedAt = time.Now()

	// Reactivate a previously-left row for this exact card, if any.
	updateQuery := `
		UPDATE game_players
		SET user_id = $1, paid = $2, is_eliminated = $3, joined_at = $4, left_at = NULL
		WHERE game_id = $5 AND card_id = $6 AND left_at IS NOT NULL
	`

	var err error
	var result sql.Result
	if tx != nil {
		result, err = tx.ExecContext(ctx, updateQuery, player.UserID, player.Paid, player.IsEliminated, player.JoinedAt, player.GameID, player.CardID)
	} else {
		result, err = r.db.ExecContext(ctx, updateQuery, player.UserID, player.Paid, player.IsEliminated, player.JoinedAt, player.GameID, player.CardID)
	}

	if err != nil {
		return fmt.Errorf("failed to update player: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	// If no left row was reactivated, insert a fresh card row.
	if rowsAffected == 0 {
		insertQuery := `
			INSERT INTO game_players (id, game_id, user_id, card_id, paid, is_eliminated, joined_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`

		if tx != nil {
			_, err = tx.ExecContext(ctx, insertQuery, player.ID, player.GameID, player.UserID, player.CardID, player.Paid, player.IsEliminated, player.JoinedAt)
		} else {
			_, err = r.db.ExecContext(ctx, insertQuery, player.ID, player.GameID, player.UserID, player.CardID, player.Paid, player.IsEliminated, player.JoinedAt)
		}

		if err != nil {
			return fmt.Errorf("failed to add player: %w", err)
		}
	}

	return nil
}

// MarkUserCardsPaidTx flips all of a user's active reserved (unpaid) cards in a
// game to paid, returning how many rows changed. Used when the countdown ends
// and reservations are charged.
func (r *gameRepository) MarkUserCardsPaidTx(ctx context.Context, tx *sql.Tx, gameID, userID uuid.UUID) (int64, error) {
	res, err := tx.ExecContext(ctx, `
		UPDATE game_players
		SET paid = true
		WHERE game_id = $1 AND user_id = $2 AND left_at IS NULL AND paid = false
	`, gameID, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to mark cards paid: %w", err)
	}
	return res.RowsAffected()
}

// MarkCardsBonusFundedTx flags `n` of a user's just-paid cards in a game as
// bought with play-only bonus, stamping the expiry of the grant they consumed.
//
// Which specific cards get the flag does not matter — every card in a game
// costs the same stake — so it takes the n lowest card ids for determinism.
// What matters is that exactly n carry it, because the refund paths use this
// flag to decide whether a stake returns as bonus or as withdrawable cash.
func (r *gameRepository) MarkCardsBonusFundedTx(ctx context.Context, tx *sql.Tx, gameID, userID uuid.UUID, n int, expiresAt time.Time) (int64, error) {
	if n <= 0 {
		return 0, nil
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE game_players
		SET paid_from_bonus = true, bonus_expires_at = $4
		WHERE ctid IN (
			SELECT ctid FROM game_players
			WHERE game_id = $1 AND user_id = $2 AND left_at IS NULL AND paid_from_bonus = false
			ORDER BY card_id
			LIMIT $3
		)
	`, gameID, userID, n, expiresAt)
	if err != nil {
		return 0, fmt.Errorf("failed to mark cards bonus-funded: %w", err)
	}
	return res.RowsAffected()
}

// RemovePlayerCard soft-deletes one specific card row (sets left_at).
func (r *gameRepository) RemovePlayerCard(ctx context.Context, tx *sql.Tx, gameID, userID uuid.UUID, cardID int) (int64, error) {
	query := `
		UPDATE game_players
		SET left_at = $4
		WHERE game_id = $1 AND user_id = $2 AND card_id = $3 AND left_at IS NULL
	`

	leftAt := time.Now()
	var res sql.Result
	var err error
	if tx != nil {
		res, err = tx.ExecContext(ctx, query, gameID, userID, cardID, leftAt)
	} else {
		res, err = r.db.ExecContext(ctx, query, gameID, userID, cardID, leftAt)
	}

	if err != nil {
		return 0, fmt.Errorf("failed to remove player card: %w", err)
	}

	return res.RowsAffected()
}

// FindPlayer finds any one active card row for the user in a game.
func (r *gameRepository) FindPlayer(ctx context.Context, gameID, userID uuid.UUID) (*domain.GamePlayer, error) {
	query := `
		SELECT id, game_id, user_id, card_id, is_eliminated, joined_at, left_at
		FROM game_players
		WHERE game_id = $1 AND user_id = $2 AND left_at IS NULL
		ORDER BY joined_at
		LIMIT 1
	`

	player := &domain.GamePlayer{}
	err := r.db.QueryRowContext(ctx, query, gameID, userID).Scan(
		&player.ID,
		&player.GameID,
		&player.UserID,
		&player.CardID,
		&player.IsEliminated,
		&player.JoinedAt,
		&player.LeftAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("player not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find player: %w", err)
	}

	return player, nil
}

// FindPlayersByUser returns all of a user's active card rows in a game.
func (r *gameRepository) FindPlayersByUser(ctx context.Context, gameID, userID uuid.UUID) ([]*domain.GamePlayer, error) {
	query := `
		SELECT id, game_id, user_id, card_id, paid, is_eliminated, joined_at, left_at, paid_from_bonus, bonus_expires_at
		FROM game_players
		WHERE game_id = $1 AND user_id = $2 AND left_at IS NULL
		ORDER BY joined_at
	`

	rows, err := r.db.QueryContext(ctx, query, gameID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to find user cards: %w", err)
	}
	defer rows.Close()

	players := []*domain.GamePlayer{}
	for rows.Next() {
		player := &domain.GamePlayer{}
		if err := rows.Scan(
			&player.ID,
			&player.GameID,
			&player.UserID,
			&player.CardID,
			&player.Paid,
			&player.IsEliminated,
			&player.JoinedAt,
			&player.LeftAt,
			&player.PaidFromBonus,
			&player.BonusExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan user card: %w", err)
		}
		players = append(players, player)
	}

	return players, nil
}

// FindPlayerCard returns the user's active row for one specific card.
func (r *gameRepository) FindPlayerCard(ctx context.Context, gameID, userID uuid.UUID, cardID int) (*domain.GamePlayer, error) {
	query := `
		SELECT id, game_id, user_id, card_id, is_eliminated, joined_at, left_at
		FROM game_players
		WHERE game_id = $1 AND user_id = $2 AND card_id = $3 AND left_at IS NULL
	`

	player := &domain.GamePlayer{}
	err := r.db.QueryRowContext(ctx, query, gameID, userID, cardID).Scan(
		&player.ID,
		&player.GameID,
		&player.UserID,
		&player.CardID,
		&player.IsEliminated,
		&player.JoinedAt,
		&player.LeftAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("player not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find player card: %w", err)
	}

	return player, nil
}

// CountActiveCardsForUser counts a user's active cards in a game (cap check).
func (r *gameRepository) CountActiveCardsForUser(ctx context.Context, gameID, userID uuid.UUID) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM game_players
		WHERE game_id = $1 AND user_id = $2 AND left_at IS NULL
	`

	var count int
	if err := r.db.QueryRowContext(ctx, query, gameID, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count user cards: %w", err)
	}

	return count, nil
}

// CountDistinctPlayers counts distinct active users in a game (start rule).
func (r *gameRepository) CountDistinctPlayers(ctx context.Context, gameID uuid.UUID) (int, error) {
	query := `
		SELECT COUNT(DISTINCT user_id)
		FROM game_players
		WHERE game_id = $1 AND left_at IS NULL
	`

	var count int
	if err := r.db.QueryRowContext(ctx, query, gameID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count distinct players: %w", err)
	}

	return count, nil
}

// GetPlayers gets all players in a game
func (r *gameRepository) GetPlayers(ctx context.Context, gameID uuid.UUID) ([]*domain.GamePlayer, error) {
	query := `
		SELECT gp.id, gp.game_id, gp.user_id, gp.card_id, gp.paid, gp.is_eliminated,
		       gp.joined_at, gp.left_at, gp.paid_from_bonus, gp.bonus_expires_at, u.is_bot
		FROM game_players gp
		JOIN users u ON u.id = gp.user_id
		WHERE gp.game_id = $1 AND gp.left_at IS NULL
	`

	rows, err := r.db.QueryContext(ctx, query, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get players: %w", err)
	}
	defer rows.Close()

	players := []*domain.GamePlayer{}
	for rows.Next() {
		player := &domain.GamePlayer{}
		err := rows.Scan(
			&player.ID,
			&player.GameID,
			&player.UserID,
			&player.CardID,
			&player.Paid,
			&player.IsEliminated,
			&player.JoinedAt,
			&player.LeftAt,
			&player.PaidFromBonus,
			&player.BonusExpiresAt,
			&player.IsBot,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan player: %w", err)
		}
		players = append(players, player)
	}

	return players, nil
}

// EliminatePlayerCard marks one specific card as eliminated (after a wrong
// claim). The player's other cards are left untouched.
func (r *gameRepository) EliminatePlayerCard(ctx context.Context, tx *sql.Tx, gameID, userID uuid.UUID, cardID int) error {
	query := `
		UPDATE game_players
		SET is_eliminated = TRUE
		WHERE game_id = $1 AND user_id = $2 AND card_id = $3
	`

	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, gameID, userID, cardID)
	} else {
		_, err = r.db.ExecContext(ctx, query, gameID, userID, cardID)
	}

	if err != nil {
		return fmt.Errorf("failed to eliminate player card: %w", err)
	}

	return nil
}

// MarkCardWinner flags one specific card as a winner and records its prize
// share. Runs inside the winner-finalization transaction.
func (r *gameRepository) MarkCardWinner(ctx context.Context, tx *sql.Tx, gameID, userID uuid.UUID, cardID int, prize float64) error {
	query := `
		UPDATE game_players
		SET is_winner = TRUE, prize_won = $4
		WHERE game_id = $1 AND user_id = $2 AND card_id = $3
	`

	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, gameID, userID, cardID, prize)
	} else {
		_, err = r.db.ExecContext(ctx, query, gameID, userID, cardID, prize)
	}
	if err != nil {
		return fmt.Errorf("failed to mark card winner: %w", err)
	}

	return nil
}

// FindWinningCards returns every winning card of a game with its owner's name
// and prize share, ordered by join time then card ID (matching the order used
// when the pot was split). MarkedNumbers is left for the caller to reconstruct
// from the drawn set.
// GetUserWinnings sums the user's prize shares across winning cards — today
// (Ethiopian time, since games are played on Addis wall-clock days) and all
// time. Games finished BEFORE per-card winner tracking (migration 017) have
// winner_id but no flagged cards, so those fall back to the full prize pool —
// the same fallback FindGamesByUserID uses for history rows.
// GetUserGameStats returns a player's lifetime play record in one query.
func (r *gameRepository) GetUserGameStats(ctx context.Context, userID uuid.UUID) (*domain.UserGameStats, error) {
	query := `
		SELECT
			(SELECT count(DISTINCT game_id) FROM game_players WHERE user_id = $1 AND paid) AS played,
			(SELECT count(DISTINCT gid) FROM (
				SELECT gp.game_id AS gid FROM game_players gp JOIN games g ON g.id = gp.game_id
				 WHERE gp.user_id = $1 AND gp.is_winner AND g.state = 'FINISHED'
				UNION
				SELECT g.id FROM games g WHERE g.winner_id = $1 AND g.state = 'FINISHED'
			) w) AS won,
			(SELECT COALESCE(SUM(prize), 0) FROM (
				SELECT gp.prize_won AS prize FROM game_players gp JOIN games g ON g.id = gp.game_id
				 WHERE gp.user_id = $1 AND gp.is_winner AND g.state = 'FINISHED'
				UNION ALL
				SELECT g.prize_pool FROM games g
				 WHERE g.winner_id = $1 AND g.state = 'FINISHED'
				   AND NOT EXISTS (SELECT 1 FROM game_players gp2 WHERE gp2.game_id = g.id AND gp2.is_winner)
			) wins) AS total_won,
			(SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE user_id = $1 AND category = 'bet') AS staked,
			(SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE user_id = $1 AND category = 'deposit' AND status = 'completed') AS deposited,
			(SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE user_id = $1 AND category = 'withdrawal' AND status = 'completed') AS withdrawn,
			(SELECT COALESCE(SUM(amount), 0) FROM bonus_grants WHERE user_id = $1) AS bonus_total,
			(SELECT COALESCE(balance, 0) FROM wallets WHERE user_id = $1) AS real_balance,
			(SELECT COALESCE(SUM(remaining), 0) FROM bonus_grants WHERE user_id = $1 AND remaining > 0 AND expires_at > now()) AS bonus_balance,
			(SELECT count(*) FROM users WHERE referred_by = $1) AS referred_count
	`
	s := &domain.UserGameStats{}
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&s.GamesPlayed, &s.GamesWon, &s.TotalWon, &s.TotalStaked,
		&s.TotalDeposited, &s.TotalWithdrawn, &s.TotalBonus, &s.RealBalance, &s.BonusBalance, &s.ReferredCount,
	); err != nil {
		return nil, fmt.Errorf("failed to get user game stats: %w", err)
	}
	return s, nil
}

func (r *gameRepository) GetUserWinnings(ctx context.Context, userID uuid.UUID) (float64, float64, error) {
	query := `
		WITH wins AS (
			SELECT gp.prize_won AS prize, g.finished_at
			FROM game_players gp
			JOIN games g ON g.id = gp.game_id
			WHERE gp.user_id = $1 AND gp.is_winner AND g.state = 'FINISHED'
			UNION ALL
			SELECT g.prize_pool AS prize, g.finished_at
			FROM games g
			WHERE g.winner_id = $1 AND g.state = 'FINISHED'
			  AND NOT EXISTS (
				SELECT 1 FROM game_players gp2 WHERE gp2.game_id = g.id AND gp2.is_winner
			  )
		)
		SELECT
			COALESCE(SUM(prize) FILTER (
				WHERE (finished_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Addis_Ababa')::date
					= (now() AT TIME ZONE 'Africa/Addis_Ababa')::date
			), 0),
			COALESCE(SUM(prize), 0)
		FROM wins
	`
	var today, total float64
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&today, &total); err != nil {
		return 0, 0, fmt.Errorf("failed to sum winnings: %w", err)
	}
	return today, total, nil
}

func (r *gameRepository) FindWinningCards(ctx context.Context, gameID uuid.UUID) ([]*domain.GameWinner, error) {
	query := `
		SELECT gp.user_id, gp.card_id, gp.prize_won, u.first_name, u.last_name
		FROM game_players gp
		JOIN users u ON u.id = gp.user_id
		WHERE gp.game_id = $1 AND gp.is_winner = TRUE
		ORDER BY gp.joined_at, gp.card_id
	`

	rows, err := r.db.QueryContext(ctx, query, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to find winning cards: %w", err)
	}
	defer rows.Close()

	winners := []*domain.GameWinner{}
	for rows.Next() {
		var w domain.GameWinner
		var firstName string
		var lastName sql.NullString
		if err := rows.Scan(&w.UserID, &w.CardID, &w.Prize, &firstName, &lastName); err != nil {
			return nil, fmt.Errorf("failed to scan winning card: %w", err)
		}
		w.WinnerName = firstName
		if lastName.Valid && lastName.String != "" {
			w.WinnerName = firstName + " " + lastName.String
		}
		winners = append(winners, &w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating winning cards: %w", err)
	}

	return winners, nil
}

// GetTakenCards gets all taken card IDs for a game
func (r *gameRepository) GetTakenCards(ctx context.Context, gameID uuid.UUID) ([]int, error) {
	query := `
		SELECT card_id
		FROM game_players
		WHERE game_id = $1 AND left_at IS NULL
	`

	rows, err := r.db.QueryContext(ctx, query, gameID)
	if err != nil {
		return nil, fmt.Errorf("failed to get taken cards: %w", err)
	}
	defer rows.Close()

	cards := []int{}
	for rows.Next() {
		var cardID int
		if err := rows.Scan(&cardID); err != nil {
			return nil, fmt.Errorf("failed to scan card ID: %w", err)
		}
		cards = append(cards, cardID)
	}

	return cards, nil
}

// SaveDrawnNumber saves a drawn number to the database
func (r *gameRepository) SaveDrawnNumber(ctx context.Context, gameID uuid.UUID, letter string, number int) error {
	query := `
		INSERT INTO drawn_numbers (game_id, letter, number, drawn_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (game_id, letter, number) DO NOTHING
	`

	_, err := r.db.ExecContext(ctx, query, gameID, letter, number, time.Now())
	if err != nil {
		return fmt.Errorf("failed to save drawn number: %w", err)
	}

	return nil
}

// FindGamesByUserID finds all games a user has participated in
func (r *gameRepository) FindGamesByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.GameHistoryEntry, error) {
	// A player may hold several cards in one game, which would otherwise produce
	// duplicate history rows. DISTINCT ON (g.id) collapses each game to a single
	// entry, keeping the most recently joined card as the representative one so
	// pagination counts games, not cards.
	query := `
		SELECT
			id, game_type, state, bet_amount, min_players, player_count,
			prize_pool, house_cut, winner_id, countdown_ends, started_at,
			finished_at, created_at, updated_at,
			card_id, cards_held, is_eliminated, joined_at, left_at,
			user_won, win_amount
		FROM (
			SELECT DISTINCT ON (g.id)
				g.id, g.game_type, g.state, g.bet_amount, g.min_players, g.player_count,
				g.prize_pool, g.house_cut, g.winner_id, g.countdown_ends, g.started_at,
				g.finished_at, g.created_at, g.updated_at,
				gp.card_id, gp.is_eliminated, gp.joined_at, gp.left_at,
				COUNT(*) OVER (PARTITION BY g.id) AS cards_held,
				-- Did any of this user's cards win, and how much did they win in
				-- total (summed across their winning cards after a pot split)?
				-- Fall back to games.winner_id for historical games finished before
				-- per-card winner tracking existed (is_winner/prize_won not backfilled):
				-- there the winner took the whole pool.
				-- COALESCE to FALSE: for a live game winner_id is NULL, so the
				-- winner_id = $1 comparison is NULL and the whole OR collapses to
				-- NULL, which then fails to scan into the Go bool. An unfinished
				-- game simply has no winner.
				COALESCE(bool_or(gp.is_winner) OVER (PARTITION BY g.id) OR g.winner_id = $1, FALSE) AS user_won,
				CASE
					WHEN bool_or(gp.is_winner) OVER (PARTITION BY g.id)
						THEN COALESCE(SUM(gp.prize_won) OVER (PARTITION BY g.id), 0)
					WHEN g.winner_id = $1 THEN g.prize_pool
					ELSE 0
				END AS win_amount
			FROM game_players gp
			INNER JOIN games g ON gp.game_id = g.id
			WHERE gp.user_id = $1
			ORDER BY g.id, gp.joined_at DESC
		) sub
		ORDER BY joined_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to find games by user ID: %w", err)
	}
	defer rows.Close()

	return scanGameHistoryRows(rows)
}

// scanGameHistoryRows scans rows produced by the game-history projection (see
// FindGamesByUserID) into GameHistoryEntry values. Both the paginated history
// and the single active-game lookup select the same columns, so they share
// this scanner.
func scanGameHistoryRows(rows *sql.Rows) ([]*domain.GameHistoryEntry, error) {
	entries := []*domain.GameHistoryEntry{}
	for rows.Next() {
		entry := &domain.GameHistoryEntry{
			Game: &domain.Game{},
		}
		var winnerID sql.NullString
		var countdownEnds sql.NullTime
		var startedAt sql.NullTime
		var finishedAt sql.NullTime
		var leftAt sql.NullTime

		err := rows.Scan(
			&entry.Game.ID,
			&entry.Game.GameType,
			&entry.Game.State,
			&entry.Game.BetAmount,
			&entry.Game.MinPlayers,
			&entry.Game.PlayerCount,
			&entry.Game.PrizePool,
			&entry.Game.HouseCut,
			&winnerID,
			&countdownEnds,
			&startedAt,
			&finishedAt,
			&entry.Game.CreatedAt,
			&entry.Game.UpdatedAt,
			&entry.CardID,
			&entry.CardsHeld,
			&entry.IsEliminated,
			&entry.JoinedAt,
			&leftAt,
			&entry.IsWinner,
			&entry.WinAmount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan game history entry: %w", err)
		}

		// What the player actually spent in this game across all their cards.
		entry.TotalStake = float64(entry.CardsHeld) * entry.Game.BetAmount

		// Handle nullable fields. IsWinner/WinAmount are derived from the per-card
		// winner flags (set above), so co-winners of a split pot are reported
		// correctly even though games.winner_id only records the primary winner.
		if winnerID.Valid {
			parsedID, err := uuid.Parse(winnerID.String)
			if err != nil {
				return nil, fmt.Errorf("failed to parse winner_id: %w", err)
			}
			entry.Game.WinnerID = &parsedID
		}
		if countdownEnds.Valid {
			entry.Game.CountdownEnds = &countdownEnds.Time
		}
		if startedAt.Valid {
			entry.Game.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			entry.Game.FinishedAt = &finishedAt.Time
		}
		if leftAt.Valid {
			entry.LeftAt = &leftAt.Time
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating game history entries: %w", err)
	}

	return entries, nil
}

// FindActiveGameByUserID returns the game the user is currently playing in: one
// they still hold at least one live card in (left_at IS NULL and not eliminated)
// whose state is not yet terminal (WAITING, COUNTDOWN or DRAWING). Returns
// (nil, nil) when the user isn't in any live game. If somehow in more than one,
// the most recently joined is returned so the "return to live game" button lands
// on the newest.
//
// Elimination is per card, so a player holding several cards stays "in a live
// game" until every one of their cards is eliminated. Because the WHERE filters
// out left and eliminated cards, CardsHeld here counts only the cards the player
// still has in play.
func (r *gameRepository) FindActiveGameByUserID(ctx context.Context, userID uuid.UUID) (*domain.GameHistoryEntry, error) {
	query := `
		SELECT
			id, game_type, state, bet_amount, min_players, player_count,
			prize_pool, house_cut, winner_id, countdown_ends, started_at,
			finished_at, created_at, updated_at,
			card_id, cards_held, is_eliminated, joined_at, left_at,
			user_won, win_amount
		FROM (
			SELECT DISTINCT ON (g.id)
				g.id, g.game_type, g.state, g.bet_amount, g.min_players, g.player_count,
				g.prize_pool, g.house_cut, g.winner_id, g.countdown_ends, g.started_at,
				g.finished_at, g.created_at, g.updated_at,
				gp.card_id, gp.is_eliminated, gp.joined_at, gp.left_at,
				COUNT(*) OVER (PARTITION BY g.id) AS cards_held,
				-- COALESCE to FALSE: for a live game winner_id is NULL, so the
				-- winner_id = $1 comparison is NULL and the whole OR collapses to
				-- NULL, which then fails to scan into the Go bool. An unfinished
				-- game simply has no winner.
				COALESCE(bool_or(gp.is_winner) OVER (PARTITION BY g.id) OR g.winner_id = $1, FALSE) AS user_won,
				CASE
					WHEN bool_or(gp.is_winner) OVER (PARTITION BY g.id)
						THEN COALESCE(SUM(gp.prize_won) OVER (PARTITION BY g.id), 0)
					WHEN g.winner_id = $1 THEN g.prize_pool
					ELSE 0
				END AS win_amount
			FROM game_players gp
			INNER JOIN games g ON gp.game_id = g.id
			WHERE gp.user_id = $1
				AND gp.left_at IS NULL
				-- Only count cards still in play. A player may hold several cards;
				-- as long as ONE survives the game is still returnable. The pill
				-- vanishes only once every card the player holds is eliminated.
				AND gp.is_eliminated = FALSE
				AND g.state IN ('WAITING', 'COUNTDOWN', 'DRAWING')
			ORDER BY g.id, gp.joined_at DESC
		) sub
		ORDER BY joined_at DESC
		LIMIT 1
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to find active game for user: %w", err)
	}
	defer rows.Close()

	entries, err := scanGameHistoryRows(rows)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return entries[0], nil
}

// CountGamesByType counts games by type (only completed games: FINISHED or CLOSED)
func (r *gameRepository) CountGamesByType(ctx context.Context) (map[domain.GameType]int, error) {
	query := `
		SELECT game_type, COUNT(*) as count
		FROM games
		WHERE state IN ('FINISHED', 'CLOSED')
		GROUP BY game_type
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count games by type: %w", err)
	}
	defer rows.Close()

	result := make(map[domain.GameType]int)
	for rows.Next() {
		var gameType domain.GameType
		var count int
		if err := rows.Scan(&gameType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan game count: %w", err)
		}
		result[gameType] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating game counts: %w", err)
	}

	return result, nil
}

// GetTotalHouseCut calculates the total house cut from all games
func (r *gameRepository) GetTotalHouseCut(ctx context.Context) (float64, error) {
	// Exclude bot-only games (no real player ever joined): their "house cut" is
	// fake money cycling between house-owned bot wallets, not real revenue.
	query := `
		SELECT COALESCE(SUM(prize_pool * house_cut / (1 - house_cut)), 0) as total_house_cut
		FROM games g
		WHERE g.state IN ('FINISHED', 'CLOSED')
		  AND EXISTS (
		        SELECT 1 FROM game_players gp
		        JOIN users u ON u.id = gp.user_id
		        WHERE gp.game_id = g.id AND u.is_bot = false
		  )
	`

	var totalHouseCut float64
	err := r.db.QueryRowContext(ctx, query).Scan(&totalHouseCut)
	if err != nil {
		return 0, fmt.Errorf("failed to get total house cut: %w", err)
	}

	return totalHouseCut, nil
}

// realGameFilter is the WHERE fragment shared by the house-cut breakdowns —
// finished games that had at least one real (non-bot) player, so bot-only games
// don't inflate revenue.
const realGameFilter = `g.state IN ('FINISHED','CLOSED') AND EXISTS (
	SELECT 1 FROM game_players gp JOIN users u ON u.id = gp.user_id
	WHERE gp.game_id = g.id AND u.is_bot = false)`

// HouseCutByTier breaks the house cut down per game tier (real-player games).
func (r *gameRepository) HouseCutByTier(ctx context.Context) ([]domain.HouseCutTier, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT g.game_type, count(*),
		       COALESCE(SUM(g.prize_pool * g.house_cut / (1 - g.house_cut)), 0)
		FROM games g WHERE `+realGameFilter+`
		GROUP BY g.game_type ORDER BY g.game_type`)
	if err != nil {
		return nil, fmt.Errorf("house cut by tier: %w", err)
	}
	defer rows.Close()
	var out []domain.HouseCutTier
	for rows.Next() {
		var t domain.HouseCutTier
		if err := rows.Scan(&t.Tier, &t.Games, &t.HouseCut); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// HouseCutByDay breaks the house cut down per Ethiopian day for the last `days`.
func (r *gameRepository) HouseCutByDay(ctx context.Context, days int) ([]domain.HouseCutDay, error) {
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT (g.finished_at AT TIME ZONE 'UTC' AT TIME ZONE 'Africa/Addis_Ababa')::date::text AS day,
		       count(*),
		       COALESCE(SUM(g.prize_pool * g.house_cut / (1 - g.house_cut)), 0)
		FROM games g
		WHERE %s AND g.finished_at IS NOT NULL
		  AND g.finished_at > now() - interval '%d days'
		GROUP BY day ORDER BY day DESC`, realGameFilter, days))
	if err != nil {
		return nil, fmt.Errorf("house cut by day: %w", err)
	}
	defer rows.Close()
	var out []domain.HouseCutDay
	for rows.Next() {
		var d domain.HouseCutDay
		if err := rows.Scan(&d.Day, &d.Games, &d.HouseCut); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// FindRecentWinners returns the most recently finished games that had a winner,
// joined to the winner's name, newest first.
func (r *gameRepository) FindRecentWinners(ctx context.Context, limit int) ([]*domain.RecentWinner, error) {
	query := `
		SELECT g.id, g.game_type, g.prize_pool, g.finished_at, u.first_name, u.last_name
		FROM games g
		JOIN users u ON u.id = g.winner_id
		WHERE g.state = 'FINISHED' AND g.winner_id IS NOT NULL AND g.finished_at IS NOT NULL
		ORDER BY g.finished_at DESC
		LIMIT $1
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to find recent winners: %w", err)
	}
	defer rows.Close()

	winners := []*domain.RecentWinner{}
	for rows.Next() {
		var w domain.RecentWinner
		var firstName string
		var lastName sql.NullString
		if err := rows.Scan(&w.GameID, &w.GameType, &w.Prize, &w.FinishedAt, &firstName, &lastName); err != nil {
			return nil, fmt.Errorf("failed to scan recent winner: %w", err)
		}
		w.WinnerName = firstName
		if lastName.Valid && lastName.String != "" {
			w.WinnerName = firstName + " " + lastName.String
		}
		winners = append(winners, &w)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating recent winners: %w", err)
	}

	return winners, nil
}

// CancelEmptyStaleGames cancels WAITING/COUNTDOWN games with no active players
// that haven't been touched since olderThan. Empty games hold no stakes, so no
// refunds are needed. Returns how many rows were cancelled.
func (r *gameRepository) CancelEmptyStaleGames(ctx context.Context, olderThan time.Time) (int64, error) {
	query := `
		UPDATE games g
		SET state = 'CANCELLED',
		    countdown_ends = NULL,
		    player_count = 0,
		    prize_pool = 0,
		    updated_at = CURRENT_TIMESTAMP
		WHERE g.state IN ('WAITING', 'COUNTDOWN')
		  AND g.updated_at < $1
		  AND NOT EXISTS (
		      SELECT 1 FROM game_players gp
		      WHERE gp.game_id = g.id AND gp.left_at IS NULL
		  )
	`
	res, err := r.db.ExecContext(ctx, query, olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to cancel empty stale games: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// TouchUpdatedAt bumps a game's updated_at, protecting a just-served lobby game
// from the empty-game sweeper during the brief join window.
func (r *gameRepository) TouchUpdatedAt(ctx context.Context, id uuid.UUID) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE games SET updated_at = CURRENT_TIMESTAMP WHERE id = $1`, id); err != nil {
		return fmt.Errorf("failed to touch game: %w", err)
	}
	return nil
}

// CountGamesByUserID counts distinct games, not cards, so a multi-card player
// still occupies one row in the paginated admin history.
func (r *gameRepository) CountGamesByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT game_id)
		FROM game_players
		WHERE user_id = $1
	`, userID).Scan(&total); err != nil {
		return 0, fmt.Errorf("failed to count games by user ID: %w", err)
	}
	return total, nil
}

// adminGameWhere builds the full-dataset filters used by the admin game list.
func adminGameWhere(filter domain.AdminGameFilter) (string, []any) {
	clauses := []string{"1=1"}
	args := make([]any, 0, 3)
	add := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	if filter.State != nil {
		clauses = append(clauses, "state = "+add(*filter.State))
	}
	if filter.GameType != nil {
		clauses = append(clauses, "game_type = "+add(*filter.GameType))
	}
	if filter.Active {
		clauses = append(clauses, "state IN ('WAITING', 'COUNTDOWN', 'DRAWING')")
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		placeholder := add("%" + search + "%")
		clauses = append(clauses, `concat_ws(' ', id::text, round_code, game_type::text,
			state::text, bet_amount::text, player_count::text) ILIKE `+placeholder)
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// FindAdmin searches before pagination so an admin can find any matching game.
func (r *gameRepository) FindAdmin(ctx context.Context, filter domain.AdminGameFilter, limit, offset int) ([]*domain.Game, error) {
	where, args := adminGameWhere(filter)
	query := `SELECT id, game_type, state, bet_amount, min_players, player_count,
		prize_pool, house_cut, round_code, winner_id, countdown_ends, started_at,
		finished_at, created_at, updated_at FROM games` + where
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to find admin games: %w", err)
	}
	defer rows.Close()
	games := make([]*domain.Game, 0)
	for rows.Next() {
		game, err := scanGame(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to scan admin game: %w", err)
		}
		games = append(games, game)
	}
	return games, rows.Err()
}

func (r *gameRepository) CountAdmin(ctx context.Context, filter domain.AdminGameFilter) (int, error) {
	where, args := adminGameWhere(filter)
	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM games"+where, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count admin games: %w", err)
	}
	return count, nil
}
