-- Admin-visible operational warnings. These rows mirror selected server logs
-- that operators need inside the dashboard instead of only in stdout.

CREATE TABLE IF NOT EXISTS admin_event_logs (
    id         UUID PRIMARY KEY,
    level      TEXT NOT NULL,
    source     TEXT NOT NULL,
    message    TEXT NOT NULL,
    game_id    UUID REFERENCES games(id) ON DELETE SET NULL,
    metadata   JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_admin_event_logs_created_at ON admin_event_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_admin_event_logs_level_created ON admin_event_logs (level, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_admin_event_logs_source_created ON admin_event_logs (source, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_admin_event_logs_game_id ON admin_event_logs (game_id);
