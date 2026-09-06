-- Allow multiple players to have the same card_id in a game
-- Remove the UNIQUE constraint on (game_id, card_id)

-- Drop the unique constraint if it exists
DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    -- Find the constraint name for unique constraint on (game_id, card_id)
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid = 'game_players'::regclass
      AND contype = 'u'
      AND array_length(conkey, 1) = 2
      AND conkey[1] = (SELECT attnum FROM pg_attribute WHERE attrelid = 'game_players'::regclass AND attname = 'game_id')
      AND conkey[2] = (SELECT attnum FROM pg_attribute WHERE attrelid = 'game_players'::regclass AND attname = 'card_id');
    
    -- Drop the constraint if found
    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE game_players DROP CONSTRAINT %I', constraint_name);
        RAISE NOTICE 'Dropped constraint: %', constraint_name;
    ELSE
        RAISE NOTICE 'Constraint not found, may have already been dropped';
    END IF;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'Error dropping constraint: %', SQLERRM;
END $$;

