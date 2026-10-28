-- 000948_donations_tips.up.sql
-- §125 Donations & Tips: recipient tip settings, outbound external links,
-- and the tips ledger. Settlement paths reuse existing primitives —
-- wallet_ledger pairs (source='tip') for points/credits and the §17
-- payments table (purpose_type='donation', purpose_id=tips.id) for fiat.
-- RBAC node seeds grant tips.send to member tiers, tips.receive to
-- pro/business + admin tiers (mentors pass via user_profiles.is_mentor
-- in service code), and tips.moderate to admin tiers. Notification type
-- tip.received (billing category) powers recipient fan-out.
--
-- Ref: .jeos/sections/125-donations-tips.md

BEGIN;

CREATE TABLE public.tip_settings (
    id                       uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_type               text NOT NULL,
    owner_id                 uuid NOT NULL,
    tips_enabled             boolean DEFAULT false NOT NULL,
    accept_points            boolean DEFAULT true NOT NULL,
    accept_credits           boolean DEFAULT true NOT NULL,
    accept_fiat              boolean DEFAULT true NOT NULL,
    accept_external          boolean DEFAULT false NOT NULL,
    oc_collective_slug       text,
    oc_api_token_encrypted   text,
    oc_webhook_secret        text,
    min_tip_amount           bigint DEFAULT 0 NOT NULL,
    thank_you_message        text,
    created_at               timestamp with time zone DEFAULT now() NOT NULL,
    updated_at               timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT tip_settings_pkey PRIMARY KEY (id),
    CONSTRAINT tip_settings_owner_type_check CHECK ((owner_type = ANY (ARRAY['user'::text, 'organization'::text, 'platform'::text]))),
    CONSTRAINT tip_settings_owner_unique UNIQUE (owner_type, owner_id)
);

CREATE TABLE public.tip_external_links (
    id              uuid DEFAULT gen_random_uuid() NOT NULL,
    tip_settings_id uuid NOT NULL,
    kind            text NOT NULL,
    label           text NOT NULL,
    url             text NOT NULL,
    position        integer DEFAULT 0 NOT NULL,
    created_at      timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT tip_external_links_pkey PRIMARY KEY (id),
    CONSTRAINT tip_external_links_kind_check CHECK ((kind = ANY (ARRAY['paypal'::text, 'kofi'::text, 'buymeacoffee'::text, 'opencollective'::text, 'other'::text]))),
    CONSTRAINT tip_external_links_settings_fk FOREIGN KEY (tip_settings_id) REFERENCES public.tip_settings(id) ON DELETE CASCADE
);

CREATE TABLE public.tips (
    id                        uuid DEFAULT gen_random_uuid() NOT NULL,
    sender_user_id            uuid,
    external_sender_name      text,
    recipient_type            text NOT NULL,
    recipient_user_id         uuid,
    recipient_organization_id uuid,
    content_id                uuid,
    method                    text NOT NULL,
    points_amount             bigint DEFAULT 0 NOT NULL,
    credits_amount            bigint DEFAULT 0 NOT NULL,
    fiat_amount               bigint,
    fiat_currency             text,
    fee_amount                bigint DEFAULT 0 NOT NULL,
    net_credits               bigint DEFAULT 0 NOT NULL,
    payment_id                uuid,
    oc_contribution_id        text,
    message                   text,
    is_anonymous              boolean DEFAULT false NOT NULL,
    status                    text DEFAULT 'pending'::text NOT NULL,
    idempotency_key           text,
    created_at                timestamp with time zone DEFAULT now() NOT NULL,
    updated_at                timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT tips_pkey PRIMARY KEY (id),
    CONSTRAINT tips_sender_fk FOREIGN KEY (sender_user_id) REFERENCES public.users(id),
    CONSTRAINT tips_recipient_user_fk FOREIGN KEY (recipient_user_id) REFERENCES public.users(id),
    CONSTRAINT tips_recipient_org_fk FOREIGN KEY (recipient_organization_id) REFERENCES public.organizations(id),
    CONSTRAINT tips_content_fk FOREIGN KEY (content_id) REFERENCES public.contents(id),
    CONSTRAINT tips_payment_fk FOREIGN KEY (payment_id) REFERENCES public.payments(id),
    CONSTRAINT tips_recipient_type_check CHECK ((recipient_type = ANY (ARRAY['platform'::text, 'user'::text, 'organization'::text]))),
    CONSTRAINT tips_recipient_check CHECK (
        ((recipient_type = 'platform'::text) AND (recipient_user_id IS NULL) AND (recipient_organization_id IS NULL)) OR
        ((recipient_type = 'user'::text) AND (recipient_user_id IS NOT NULL) AND (recipient_organization_id IS NULL)) OR
        ((recipient_type = 'organization'::text) AND (recipient_user_id IS NULL) AND (recipient_organization_id IS NOT NULL))),
    CONSTRAINT tips_method_check CHECK ((method = ANY (ARRAY['points'::text, 'credits'::text, 'stripe'::text, 'paystack'::text, 'nowpayments'::text, 'manual_transfer'::text, 'opencollective'::text]))),
    CONSTRAINT tips_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'completed'::text, 'failed'::text, 'refunded'::text]))),
    CONSTRAINT tips_oc_dedupe UNIQUE (oc_contribution_id)
);

CREATE INDEX idx_tips_recipient_user ON public.tips USING btree (recipient_user_id, created_at DESC);
CREATE INDEX idx_tips_recipient_org ON public.tips USING btree (recipient_organization_id, created_at DESC);
CREATE INDEX idx_tips_sender ON public.tips USING btree (sender_user_id, created_at DESC);
CREATE INDEX idx_tips_content ON public.tips USING btree (content_id) WHERE (content_id IS NOT NULL);
CREATE INDEX idx_tips_status_method ON public.tips USING btree (status, method);

-- RBAC nodes (conventions/permission-key-naming.md)
INSERT INTO permissions (key, description) VALUES
    ('tips.send',     'Send tips/donations to creators, organizations, or the platform'),
    ('tips.receive',  'Enable tips and configure accepted settlement methods'),
    ('tips.moderate', 'Admin tips list, stats, and refund actions')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id FROM user_roles ur, permissions p
WHERE ur.slug IN ('member', 'pro_member', 'business_member') AND p.key = 'tips.send'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id FROM user_roles ur, permissions p
WHERE ur.slug IN ('pro_member', 'business_member', 'super_admin', 'super_admin_role', 'admin-group-role') AND p.key = 'tips.receive'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role', 'admin-group-role') AND p.key = 'tips.moderate'
ON CONFLICT DO NOTHING;

-- Notification type for recipient fan-out (tips:notify_received task).
INSERT INTO public.notification_types (key, label, description, enabled, default_in_app, default_email, default_push, category)
VALUES ('tip.received', 'Tip Received', 'Someone tipped you', true, true, true, false, 'billing')
ON CONFLICT (key) DO NOTHING;

COMMIT;
