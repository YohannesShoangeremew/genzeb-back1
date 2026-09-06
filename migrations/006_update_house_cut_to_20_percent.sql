-- Update house_cut to 20% (0.2) for all active games (WAITING and COUNTDOWN)
-- Only update games that haven't started yet to avoid affecting games in progress
UPDATE games
SET house_cut = 0.2,
    updated_at = CURRENT_TIMESTAMP
WHERE state IN ('WAITING', 'COUNTDOWN')
  AND house_cut != 0.2;

-- Note: Games in DRAWING, FINISHED, CLOSED, or CANCELLED states are not updated
-- to preserve the house_cut that was used when those games were active.


