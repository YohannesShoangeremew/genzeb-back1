package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bingo/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// GameStateService handles game state in Redis
type GameStateService struct {
	client *redis.Client
}

// NewGameStateService creates a new game state service
func NewGameStateService(client *redis.Client) *GameStateService {
	return &GameStateService{client: client}
}

// acquireOrRenewLeaseScript sets the lease to our token when it is free OR
// already ours, and (re)arms its TTL — an atomic compare-and-set-and-expire so
// two processes can never both believe they own it. Returns 1 when we hold it.
var acquireOrRenewLeaseScript = redis.NewScript(`
	local v = redis.call('get', KEYS[1])
	if v == false or v == ARGV[1] then
		redis.call('set', KEYS[1], ARGV[1], 'PX', ARGV[2])
		return 1
	end
	return 0
`)

// releaseLeaseScript deletes the lease only if we still own it, so a slow
// process can't wipe a lease another instance has since taken over.
var releaseLeaseScript = redis.NewScript(`
	if redis.call('get', KEYS[1]) == ARGV[1] then
		return redis.call('del', KEYS[1])
	end
	return 0
`)

// AcquireOrRenewDrawLease claims the game's draw lease for `token` (or renews it
// if already ours) and arms it for ttl. Returns true only while we own it. When
// Redis is absent it returns true so a single-instance dev setup still draws.
func (s *GameStateService) AcquireOrRenewDrawLease(ctx context.Context, gameID uuid.UUID, token string, ttl time.Duration) (bool, error) {
	if s.client == nil {
		return true, nil
	}
	res, err := acquireOrRenewLeaseScript.Run(ctx, s.client,
		[]string{GameDrawLeaseKey(gameID.String())}, token, ttl.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

// ReleaseDrawLease frees the game's draw lease if we still hold it. Best-effort.
func (s *GameStateService) ReleaseDrawLease(ctx context.Context, gameID uuid.UUID, token string) {
	if s.client == nil {
		return
	}
	_ = releaseLeaseScript.Run(ctx, s.client, []string{GameDrawLeaseKey(gameID.String())}, token).Err()
}

// MarkTierBrowsed records that a real player just opened this tier's lobby,
// keeping it "recently browsed" for ttl. Called on every real lobby fetch; the
// filler bots read it (TierBrowsedRecently) to decide whether to run games with
// zero real players. No-op (nil error) when Redis is not configured.
func (s *GameStateService) MarkTierBrowsed(ctx context.Context, tier string, ttl time.Duration) error {
	if s.client == nil {
		return nil
	}
	return s.client.Set(ctx, LobbyActivityKey(tier), "1", ttl).Err()
}

// TierBrowsedRecently reports whether a real player has opened this tier's lobby
// within the activity window (i.e. the marker key still exists). Returns false
// when Redis is unavailable, so a Redis outage makes the bots idle rather than
// churn empty games unattended.
func (s *GameStateService) TierBrowsedRecently(ctx context.Context, tier string) (bool, error) {
	if s.client == nil {
		return false, nil
	}
	n, err := s.client.Exists(ctx, LobbyActivityKey(tier)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// SaveGameState saves game state to Redis
func (s *GameStateService) SaveGameState(ctx context.Context, game *domain.Game) error {
	if s.client == nil {
		return fmt.Errorf("Redis client is not configured")
	}
	key := GameStateKey(game.ID.String())

	data, err := json.Marshal(game)
	if err != nil {
		return fmt.Errorf("failed to marshal game: %w", err)
	}

	return s.client.Set(ctx, key, data, 24*time.Hour).Err()
}

// GetGameState gets game state from Redis
func (s *GameStateService) GetGameState(ctx context.Context, gameID uuid.UUID) (*domain.Game, error) {
	if s.client == nil {
		return nil, fmt.Errorf("Redis client is not configured")
	}
	key := GameStateKey(gameID.String())

	data, err := s.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("game state not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get game state: %w", err)
	}

	var game domain.Game
	if err := json.Unmarshal(data, &game); err != nil {
		return nil, fmt.Errorf("failed to unmarshal game: %w", err)
	}

	return &game, nil
}

// AddPlayer adds a player to Redis set
func (s *GameStateService) AddPlayer(ctx context.Context, gameID uuid.UUID, userID uuid.UUID) error {
	key := GamePlayersKey(gameID.String())
	return s.client.SAdd(ctx, key, userID.String()).Err()
}

// RemovePlayer removes a player from Redis set
func (s *GameStateService) RemovePlayer(ctx context.Context, gameID uuid.UUID, userID uuid.UUID) error {
	key := GamePlayersKey(gameID.String())
	return s.client.SRem(ctx, key, userID.String()).Err()
}

// GetPlayerCount gets the number of players
func (s *GameStateService) GetPlayerCount(ctx context.Context, gameID uuid.UUID) (int64, error) {
	if s.client == nil {
		return 0, fmt.Errorf("Redis client is not configured")
	}
	key := GamePlayersKey(gameID.String())
	return s.client.SCard(ctx, key).Result()
}

// IsPlayerInGame checks if a player is in the game
func (s *GameStateService) IsPlayerInGame(ctx context.Context, gameID, userID uuid.UUID) (bool, error) {
	if s.client == nil {
		return false, fmt.Errorf("Redis client is not configured")
	}
	key := GamePlayersKey(gameID.String())
	return s.client.SIsMember(ctx, key, userID.String()).Result()
}

// AddDrawnNumber adds a drawn number to Redis list
func (s *GameStateService) AddDrawnNumber(ctx context.Context, gameID uuid.UUID, number domain.DrawnNumber) error {
	key := GameDrawnNumbersKey(gameID.String())

	data, err := json.Marshal(number)
	if err != nil {
		return fmt.Errorf("failed to marshal drawn number: %w", err)
	}

	return s.client.RPush(ctx, key, data).Err()
}

// GetDrawnNumbers gets all drawn numbers
func (s *GameStateService) GetDrawnNumbers(ctx context.Context, gameID uuid.UUID) ([]domain.DrawnNumber, error) {
	if s.client == nil {
		return nil, fmt.Errorf("Redis client is not configured")
	}
	key := GameDrawnNumbersKey(gameID.String())

	data, err := s.client.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get drawn numbers: %w", err)
	}

	numbers := make([]domain.DrawnNumber, 0, len(data))
	for _, item := range data {
		var number domain.DrawnNumber
		if err := json.Unmarshal([]byte(item), &number); err != nil {
			continue
		}
		numbers = append(numbers, number)
	}

	return numbers, nil
}

// AddTakenCard adds a taken card ID to Redis set
func (s *GameStateService) AddTakenCard(ctx context.Context, gameID uuid.UUID, cardID int) error {
	key := GameTakenCardsKey(gameID.String())
	return s.client.SAdd(ctx, key, cardID).Err()
}

// RemoveTakenCard removes a taken card ID from Redis set
func (s *GameStateService) RemoveTakenCard(ctx context.Context, gameID uuid.UUID, cardID int) error {
	if s.client == nil {
		return fmt.Errorf("Redis client is not configured")
	}
	key := GameTakenCardsKey(gameID.String())
	return s.client.SRem(ctx, key, cardID).Err()
}

// GetTakenCards gets all taken card IDs
func (s *GameStateService) GetTakenCards(ctx context.Context, gameID uuid.UUID) ([]int, error) {
	if s.client == nil {
		return nil, fmt.Errorf("Redis client is not configured")
	}
	key := GameTakenCardsKey(gameID.String())

	cardIDs, err := s.client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get taken cards: %w", err)
	}

	cards := make([]int, 0, len(cardIDs))
	for _, idStr := range cardIDs {
		var cardID int
		if _, err := fmt.Sscanf(idStr, "%d", &cardID); err == nil {
			cards = append(cards, cardID)
		}
	}

	return cards, nil
}

// SetCountdown sets the countdown end time
func (s *GameStateService) SetCountdown(ctx context.Context, gameID uuid.UUID, endsAt time.Time) error {
	key := GameCountdownKey(gameID.String())
	return s.client.Set(ctx, key, endsAt.Unix(), 2*time.Minute).Err()
}

// GetCountdown gets the countdown end time
func (s *GameStateService) GetCountdown(ctx context.Context, gameID uuid.UUID) (time.Time, error) {
	if s.client == nil {
		return time.Time{}, fmt.Errorf("Redis client is not configured")
	}
	key := GameCountdownKey(gameID.String())

	seconds, err := s.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return time.Time{}, fmt.Errorf("countdown not found")
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get countdown: %w", err)
	}

	return time.Unix(seconds, 0), nil
}

