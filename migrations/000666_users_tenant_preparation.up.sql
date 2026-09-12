-- Phase 1.2 — users subsystem single-tenant preparation.
-- Adds global unique constraints/indexes needed before tenant_id filters are
-- removed from users.sql and the tenant_id columns can eventually be dropped.
-- All data is already verified to belong to the default tenant.

DO $$
BEGIN
    IF EXISTS (SELECT email FROM users GROUP BY email HAVING COUNT(*) > 1) THEN
        RAISE EXCEPTION 'Duplicate emails found; cannot make email globally unique';
    END IF;
    IF EXISTS (SELECT username FROM users GROUP BY username HAVING COUNT(*) > 1) THEN
        RAISE EXCEPTION 'Duplicate usernames found; cannot make username globally unique';
    END IF;
    IF EXISTS (SELECT user_id FROM user_profiles GROUP BY user_id HAVING COUNT(*) > 1) THEN
        RAISE EXCEPTION 'Duplicate user_profiles rows found';
    END IF;
    IF EXISTS (SELECT user_id FROM user_preferences GROUP BY user_id HAVING COUNT(*) > 1) THEN
        RAISE EXCEPTION 'Duplicate user_preferences rows found';
    END IF;
    IF EXISTS (SELECT user_id FROM user_privacy_settings GROUP BY user_id HAVING COUNT(*) > 1) THEN
        RAISE EXCEPTION 'Duplicate user_privacy_settings rows found';
    END IF;
    IF EXISTS (SELECT provider, provider_user_id FROM oauth_accounts GROUP BY provider, provider_user_id HAVING COUNT(*) > 1) THEN
        RAISE EXCEPTION 'Duplicate oauth_accounts provider/provider_user_id found';
    END IF;
    IF EXISTS (SELECT user_id, provider FROM oauth_accounts GROUP BY user_id, provider HAVING COUNT(*) > 1) THEN
        RAISE EXCEPTION 'Duplicate oauth_accounts user_id/provider found';
    END IF;
END
$$;

CREATE UNIQUE INDEX idx_users_email_global ON users(email);
CREATE UNIQUE INDEX idx_users_username_global ON users(username);

CREATE UNIQUE INDEX idx_user_profiles_user_id_global ON user_profiles(user_id);

CREATE UNIQUE INDEX idx_user_preferences_user_id_global ON user_preferences(user_id);

-- user_privacy_settings is currently (tenant_id, user_id); make it (user_id).
ALTER TABLE user_privacy_settings DROP CONSTRAINT user_privacy_settings_pkey;
ALTER TABLE user_privacy_settings ADD PRIMARY KEY (user_id);

CREATE UNIQUE INDEX idx_oauth_accounts_provider_user_id_global ON oauth_accounts(provider, provider_user_id);
CREATE UNIQUE INDEX idx_oauth_accounts_user_provider_global ON oauth_accounts(user_id, provider);

-- Better index for refresh_tokens queries without tenant filter.
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);
