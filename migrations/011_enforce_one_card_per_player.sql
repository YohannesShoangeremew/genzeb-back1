-- Enforce one card per player per game (canonical decision).
-- This reverses migration 005 (which allowed duplicate cards). Idempotent:
-- only adds the UNIQUE(game_id, card_id) constraint if it is missing, and
-- only if no duplicate (game_id, card_id) rows currently exist.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'game_players'::regclass
          AND contype = 'u'
          AND conname = 'game_players_game_id_card_id_key'
    ) THEN
        ALTER TABLE game_players
            ADD CONSTRAINT game_players_game_id_card_id_key UNIQUE (game_id, card_id);
        RAISE NOTICE 'Added UNIQUE(game_id, card_id)';
    ELSE
        RAISE NOTICE 'UNIQUE(game_id, card_id) already present';
    END IF;
END $$;
