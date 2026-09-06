-- App settings: operator-tunable knobs edited from the admin dashboard. Starts
-- with the minimum deposit (default 50 birr); add more columns here over time.
-- Single-row table (id = 1), same pattern as bot_config / bonus_config.

CREATE TABLE IF NOT EXISTS app_settings (
    id          INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    min_deposit NUMERIC(10, 2) NOT NULL DEFAULT 50 CHECK (min_deposit >= 0),
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO app_settings (id, min_deposit) VALUES (1, 50)
ON CONFLICT (id) DO NOTHING;
