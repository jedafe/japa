ALTER TABLE wallet_settings
    ADD COLUMN daily_earning_cap INT NOT NULL DEFAULT 1000,
    ADD COLUMN enable_earning BOOLEAN NOT NULL DEFAULT true;
