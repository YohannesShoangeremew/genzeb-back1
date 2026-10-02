BEGIN;

-- 1. Create the agents table
CREATE TABLE IF NOT EXISTS agents (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    telegram_id   BIGINT UNIQUE NOT NULL,
    referral_code VARCHAR(32) UNIQUE NOT NULL,
    balance       NUMERIC(12, 2) NOT NULL DEFAULT 0.00 CHECK (balance >= 0),
    commission_rate NUMERIC(5, 4) NOT NULL DEFAULT 0.05, -- e.g. 5% default commission
    is_active     BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agents_telegram_id ON agents(telegram_id);
CREATE INDEX IF NOT EXISTS idx_agents_referral_code ON agents(referral_code);

-- 2. Link users to an agent
ALTER TABLE users 
    ADD COLUMN IF NOT EXISTS agent_id UUID REFERENCES agents(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_users_agent_id ON users(agent_id) WHERE agent_id IS NOT NULL;

-- 3. Create commission ledger table
CREATE TABLE IF NOT EXISTS agent_commissions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id   UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    player_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    game_id    UUID REFERENCES games(id) ON DELETE SET NULL,
    amount     NUMERIC(10, 2) NOT NULL CHECK (amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_agent_commissions_agent_id ON agent_commissions(agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_commissions_game_id ON agent_commissions(game_id);

-- 4. Extend transactions_category_check constraint to include agent payouts/commissions
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'transactions_category_check'
    ) THEN
        ALTER TABLE transactions DROP CONSTRAINT transactions_category_check;
    END IF;
    
    ALTER TABLE transactions ADD CONSTRAINT transactions_category_check
        CHECK (category IN (
            'deposit', 'withdrawal', 'bet', 'winnings', 'refund',
            'transfer_in', 'transfer_out', 'admin_credit', 'admin_debit',
            'bot_funding', 'agent_commission', 'agent_payout'
        ));
END $$;

COMMIT;