-- Per-campaign bonus expiry.
--
-- WHY. A "first N players" giveaway lives on urgency — "claim it and play
-- tonight" pulls harder than a bonus that sits in the wallet for a week. So a
-- campaign needs its own, potentially much shorter, expiry than the general
-- grant policy, which stays the sensible default for hand-granted bonus.
--
-- UNIT. Stored in MINUTES, not days, because the whole point is sub-day
-- lifetimes (three hours, ninety minutes). Minutes is the smallest unit the
-- admin can pick, so one integer column expresses every choice — the UI turns
-- "3 hours" into 180 and back. bonus_grants already stores a full timestamp
-- (see 030), so nothing downstream needed a schema change to expire in hours.
--
-- NULL means "use the global bonus_config.expiry_days" — a campaign created
-- without an explicit expiry behaves exactly as before this migration, so
-- existing behaviour is unchanged by default.

BEGIN;

ALTER TABLE bonus_campaigns
    ADD COLUMN IF NOT EXISTS expiry_minutes INTEGER
        CHECK (expiry_minutes IS NULL OR expiry_minutes > 0);

COMMIT;
