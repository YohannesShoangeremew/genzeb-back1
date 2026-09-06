-- Allow a player to hold multiple cards in one game (up to 4, enforced in app).
-- Reverses the one-card-per-player rule from migration 011 by dropping the
-- UNIQUE(game_id, user_id) constraint. Each card a player buys is its own
-- game_players row.
--
-- The UNIQUE(game_id, card_id) constraint is intentionally KEPT: a given card
-- can still only be held by one player in a game (no two people on one card).
-- The 4-cards-per-player cap is enforced in the use case, not here.

ALTER TABLE game_players DROP CONSTRAINT IF EXISTS game_players_game_id_user_id_key;
