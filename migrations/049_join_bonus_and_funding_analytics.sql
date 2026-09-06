-- Configurable reward for genuinely joining a game. A reservation alone is not
-- enough: the application writes an award only after the player's stake is paid.
-- The composite primary key makes the reward idempotent across retries,
-- countdown restarts, multiple cards, and leave/rejoin attempts.

BEGIN;

ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS join_bonus_enabled BOOLEAN       NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS join_bonus_amount  NUMERIC(10,2) NOT NULL DEFAULT 0
        CHECK (join_bonus_amount >= 0);

CREATE TABLE IF NOT EXISTS game_join_bonus_awards (
    game_id        UUID NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bonus_grant_id UUID NOT NULL UNIQUE,
    amount         NUMERIC(10,2) NOT NULL CHECK (amount > 0),
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (game_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_game_join_bonus_awards_user
    ON game_join_bonus_awards (user_id, created_at DESC);

COMMIT;
