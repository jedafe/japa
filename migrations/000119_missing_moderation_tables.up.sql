-- AUDIT-BE-033: Add missing tables referenced by moderation, GDPR, affiliate, and SEO handlers

-- Moderation actions (ban/unban/warn) — referenced by internal/admin/moderation.go
CREATE TABLE user_moderation_actions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    action      text NOT NULL CHECK (action IN ('ban', 'unban', 'warning')),
    reason      text,
    expires_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_user_moderation_user   ON user_moderation_actions (user_id, action);
CREATE INDEX idx_user_moderation_tenant ON user_moderation_actions (tenant_id, action);

-- Moderation appeals — referenced by internal/admin/moderation.go
CREATE TABLE moderation_appeals (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type text NOT NULL,
    entity_id   uuid NOT NULL,
    reason      text NOT NULL,
    status      text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_moderation_appeals_status ON moderation_appeals (status, created_at DESC);

-- Admin broadcasts — referenced by internal/admin/moderation.go
CREATE TABLE admin_broadcasts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title           text NOT NULL,
    body            text NOT NULL,
    sent_by         uuid REFERENCES users(id) ON DELETE SET NULL,
    sent_at         timestamptz NOT NULL DEFAULT now(),
    recipient_count int NOT NULL DEFAULT 0
);

-- Affiliate program settings — referenced by internal/affiliate/handlers.go
CREATE TABLE affiliate_program_settings (
    tenant_id        uuid PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    commission_rates jsonb NOT NULL DEFAULT '{}',
    updated_at       timestamptz NOT NULL DEFAULT now()
);
