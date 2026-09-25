-- 000327_create_appointment_alert_tables.up.sql
-- Embassy Appointment Alerter (§91, ADR-099) — appointment_alerts + monitored_embassies.
--
-- Adaptation: §91 references sites(id), geo_countries(id), visa_categories(id).
-- Per ADR-053, tenancy uses tenants(id) (uuid), countries are spaces with
-- level='country', and visa category is stored as nullable text.
--
-- Ref: japaconnect-engineering/sections/91-embassy-appointment-alerter.md

BEGIN;

CREATE TABLE IF NOT EXISTS monitored_embassies (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    destination_space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    slug                    text NOT NULL,
    name                    text NOT NULL,
    booking_url             text NOT NULL,
    poll_interval_seconds   int NOT NULL DEFAULT 900,
    is_active               boolean NOT NULL DEFAULT true,
    last_polled_at          timestamptz,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_monitored_embassies_tenant_dest
    ON monitored_embassies(tenant_id, destination_space_id, is_active);

CREATE TABLE IF NOT EXISTS appointment_alerts (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id                 uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    destination_space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE RESTRICT,
    embassy_slug            text NOT NULL,
    visa_category           text,
    date_range_start        date NOT NULL,
    date_range_end          date NOT NULL,
    status                  text NOT NULL DEFAULT 'active', -- active|paused|fired|expired
    last_checked_at         timestamptz,
    fired_at                timestamptz,
    expires_at              date,
    notification_channels   text[] NOT NULL DEFAULT '{in_app,email}',
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_appointment_alerts_tenant_status
    ON appointment_alerts(tenant_id, status, embassy_slug);
CREATE INDEX IF NOT EXISTS idx_appointment_alerts_user
    ON appointment_alerts(user_id, status);

COMMIT;
