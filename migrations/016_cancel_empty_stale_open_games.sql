-- Hide stale open games left behind by the G1-G7 -> REGULAR/VIP migration.
--
-- Current live tiers are REGULAR=10 and VIP=50. Older WAITING/COUNTDOWN rows
-- can still have old bet amounts (for example REGULAR=5 or VIP=200), and those
-- rows must not be offered to new players because the deployed frontend prices
-- the current tiers.
--
-- This migration only cancels empty open games. Games with active cards are
-- intentionally left untouched so balances are not changed outside the game
-- refund path; the backend query now excludes them from "available games".

UPDATE games g
SET
    state = 'CANCELLED',
    countdown_ends = NULL,
    player_count = 0,
    prize_pool = 0,
    updated_at = CURRENT_TIMESTAMP
WHERE g.state IN ('WAITING', 'COUNTDOWN')
  AND (
        (g.game_type = 'REGULAR' AND g.bet_amount <> 10)
     OR (g.game_type = 'VIP' AND g.bet_amount <> 50)
  )
  AND NOT EXISTS (
      SELECT 1
      FROM game_players gp
      WHERE gp.game_id = g.id
        AND gp.left_at IS NULL
  );
