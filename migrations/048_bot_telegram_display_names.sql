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
            '`N`a`t`i','b e k i .','𝑩','b e k k k',
            '𝘚𝘰𝘭𝘪𝘵𝘶𝘥𝘦','God''s property','_m.a.k.i_','B e k a',
            'ኪያ','B a b i','`Y`a`d`i','n a t i .',
            '• 𝘕 •','n a t i i i i','𝘌𝘤𝘭𝘪𝘱𝘴𝘦','𝘠𝘢 𝘈𝘭𝘭𝘢𝘩',
            '✞ 𝘕 𝘢 𝘵 𝘪 ✞','E n j a','ማክ','M a m u s h',
            '`F`i`k`i`r`','y a b s','♔ 𝘚 ♔','e d u u u',
            '𝘗𝘦𝘢𝘤𝘦','His','☾ 𝘉 ☽','A b e t',
            'ኑ','K i k i','`A`b`i`','m a k i _',
            '𝙅 .','h a n i i','𝘊𝘩𝘢𝘰𝘴','𝘎𝘳𝘢𝘤𝘦',
            '☠︎︎ 𝘚 𝘢 𝘮 ☠︎︎','T e w','ቤካ','M i m i',
            '`H`a`b`i','e d u','𝒦','y a b s s s',
            '𝘓𝘰𝘯𝘦𝘳','𝘈𝘮𝘦𝘯','❀ 𝘌 𝘥 𝘶 ❀','M i n',
            'ሳሚ','N a n i','`M`e`l`a','h a n i .',
            '𝕯','s a m m y y','𝘝𝘪𝘣𝘦𝘴','𝘊𝘩𝘰𝘴𝘦𝘯',
            '✰ 𝘒 𝘪 𝘳 𝘢 ✰','A w o','ናቲ','T u t i',
            '`T`s`e`d`a`','s a m i','𝔐','m a k k k',
            '𝘈𝘶𝘳𝘢','𝘎𝘰𝘴𝘱𝘦𝘭','• 𝘺 𝘢 𝘣 𝘴 •','I s h i',
            'ሃኒ','J i j i','`B`e`k`i`','b e t i .',
            'ℋ','a b i i i','𝘔𝘰𝘰𝘯𝘤𝘩𝘪𝘭𝘥','𝘑𝘦𝘴𝘶𝘴',
            '~ 𝘮 𝘪 𝘬 𝘪 ~','K i y a','ኤዱ','F i f i',
            '`R`o`b`i','c h a l a','𝒴','f i k i r r r',
            '𝘚𝘰𝘶𝘭','𝘍𝘢𝘪𝘵𝘩','[ 𝘩 𝘢 𝘯 𝘪 ]','E r e',
            'ዮኒ','C h u c h u','`D`a`w`i`t','m u n i',
            'ℰ','l i l i i','𝘉𝘳𝘰𝘬𝘦𝘯','𝘔𝘢𝘳𝘺''𝘴',
            '{ 𝘣 𝘦 𝘵 𝘪 }','G i d a','ዳኒ','D i d i',
            '`Y`o`n`i`','n a m i','♚ 𝘼 ♚','d a n i i',
            '𝘍𝘢𝘥𝘦𝘥','𝘈𝘭𝘩𝘢𝘮𝘥𝘶𝘭𝘪𝘭𝘭𝘢𝘩','♔ 𝘫 𝘰 ♔','E b a k',
            'ፍቅር','B o b o','`K`a`l`e`b','r o z i',
            '✞ 𝘛 ✞','r o b b','𝘎𝘩𝘰𝘴𝘵','𝘚𝘢𝘣𝘳',
            '☼ 𝘴 𝘰 𝘭 ☼','T i k','ሰላም','M o m o',
            '`F`e`n`a','m e r o n','𝘙','j e r r y y',
            '𝘚𝘪𝘭𝘦𝘯𝘤𝘦','𝘛𝘢𝘸𝘢𝘬𝘬𝘶𝘭','✧ 𝘦 𝘻 𝘪 ✧','F e n',
            'ፀጋ','J o j o','`E`z`i`','y e r u s',
            '𝘓','t u t u u','𝘌𝘮𝘱𝘵𝘺','𝘉𝘭𝘦𝘴𝘴𝘦𝘥',
            '♤ 𝘥 𝘢 𝘯 ♤','L e m e n','ጌታ','N o n o',
            '`B`r`u`k','m i n a','𝘗','e z i i i',
            '𝘝𝘰𝘪𝘥','𝘙𝘦𝘥𝘦𝘦𝘮𝘦𝘥','♧ 𝘳 𝘰 𝘣 ♧','M a n',
            'ራህመት','P i p i','`S`a`f`i','t s e d',
            '𝘊','f i o o o','𝘕𝘰𝘵𝘩𝘪𝘯𝘨','𝘚𝘢𝘷𝘦𝘥',
            '♢ 𝘧 𝘪 𝘬 ♢','H u l','ጁማ','T o t o',
            '`E`l`u','y o f e','𝘝','s o l l',
            '𝘚𝘩𝘢𝘥𝘰𝘸','𝘏𝘰𝘭𝘺','♡ 𝘭 𝘪 𝘥 ♡','C h i l',
            'ህይወት','L o l o','`J`e`r`i','a b i . .',
            '𝘡','b a b b','𝘉𝘭𝘪𝘴𝘴','𝘗𝘳𝘢𝘺',
            '♛ 𝘯 𝘢 ♛','W e y','ተስፋ','Y o y o',
            '`R`e`d`i','m e z e','𝘎','g a r i i',
            '𝘚𝘦𝘳𝘦𝘯𝘪𝘵𝘺','𝘓𝘰𝘳𝘥','⚡︎ 𝘬 𝘢 𝘭 ⚡︎','G e d',
            'ብርሃን','G o g o','`B`o`g`i','z e d',
            '𝘍','n o a h h','𝘓𝘰𝘴𝘵','𝘊𝘩𝘳𝘪𝘴𝘵',
            '⚓︎ 𝘣 𝘳 𝘶 ⚓︎','Z i m','ቃል','S h u s h u',
            '`M`i`k`i','j a p p y','𝘛','s i d d',
            '𝘛𝘪𝘳𝘦𝘥','𝘚𝘢𝘪𝘯𝘵','☁︎ 𝘴 𝘦 𝘭 ☁︎','B e s m',
            'እምነት','K u k u','`S`e`l`i','l e l a',
            '𝘞','y e m i i','𝘌𝘯𝘪𝘨𝘮𝘢','𝘗𝘦𝘢𝘤𝘦',
            '✈︎ 𝘮 𝘦 𝘭 ✈︎','K e f','ፀሀይ','M a c a',
            '`L`i`n`a','e b a','𝘘','f a r r',
            '𝘈𝘣𝘺𝘴𝘴','𝘔𝘦𝘳𝘤𝘺','✌︎ 𝘵 𝘴 𝘦 ✌︎','A r e',
            'ጨረቃ','P a p a','`M`i`k`y','t o k a',
            '𝘟','k a l l','𝘔𝘪𝘳𝘢𝘨𝘦','𝘏𝘦𝘢𝘷𝘦𝘯',
            '✍︎ 𝘢 𝘣 ✍︎','B e q','ኪያዬ','D a d a',
            '`D`a`g`i','j o c c h a','𝘠','k i r a a',
            '𝘌𝘤𝘩𝘰','𝘈𝘯𝘨𝘦𝘭','☘︎ 𝘧 𝘢 ☘︎','E n d e',
            'ሰማይ','C o c o','`F`a`s`i`l','f r a y',
            '𝘖','l u c c','𝘊𝘩𝘪𝘭𝘭','𝘗𝘳𝘰𝘱𝘩𝘦𝘵',
            '☂︎ 𝘨 𝘢 ☂︎','D e r','ምድር','Z i z i',
            '`H`e`n`i','h i w i','𝘐','t i t i i',
            '𝘋𝘢𝘸𝘯','𝘕𝘰𝘶𝘳','☕︎ 𝘫 𝘦 ☕︎','T e l',
            'ዝምታ','R i r i','`A`m`u','m a c k',
            '𝘜','a l x x','𝘋𝘶𝘴𝘬','𝘋𝘦𝘦𝘯',
            '♈︎ 𝘢 𝘮 ♈︎','M e c h','እውነት','V i v i',
            '`T`a`r`i','y o d i','𝘒','m a r i i',
            '𝘓𝘶𝘯𝘢','𝘎𝘭𝘰𝘳𝘺','☯︎ 𝘳 𝘦 ☯︎','S e w',
            'ፍቅር☥','L i l i','`N`a`h`o`m`','n e n a .',
            '𝘏 .','y u m i i i','𝘚𝘵𝘢𝘳 .','𝘡𝘪𝘰𝘯',
            '☽ 𝘺 𝘰 ☾','F a r','ኑሮ .','W u w u'
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
