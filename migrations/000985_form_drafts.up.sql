-- §71 amendment: generic in-progress form drafts — reusable server-side
-- save-progress for multi-step tools (readiness, tour, sop, other wizards).
-- Keyed by (user_id, kind, draft_key); `data` is the tool's opaque payload.
CREATE TABLE IF NOT EXISTS form_drafts (
    id          uuid        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        text        NOT NULL,
    draft_key   text        NOT NULL DEFAULT '',
    data        jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT form_drafts_kind_format CHECK (kind ~ '^[a-z][a-z0-9_-]{0,63}$'),
    CONSTRAINT form_drafts_unique UNIQUE (user_id, kind, draft_key)
);
CREATE INDEX IF NOT EXISTS idx_form_drafts_user ON form_drafts (user_id, kind);
