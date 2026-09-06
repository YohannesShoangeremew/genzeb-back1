-- Dynamic room staffing: bots fill the gap to an admin-controlled total.
-- The older min_real_players/target_bots columns remain for rolling-client
-- compatibility but are no longer used by the automatic sweeper.

ALTER TABLE bot_config
    ADD COLUMN IF NOT EXISTS minimum_room_players INTEGER NOT NULL DEFAULT 20;

UPDATE bot_config
SET minimum_room_players = 20
WHERE minimum_room_players < 2 OR minimum_room_players > 200;

ALTER TABLE bot_config
    DROP CONSTRAINT IF EXISTS bot_config_minimum_room_players_check;

ALTER TABLE bot_config
    ADD CONSTRAINT bot_config_minimum_room_players_check
    CHECK (minimum_room_players BETWEEN 2 AND 200);

COMMENT ON COLUMN bot_config.minimum_room_players IS
    $$Desired real+bot lobby size. Below it bots fill the gap; at/above it the saved bot-winning policy is suspended.$$;

COMMENT ON COLUMN bot_config.min_real_players IS
    $$Deprecated compatibility field. Dynamic staffing supports zero-real bot seeding without a join floor.$$;

COMMENT ON COLUMN bot_config.target_bots IS
    $$Deprecated compatibility field. Replaced by minimum_room_players.$$;
