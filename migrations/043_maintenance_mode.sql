-- Maintenance mode: a master switch (edited from the admin settings page) that
-- puts the player Mini App into read-only "we'll be right back" mode without
-- stopping the whole service, so the admin dashboard and API stay up for review.
-- Extends the single-row app_settings table (migration 038).

ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS maintenance_mode    BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS maintenance_message TEXT    NOT NULL DEFAULT '';
