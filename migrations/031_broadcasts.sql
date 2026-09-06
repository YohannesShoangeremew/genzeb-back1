-- Telegram broadcasts: one admin message pushed to every registered player.
--
-- WHY A TABLE AND NOT JUST A LOOP. Telegram caps bulk sending at roughly 30
-- messages a second, so a broadcast to a few thousand players takes minutes.
-- That cannot live inside an HTTP request: the admin would stare at a spinner
-- and a timeout or a page refresh would leave them with no idea how far it
-- got, or whether re-submitting would double-send. The row is created up
-- front, the sending runs in the background, and the admin polls this for
-- progress.
--
-- sent/failed are updated as the run proceeds, so a broadcast interrupted by a
-- deploy leaves an honest partial record rather than looking untouched.
-- `status` distinguishes a run still going from one that finished, and from
-- one that died mid-flight (left as 'sending' with a stale updated_at).

BEGIN;

CREATE TABLE IF NOT EXISTS broadcasts (
    id          UUID PRIMARY KEY,
    message     TEXT NOT NULL CHECK (length(trim(message)) > 0),
    -- Who to. Recorded so a later audit can tell what the run actually
    -- targeted, rather than inferring it from today's user table.
    recipients  INTEGER NOT NULL DEFAULT 0,
    sent        INTEGER NOT NULL DEFAULT 0,
    failed      INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'sending'
                CHECK (status IN ('sending', 'completed', 'failed')),
    created_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_broadcasts_created_at ON broadcasts (created_at DESC);

COMMIT;
