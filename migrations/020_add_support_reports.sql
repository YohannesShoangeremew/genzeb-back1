-- Player-submitted problem reports ("Report a problem" / contact support). A
-- player who hits an issue — a stuck deposit/withdrawal, the caller voice not
-- playing, a wrong call, lag — files a report from the Mini App; admins triage
-- and resolve them from the dashboard. Dashboard-only: no Telegram forwarding.
-- See internal/{domain,usecase,handler}/support.go.

CREATE TABLE IF NOT EXISTS support_reports (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Who filed it. Taken from the JWT server-side (never the request body), so
    -- it can't be spoofed. Cascade: if the account is deleted, its reports go.
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- What kind of problem, so admins can triage. Mirrors the Mini App picker.
    category    TEXT NOT NULL CHECK (category IN ('transaction', 'gameplay', 'other')),
    -- The player's description of the problem.
    message     TEXT NOT NULL,
    -- The game the player was in when reporting, if any. SET NULL so cleaning up
    -- an old game never deletes the report or blocks the cleanup.
    game_id     UUID REFERENCES games(id) ON DELETE SET NULL,
    -- Triage state. Admins flip 'open' -> 'resolved'.
    status      TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved_at TIMESTAMP,
    -- The admin who resolved it (users.id). SET NULL if that admin is removed.
    resolved_by UUID REFERENCES users(id) ON DELETE SET NULL
);

-- The dashboard lists newest-first, usually filtered to open. Index the columns
-- that drives that read.
CREATE INDEX IF NOT EXISTS idx_support_reports_status_created
    ON support_reports(status, created_at DESC);

-- A player's own report history (and cascade deletes) look up by user.
CREATE INDEX IF NOT EXISTS idx_support_reports_user
    ON support_reports(user_id);
