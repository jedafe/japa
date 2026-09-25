-- Phase 19: User privacy settings for /me/privacy endpoints.
-- Migration 000106: creates user_privacy_settings table.

CREATE TABLE IF NOT EXISTS user_privacy_settings (
    user_id              uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    dm_privacy           text NOT NULL DEFAULT 'everyone'
                           CHECK (dm_privacy IN ('everyone','followers_only','nobody')),
    profile_visibility   text NOT NULL DEFAULT 'public'
                           CHECK (profile_visibility IN ('public','members','followers')),
    search_indexable     boolean NOT NULL DEFAULT true,
    show_online_status   boolean NOT NULL DEFAULT true,
    show_activity        boolean NOT NULL DEFAULT true,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);
