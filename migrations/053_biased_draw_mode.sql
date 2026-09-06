-- Three-state biased draw policy exposed in the admin dashboard:
--   disabled  = ordinary random draw
--   legacy    = former full-hopper draw protecting bonus-funded cards only
--   protected = bot-card-first draw protecting every active human card
--
-- Keep bot_always_win for older API clients. Existing enabled rows map to the
-- current protected implementation; disabled rows remain disabled.

ALTER TABLE bot_config
    ADD COLUMN IF NOT EXISTS biased_draw_mode VARCHAR(16) NOT NULL DEFAULT 'disabled';

UPDATE bot_config
SET biased_draw_mode = CASE
    WHEN bot_always_win THEN 'protected'
    ELSE 'disabled'
END
WHERE biased_draw_mode NOT IN ('disabled', 'legacy', 'protected')
   OR (bot_always_win AND biased_draw_mode = 'disabled');

ALTER TABLE bot_config
    DROP CONSTRAINT IF EXISTS bot_config_biased_draw_mode_check;

ALTER TABLE bot_config
    ADD CONSTRAINT bot_config_biased_draw_mode_check
    CHECK (biased_draw_mode IN ('disabled', 'legacy', 'protected'));

COMMENT ON COLUMN bot_config.biased_draw_mode IS
    'Draw policy: disabled (ordinary), legacy (full hopper/bonus-only protection), or protected (bot-card-first/all-human protection).';
