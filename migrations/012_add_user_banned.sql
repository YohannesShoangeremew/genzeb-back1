-- Add a `banned` flag so admins can suspend users. Additive and safe:
-- defaults to false, so all existing users remain active.
ALTER TABLE users
ADD COLUMN IF NOT EXISTS banned BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_users_banned ON users(banned);
