-- Rename filler bots with female first names to male ones.
--
-- The bot name pool in internal/usecase/bot.go had ~45 female first names in
-- its tail; the client wants all bots to read as male. bot.go now carries male
-- names in those positions, so NEW bots are already male — this renames the
-- bots already stored (backfilled by migration 028).
--
-- Matched by exact first name and is_bot, so no real player is touched. Each
-- female name maps to a distinct male name absent from the male portion of the
-- pool, so re-running is a no-op (no female name remains to match). Surnames
-- were already male father-names and are left as-is. KEEP THE MAPPING BELOW IN
-- SYNC with the same-position replacements in botFirstNames.

BEGIN;

UPDATE users AS u
SET first_name = m.male
FROM (VALUES
    ('Hana','Abraham'), ('Kalkidan','Amare'), ('Liya','Asrat'),
    ('Mekdes','Assefa'), ('Nardos','Ayele'), ('Saron','Bahiru'),
    ('Tigist','Behailu'), ('Selam','Binyam'), ('Meron','Chalachew'),
    ('Rediet','Dagne'), ('Tsion','Damtew'), ('Betty','Demeke'),
    ('Feven','Endeshaw'), ('Helen','Eskinder'), ('Sena','Fekadu'),
    ('Bezawit','Feyisa'), ('Blen','Gebre'), ('Eden','Gemechu'),
    ('Eyerusalem','Getahun'), ('Firehiwot','Habte'), ('Genet','Haile'),
    ('Hiwot','Kefyalew'), ('Kidist','Lemma'), ('Lidya','Mamo'),
    ('Mahlet','Melese'), ('Marta','Mekuria'), ('Meaza','Mitiku'),
    ('Mihret','Moges'), ('Netsanet','Muluken'), ('Rahel','Negash'),
    ('Rakeb','Nigatu'), ('Ruth','Nigus'), ('Sara','Petros'),
    ('Selamawit','Sahle'), ('Semira','Seyoum'), ('Senait','Shimelis'),
    ('Seble','Sintayehu'), ('Sofia','Tariku'), ('Tizita','Tekle'),
    ('Yordanos','Temesgen'), ('Abeba','Teshome'), ('Almaz','Tilahun'),
    ('Aster','Worku'), ('Birtukan','Yidnekachew'), ('Hirut','Zenebe')
) AS m(female, male)
WHERE u.is_bot = true AND u.first_name = m.female;

COMMIT;
