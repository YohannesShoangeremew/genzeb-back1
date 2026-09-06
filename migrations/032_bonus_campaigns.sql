-- First-N claim campaigns: "today's bonus is 1000 birr, for the first 10
-- players who claim it".
--
-- MODEL. A campaign is a fixed number of equal slots, not a pot to be divided
-- at the end. 1000 birr over 10 slots is ten claims of 100, and if only four
-- players turn up the other 600 is simply never granted. Dividing the pot among
-- however many actually claimed would mean nobody can be paid until the
-- campaign closes — the player would press Claim and see nothing, which is
-- precisely the excitement this feature exists to create. Fixed slots also cap
-- the house's exposure at total_amount by construction.
--
-- The award itself is an ordinary bonus_grants row (see 030). A claim is only a
-- ticket that authorises one grant; it invents no new kind of money, so
-- expiry, play-only-ness and the liability figure all keep working untouched.
--
-- CONCURRENCY. The whole point is a race — an announcement goes out and
-- everyone presses Claim at once. Two guarantees are structural here rather
-- than left to application code:
--   * slots cannot be oversold: claimed_count is bounded by a CHECK, and the
--     claim path takes a row lock on the campaign before incrementing, so
--     twenty simultaneous claimers on ten slots produce exactly ten winners.
--   * a player cannot claim twice: PRIMARY KEY (campaign_id, user_id). Even a
--     double-tapped button on a flaky connection can only ever insert once.
-- Neither depends on the application being careful.
--
-- CLOCKS. Timestamps are written by the DATABASE (CURRENT_TIMESTAMP), never
-- passed in from the app, for the reason set out at length in 030: comparing an
-- app-written timestamp against now() silently measures the gap between two
-- clocks whenever the app process is not UTC.

BEGIN;

CREATE TABLE IF NOT EXISTS bonus_campaigns (
    id              UUID PRIMARY KEY,
    -- The advertised pot. Kept even though it equals slots * amount_per_slot,
    -- because it is the number the announcement quotes and the number the
    -- admin typed; deriving it back out for display invites rounding drift.
    total_amount    NUMERIC(10, 2) NOT NULL CHECK (total_amount > 0),
    slots           INTEGER NOT NULL CHECK (slots > 0),
    -- What one claimer receives. Computed once at creation and frozen, so
    -- every claimer in a campaign gets an identical, already-rounded amount.
    amount_per_slot NUMERIC(10, 2) NOT NULL CHECK (amount_per_slot > 0),
    claimed_count   INTEGER NOT NULL DEFAULT 0 CHECK (claimed_count >= 0),
    -- Free text for the Telegram announcement and the in-app banner.
    announcement    TEXT NOT NULL DEFAULT '',
    -- active | ended. A campaign ends when the slots run out or an admin stops
    -- it early; it is never deleted, because the claims reference it and the
    -- history is the audit trail for money given away.
    status          VARCHAR(16) NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'ended')),
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at        TIMESTAMP,
    -- Cannot hand out more slots than exist. The claim path also checks, but
    -- this makes overselling impossible rather than merely unlikely.
    CONSTRAINT bonus_campaigns_within_slots CHECK (claimed_count <= slots)
);

-- At most ONE active campaign at a time.
--
-- Without this, an admin creating today's campaign before ending yesterday's
-- leaves two live: the claim path would have to pick one, every choice is
-- arbitrary, and players would be paid from a campaign nobody announced. A
-- partial unique index makes "the active campaign" a well-defined thing.
CREATE UNIQUE INDEX IF NOT EXISTS idx_bonus_campaigns_one_active
    ON bonus_campaigns ((status)) WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_bonus_campaigns_created
    ON bonus_campaigns (created_at DESC);

-- One row per player per campaign. The PRIMARY KEY is the anti-double-claim
-- rule: it is enforced by the database on insert, so a race, a retry and a
-- double-tap all collapse to a single claim.
CREATE TABLE IF NOT EXISTS bonus_campaign_claims (
    campaign_id UUID NOT NULL REFERENCES bonus_campaigns(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- The grant this claim produced, so an admin can trace an award back to
    -- the campaign that caused it. Nullable only so the column survives a
    -- grant row being cleaned up; the claim itself is the durable record.
    grant_id    UUID REFERENCES bonus_grants(id) ON DELETE SET NULL,
    amount      NUMERIC(10, 2) NOT NULL CHECK (amount > 0),
    -- 1-based position, so the operator can see who was actually first.
    position    INTEGER NOT NULL CHECK (position > 0),
    claimed_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_bonus_campaign_claims_user
    ON bonus_campaign_claims (user_id, claimed_at DESC);

COMMIT;
