-- Canonicalize stored user phone numbers to 251XXXXXXXXX.
--
-- Registration used to store utils.NormalizePhoneNumber(phone), which strips
-- non-digits and then strips LEADING ZEROS — so "0911223344" was stored as
-- "911223344". Login and the withdrawal payout path use
-- CanonicalEthiopianPhone, which produces "251911223344". The two never match,
-- so such an account could not be found by phone at login and the duplicate
-- check at registration silently missed it. The code now uses
-- CanonicalEthiopianPhone everywhere; this brings existing rows in line.
--
-- Shapes seen in the wild: "251911223344" (Telegram, already canonical),
-- "+251911223344" (kept its plus), and "911223344" (leading zero stripped).
--
-- SAFETY:
--   * Bots are excluded — their phone_number is "BOT-00000001", and stripping
--     non-digits would turn that into a meaningless numeric string.
--   * Rows whose digits do not form a recognizable Ethiopian mobile are left
--     untouched rather than mangled.
--   * phone_number is UNIQUE. If two rows would canonicalize to the same value
--     (e.g. both "+251911223344" and "911223344" exist for different users)
--     the pre-check below aborts the whole migration and names them, instead
--     of letting the UPDATE fail halfway or silently pick a winner.
--
-- Idempotent: rows already canonical are not updated.

BEGIN;

-- Canonical form for every non-bot user, or NULL when unrecognizable.
CREATE TEMP TABLE _phone_fix ON COMMIT DROP AS
WITH digits AS (
    SELECT
        u.id,
        u.phone_number AS current_phone,
        regexp_replace(u.phone_number, '\D', '', 'g') AS d
    FROM users u
    WHERE u.is_bot = false
)
SELECT
    id,
    current_phone,
    CASE
        WHEN d LIKE '251%' AND length(d) = 12 THEN d
        WHEN left(d, 1) = '0' AND length(d) = 10 THEN '251' || substr(d, 2)
        WHEN length(d) = 9                    THEN '251' || d
        ELSE NULL
    END AS canonical
FROM digits;

-- Abort loudly if canonicalizing would collide on the UNIQUE index.
DO $$
DECLARE
    collision text;
BEGIN
    SELECT string_agg(canonical || ' (' || n || ' users)', ', ')
    INTO collision
    FROM (
        SELECT canonical, count(*) AS n
        FROM _phone_fix
        WHERE canonical IS NOT NULL
        GROUP BY canonical
        HAVING count(*) > 1
    ) dupes;

    IF collision IS NOT NULL THEN
        RAISE EXCEPTION
            'phone canonicalization would violate users_phone_number_key: %. Resolve these duplicate accounts by hand, then re-run.',
            collision;
    END IF;
END $$;

UPDATE users u
SET phone_number = f.canonical,
    updated_at   = CURRENT_TIMESTAMP
FROM _phone_fix f
WHERE u.id = f.id
  AND f.canonical IS NOT NULL
  AND f.canonical <> f.current_phone
  -- Only real Ethiopian mobiles: the subscriber part must start 9 (Ethio
  -- Telecom) or 7 (Safaricom Ethiopia).
  AND substr(f.canonical, 4, 1) IN ('9', '7');

COMMIT;