// ClearCountdown clears the countdown from Redis
func (s *GameStateService) ClearCountdown(ctx context.Context, gameID uuid.UUID) error {
	if s.client == nil {
		return fmt.Errorf("Redis client is not configured")
	}
	key := GameCountdownKey(gameID.String())
	return s.client.Del(ctx, key).Err()
}

// PublishEvent publishes a game event to Redis pub/sub
func (s *GameStateService) PublishEvent(ctx context.Context, gameID uuid.UUID, event string, data interface{}) error {
	channel := GameChannel(gameID.String())

	message := map[string]interface{}{
		"event": event,
		"data":  data,
	}

	messageJSON, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	return s.client.Publish(ctx, channel, messageJSON).Err()
}

// DeleteGameState deletes all game state from Redis
func (s *GameStateService) DeleteGameState(ctx context.Context, gameID uuid.UUID) error {
	keys := []string{
		GameStateKey(gameID.String()),
		GamePlayersKey(gameID.String()),
		GameDrawnNumbersKey(gameID.String()),
		GameTakenCardsKey(gameID.String()),
		GameCountdownKey(gameID.String()),
	}

	for _, key := range keys {
		if err := s.client.Del(ctx, key).Err(); err != nil {
			return fmt.Errorf("failed to delete key %s: %w", key, err)
		}
	}

	return nil
}
