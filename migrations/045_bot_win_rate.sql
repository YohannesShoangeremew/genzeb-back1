-- Bot win-rate + fairness mode: adds a configurable probability and an
-- on/off toggle to the bot_config policy row that the auto-filler reads each
-- sweep. When both bots and humans hold a valid bingo at the same time the
-- engine rolls against win_rate (or honours the toggle if bot_always_win is
-- enabled): the draw is also biased toward numbers that complete a bot bingo
-- when fairness mode is ON.

ALTER TABLE bot_config
    ADD COLUMN IF NOT EXISTS win_rate NUMERIC(3,2) NOT NULL DEFAULT 0.8,
    ADD COLUMN IF NOT EXISTS bot_always_win BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN bot_config.win_rate IS
    'Probability (0..1) that bots win co-winner situations. 1 = bots always win when they have a bingo alongside humans.';

COMMENT ON COLUMN bot_config.bot_always_win IS
    'When true, the draw engine favours numbers that complete a bot bingo and bots take every co-winner pot.';
