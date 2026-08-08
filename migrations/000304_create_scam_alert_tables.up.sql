-- 000304_create_scam_alert_tables.up.sql
-- Scam Alert Board (§81, ADR-089) — scam_reports + scam_alert_subscriptions.
--
-- Adaptation: §81 references sites(id) and geo_countries(id). Per ADR-053,
-- the codebase uses uuid PKs, tenants(id) for tenancy, and spaces(id) for
-- geo entities (countries and cities are modeled as spaces with a type
-- discriminator).
--
-- Ref: japaconnect-engineering/sections/81-scam-alert-board.md

BEGIN;

CREATE TABLE IF NOT EXISTS scam_reports (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    reporter_id             uuid REFERENCES users(id) ON DELETE SET NULL, -- nullable for anonymous
    scam_type               text NOT NULL, -- impersonation|fake_job|phishing|payment_fraud|housing|other
    destination_space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    origin_space_id         uuid REFERENCES spaces(id) ON DELETE SET NULL,
    origin_community        text, -- free-text community tag
    platform_used           text, -- whatsapp|telegram|email|facebook|linkedin|other
    approximate_date        date,
    amount_range            text, -- e.g. "$500-$1000"
    description             text NOT NULL,
    reported_to_authorities boolean NOT NULL DEFAULT false,
    status                  text NOT NULL DEFAULT 'pending', -- pending|confirmed|rejected
    moderator_note          text,
    is_pinned               boolean NOT NULL DEFAULT false,
    pinned_space_id         uuid REFERENCES spaces(id) ON DELETE SET NULL,
    reviewed_by             uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at             timestamptz,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    deleted_at              timestamptz -- Rule 11: soft-delete user content
);

CREATE INDEX IF NOT EXISTS idx_scam_reports_tenant_status
    ON scam_reports(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_scam_reports_tenant_dest_status
    ON scam_reports(tenant_id, destination_space_id, status);

CREATE TABLE IF NOT EXISTS scam_alert_subscriptions (
    user_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id            uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    destination_space_id uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    created_at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(user_id, destination_space_id)
);

COMMIT;
