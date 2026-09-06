-- Give filler bots plausible full names.
--
-- Bots used to draw from a 30-name first-name-only list. Players never see a
-- room roster (the game room shows only a participant count), but they do see
-- bot names in the winner overlay and recent-winners list, which are built from
-- "first_name last_name" — and since bots take most wins, that list was the
-- same handful of names on repeat. internal/usecase/bot.go now pairs a 120-name
-- first list with a 49-name surname list; the lengths are coprime, so
-- (index%120, index%49) is unique for the first 5880 bots.
--
-- This backfills bots that already exist (new ones get named at creation).
-- It derives each bot's original creation index from its telegram_id rather
-- than from row order: EnsureBotPool assigns telegram_id = -1000000000 - i for
-- i = 1..pool_size, so i = -1000000000 - telegram_id exactly, and Go indexes
-- both arrays with the zero-based (i - 1). Keying off the id instead of
-- created_at means this reproduces the Go naming exactly, is unaffected by
-- identical creation timestamps, and is safe to re-run.
--
-- KEEP IN SYNC with botFirstNames / botLastNames in internal/usecase/bot.go —
-- the arrays below must match those lists in both content and order, or an
-- existing bot will be renamed differently from a newly created one.

BEGIN;

WITH names AS (
    SELECT
        ARRAY[
            'Abel','Abenezer','Abiy','Addis','Amanuel','Anteneh','Ashenafi',
            'Bereket','Berhanu','Biruk','Bruk','Chala','Dagim','Dagmawi',
            'Daniel','Dawit','Dereje','Desalegn','Elias','Endale','Ephrem',
            'Eyasu','Eyob','Ezra','Fasil','Fikru','Fitsum','Getachew','Girma',
            'Habtamu','Hailu','Henok','Kaleb','Kalab','Kebede','Kidus',
            'Kirubel','Leul','Mekonnen','Melaku','Mesfin','Michael','Mulugeta',
            'Nahom','Natnael','Nebiyu','Robel','Samson','Samuel','Solomon',
            'Surafel','Tadesse','Tamirat','Tesfaye','Tewodros','Tsegaye','Yared',
            'Yeabsira','Yohannes','Yonas','Zelalem','Zerihun','Alemayehu',
            'Bekele','Belay','Ermias','Getnet','Gizachew','Kassahun','Mengistu',
            'Sisay','Wondwosen','Yilma','Abera','Endalkachew','Hana','Kalkidan',
            'Liya','Mekdes','Nardos','Saron','Tigist','Selam','Meron','Rediet',
            'Tsion','Betty','Feven','Helen','Sena','Bezawit','Blen','Eden',
            'Eyerusalem','Firehiwot','Genet','Hiwot','Kidist','Lidya','Mahlet',
            'Marta','Meaza','Mihret','Netsanet','Rahel','Rakeb','Ruth','Sara',
            'Selamawit','Semira','Senait','Seble','Sofia','Tizita','Yordanos',
            'Abeba','Almaz','Aster','Birtukan','Hirut'
        ] AS firsts,
        ARRAY[
            'Tesfaye','Bekele','Alemu','Girma','Hailu','Kebede','Mengistu',
            'Assefa','Tadesse','Wolde','Gebre','Haile','Desta','Abebe',
            'Mulugeta','Getachew','Negash','Teshome','Worku','Yimer','Ayele',
            'Berhe','Demissie','Fikadu','Gizaw','Kassa','Lemma','Mamo',
            'Regassa','Shiferaw','Tilahun','Wondimu','Zeleke','Abera','Adugna',
            'Bayissa','Emiru','Feyisa','Jemal','Kumsa','Melaku','Nigussie',
            'Olana','Tola','Urgessa','Chane','Dida','Sori','Gonfa'
        ] AS lasts
),
targets AS (
    SELECT
        u.id,
        -- Zero-based creation index, mirroring Go's (index - 1).
        (-1000000001 - u.telegram_id)::int AS idx
    FROM users u
    WHERE u.is_bot = true
      -- Only bots whose id follows the synthetic scheme; anything else would
      -- yield a meaningless (possibly negative) index.
      AND u.telegram_id <= -1000000001
)
UPDATE users u
SET first_name = n.firsts[(t.idx % array_length(n.firsts, 1)) + 1],
    last_name  = n.lasts[(t.idx % array_length(n.lasts, 1)) + 1],
    updated_at = CURRENT_TIMESTAMP
FROM targets t, names n
WHERE u.id = t.id
  AND t.idx >= 0;

COMMIT;
