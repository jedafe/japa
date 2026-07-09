-- 000110_bookmark_folders.up.sql
-- Adds first-class named bookmark folders (catalog: GET/POST /me/bookmark-folders,
-- PATCH/DELETE /me/bookmark-folders/:id). Distinct from bookmarks.folder (a
-- free-text default-'default' column added in 000103) — that denormalized
-- string is left untouched; reconciling the two is a separate decision.

BEGIN;

CREATE TABLE bookmark_folders (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(user_id, name)
);

CREATE INDEX idx_bookmark_folders_user ON bookmark_folders(tenant_id, user_id);

COMMIT;
