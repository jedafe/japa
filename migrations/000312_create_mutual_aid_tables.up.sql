-- 000312_create_mutual_aid_tables.up.sql
-- Mutual Aid Board (§85, ADR-093).
-- Ref: japaconnect-engineering/sections/85-mutual-aid-board.md

BEGIN;

CREATE TABLE IF NOT EXISTS mutual_aid_posts (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id               uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    post_type             text NOT NULL, -- need|offer|skill_swap
    category              text NOT NULL, -- housing|job|transport|childcare|translation|other
    title                 text NOT NULL,
    description           text NOT NULL,
    destination_space_id  uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    city_space_id         uuid REFERENCES spaces(id) ON DELETE SET NULL,
    expires_at            timestamptz,
    is_fulfilled          boolean NOT NULL DEFAULT false,
    status                text NOT NULL DEFAULT 'active', -- active|expired|moderated|fulfilled
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    deleted_at            timestamptz -- Rule 11: soft-delete user content
);

CREATE INDEX IF NOT EXISTS idx_mutual_aid_posts_tenant_status_dest
    ON mutual_aid_posts(tenant_id, status, destination_space_id)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_mutual_aid_posts_tenant_expires
    ON mutual_aid_posts(tenant_id, expires_at)
    WHERE status = 'active' AND deleted_at IS NULL;

COMMIT;
