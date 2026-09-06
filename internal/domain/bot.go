package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// BiasedDrawMode selects the draw policy used when bots participate.
type BiasedDrawMode string

const (
	BiasedDrawModeDisabled  BiasedDrawMode = "disabled"
	BiasedDrawModeLegacy    BiasedDrawMode = "legacy"
	BiasedDrawModeProtected BiasedDrawMode = "protected"
)

// IsValid reports whether mode is supported by the draw engine.
func (m BiasedDrawMode) IsValid() bool {
	switch m {
	case BiasedDrawModeDisabled, BiasedDrawModeLegacy, BiasedDrawModeProtected:
		return true
	default:
		return false
	}
}

// BotConfig is the single-row policy (bot_config table) that drives the
// automatic filler. It is read every sweep and edited from the admin dashboard.
type BotConfig struct {
	Enabled            bool           `json:"enabled" db:"enabled"`                           // master auto-fill switch
	MinRealPlayers     int            `json:"min_real_players" db:"min_real_players"`         // deprecated compatibility field; dynamic staffing no longer uses a join floor
	TargetBots         int            `json:"target_bots" db:"target_bots"`                   // deprecated compatibility field; replaced by minimum_room_players
	MinimumRoomPlayers int            `json:"minimum_room_players" db:"minimum_room_players"` // desired real+bot room size while real players are below this threshold
	Tiers              string         `json:"tiers" db:"tiers"`                               // comma-separated game types to fill, e.g. "REGULAR,VIP"
	WinRate            float64        `json:"win_rate" db:"win_rate"`                         // retained compatibility setting
	BotAlwaysWin       bool           `json:"bot_always_win" db:"bot_always_win"`             // compatibility mirror: true for legacy/protected modes
	BiasedDrawMode     BiasedDrawMode `json:"biased_draw_mode" db:"biased_draw_mode"`         // saved low-population policy: disabled, legacy, or protected
	UpdatedAt          time.Time      `json:"updated_at" db:"updated_at"`
}

// EffectiveBiasedDrawMode returns a valid mode and maps legacy boolean-only
// configs to protected/disabled for rolling-deploy compatibility.
func (c BotConfig) EffectiveBiasedDrawMode() BiasedDrawMode {
	if c.BiasedDrawMode.IsValid() {
		return c.BiasedDrawMode
	}
	if c.BotAlwaysWin {
		return BiasedDrawModeProtected
	}
	return BiasedDrawModeDisabled
}

// NormalizeBiasedDrawMode keeps the legacy boolean synchronized with the
// authoritative three-state mode before configs are returned or persisted.
func (c *BotConfig) NormalizeBiasedDrawMode() {
	if c == nil {
		return
	}
	c.BiasedDrawMode = c.EffectiveBiasedDrawMode()
	c.BotAlwaysWin = c.BiasedDrawMode != BiasedDrawModeDisabled
}

// TierList splits the stored CSV into game types, skipping blanks.
func (c BotConfig) TierList() []GameType {
	out := make([]GameType, 0, 2)
	start := 0
	s := c.Tiers + ","
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			part := s[start:i]
			// trim spaces
			for len(part) > 0 && part[0] == ' ' {
				part = part[1:]
			}
			for len(part) > 0 && part[len(part)-1] == ' ' {
				part = part[:len(part)-1]
			}
			if part != "" {
				out = append(out, GameType(part))
			}
			start = i + 1
		}
	}
	return out
}

// UpdateBotConfigRequest is the admin dashboard payload to change the policy.
// Pointers so an admin can update a single field without resetting the others.
type UpdateBotConfigRequest struct {
	Enabled            *bool           `json:"enabled,omitempty"`
	MinRealPlayers     *int            `json:"min_real_players,omitempty"`
	TargetBots         *int            `json:"target_bots,omitempty"`
	MinimumRoomPlayers *int            `json:"minimum_room_players,omitempty"`
	Tiers              *string         `json:"tiers,omitempty"`
	WinRate            *float64        `json:"win_rate,omitempty"`
	BotAlwaysWin       *bool           `json:"bot_always_win,omitempty"`
	BiasedDrawMode     *BiasedDrawMode `json:"biased_draw_mode,omitempty"`
}

// AddBotsRequest is the admin dashboard payload to manually inject bots into one
// game. Count is capped only by available bot accounts and free cards; automatic
// dynamic staffing is handled separately by the sweeper.
type AddBotsRequest struct {
	Count int `json:"count" binding:"required,min=1"`
}

// BotFillResult reports the outcome of a manual or automatic fill of one game.
type BotFillResult struct {
	GameID      uuid.UUID `json:"game_id"`
	Requested   int       `json:"requested"`
	Added       int       `json:"added"`
	RealPlayers int       `json:"real_players"`
	BotPlayers  int       `json:"bot_players"`
}

// BotRepository serves bot-specific reads and the auto-fill policy. Kept
// separate from UserRepository so the money engine and existing interfaces are
// untouched.
type BotRepository interface {
	// ListBots returns the least-recently-used bot users first, randomizing ties.
	// This rotates the full pool instead of repeatedly showing the same roster.
	ListBots(ctx context.Context, limit int) ([]*User, error)
	// CountBots returns how many bot accounts exist.
	CountBots(ctx context.Context) (int, error)
	// CountRealPlayersInGame counts distinct non-bot users still active in a game.
	CountRealPlayersInGame(ctx context.Context, gameID uuid.UUID) (int, error)
	// CountBotsInGame counts distinct bot users still active in a game.
	CountBotsInGame(ctx context.Context, gameID uuid.UUID) (int, error)
	// HasSpendableBonusPlayerInGame reports whether an active real player has
	// enough live bonus to fund at least one card at the supplied stake.
	HasSpendableBonusPlayerInGame(ctx context.Context, gameID uuid.UUID, stake float64) (bool, error)
	// SecondsSinceFirstRealPlayer reports how long ago the earliest still-active
	// real player joined, and whether the game has one at all. Used to hold bots
	// back for a moment after someone sits down. Computed in the database so it
	// does not depend on the app clock agreeing with Postgres'.
	SecondsSinceFirstRealPlayer(ctx context.Context, gameID uuid.UUID) (float64, bool, error)
	// GetConfig returns the single policy row.
	GetConfig(ctx context.Context) (*BotConfig, error)
	// UpdateConfig persists the policy row.
	UpdateConfig(ctx context.Context, cfg *BotConfig) error
}
