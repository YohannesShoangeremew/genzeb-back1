-- Backfill house-controlled filler bots with Telegram-style display names.
--
-- New bots use botDisplayNames in internal/usecase/bot.go. This migration keeps
-- already-seeded bots in sync by deriving the deterministic bot index from the
-- synthetic negative telegram_id assigned by EnsureBotPool:
-- telegram_id = -1000000000 - i, so idx = i - 1.
--
-- The users table has no separate Telegram username/display-name column. Winner
-- overlays and recent-winner feeds render first_name plus last_name when set, so
-- each Telegram-style name is stored as first_name and last_name is cleared.

BEGIN;

WITH names AS (
    SELECT ARRAY[
    'Natiye', 'beki', 'B', 'bekaaaaaaaaa', 'Solitude',
    'Godsproperty', 'maki', 'Beka', 'Kiya', 'Babi',
    'Yadi', 'TemesgenYa', 'N', 'natiiii', 'Eclipse',
    'YaAllah', 'Moges', 'Micky', 'Mack', 'Abela',
    'Fikir', 'yabs', 'Elsa', 'edu', 'Peace',
    'His', 'B', 'Abet', 'Noah', 'Kikiyeee',
    'Abi', 'makgreat', 'J', 'hanicho', 'Chaos',
    'Grace', 'Ghosttt', 'Tew', 'Beka', 'Mimi',
    'habib', 'eduta', 'K', 'yabsss', 'Loner',
    'Amen', 'Edu', 'Min', 'Sami', 'Nani',
    'Mela', 'hani', 'D', 'sammyy', 'Vibes',
    'Chosen', 'Kira', 'Awo', 'Nati', 'Tuti',
    'Tseda', 'sami', 'M', 'makkk', 'Aura',
    'Gospel', 'yabs', 'Ishi', 'Hani', 'Jiji',
    'Beki', 'beti', 'H', 'abiii', 'Moonchild',
    'Jesus', 'miki', 'Kiya', 'Edu', 'Fifi',
    'Robi', 'chala', 'Y', 'fikirrr', 'Soul',
    'Faith', 'hani', 'Ere', 'Yoni', 'Chuchu',
    'Daveeeeee', 'muni', 'elias', 'lilii', 'Broken',
    'Marys', 'beti', 'Gida', 'Dani', 'Didi',
    'Yoni', 'nami', 'A', 'danii', 'Faded',
    'Alhamdulillah', 'jo', 'Ebak', 'Fikir', 'Bobo',
    'Kaleb', 'rozi', 'T', 'robb', 'Ghost',
    'Sabr', 'solo', 'Tik', 'Selam', 'Mamo',
    'Fema', 'meron', 'R', 'jerryy', 'Silence',
    'Tawakkul', 'ezi', 'Fen', 'Tsega', 'Jojo',
    'ezra', 'yerus', 'L', 'tutuu', 'Empty',
    'Blessed', 'dan', 'Lemen', 'Geta', 'Nono',
    'Bruk', 'mina', 'P', 'eziii', 'Void',
    'Redeemed', 'rob', 'Man', 'Rahmet', 'Pipi',
    'Safi', 'tsed', 'C', 'fiooo', 'Nothing',
    'Saved', 'fik', 'Hul', 'Juma', 'Toto',
    'Elu', 'yofe', 'V', 'soll', 'Shadow',
    'Holy', 'lid', 'Chil', 'Hiwot', 'Lolo',
    'Jeri', 'abi', 'Z', 'babb', 'Bliss',
    'Pray', 'na', 'Wey', 'Tesfa', 'Yoyo',
    'Redi', 'meze', 'G', 'garii', 'Serenity',
    'Lord', 'kal', 'Ged', 'Birhan', 'Gogo',
    'Bogi', 'zed', 'F', 'noahh', 'Lost',
    'Christ', 'bru', 'Zim', 'Kal', 'Shushu',
    'Miki', 'jappy', 'T', 'sidd', 'Tired',
    'Saint', 'sel', 'Besm', 'Emnet', 'Kuku',
    'Seli', 'lela', 'W', 'yemii', 'Enigma',
    'Peace', 'mel', 'Kef', 'Tsehay', 'Maca',
    'Lina', 'eba', 'Q', 'farr', 'Abyss',
    'Mercy', 'Fitse', 'Are', 'Chereka', 'Papa',
    'Miky', 'tofik', 'X', 'AB', 'Mirage',
    'Heaven', 'Abenezer', 'Dagmawi', 'Kiyaye', 'Daniel',
    'dagi', 'jossi', 'Kirubel', 'kiraa', 'Echo',
    'Angel', 'Father', 'Ende', 'Semay', 'Coco',
    'Fasilo', 'Yichalal', 'Demelash', 'lderu', 'Chill',
    'Prophet', 'Dereje', 'Dere', 'Midir', 'Zizu',
    'Hena', 'hiwi', 'User', 'titi', 'Dawn',
    'Nour', 'Bomb', 'Tel', 'Zimta', 'Ririfikir',
    'Amu', 'mack', 'U', 'Timiro', 'Dusk',
    'Deen', 'Bella', 'ErmiyasAsa', 'Ewnet', 'Davinchi',
    'YonasG', 'yordi', 'K', 'mariiii', 'Luna',
    'Glory', 'Goytom', 'Abrish', 'Sewmalet', 'Meseret',
    'Abdulselam', 'Abdi', 'Kedir', 'babyyyy', 'Starboy',
    'MyLove', 'Yabsera', 'Faya', 'Nuro', 'ElaBest'
] AS display_names
),
targets AS (
    SELECT
        u.id,
        (-1000000001 - u.telegram_id)::int AS idx
    FROM users u
    WHERE u.is_bot = true
      AND u.telegram_id <= -1000000001
)
UPDATE users u
SET first_name = n.display_names[(t.idx % array_length(n.display_names, 1)) + 1],
    last_name = NULL,
    updated_at = CURRENT_TIMESTAMP
FROM targets t, names n
WHERE u.id = t.id
  AND t.idx >= 0;

COMMIT;
