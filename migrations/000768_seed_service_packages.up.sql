-- 000768_seed_service_packages.up.sql
-- §111 / ADR-123 — seed the complete RBAC node set, system settings, and
-- notification types for Service Packages & Bookings (BE-005: never
-- partially seed). The feature flag itself is code-registered
-- (internal/features/registry.go, key 'service_packages.enabled').
--
-- Ref: .jeos/sections/111-service-packages-bookings.md
--      .jeos/conventions/permission-key-naming.md
--      .jeos/conventions/system-settings-catalog.md

BEGIN;

-- RBAC nodes
INSERT INTO permissions (key, description) VALUES
    ('service.package.create', 'Create/edit service packages (verified-org members; org.verified re-checked in service layer)'),
    ('service.booking.create', 'Submit a service inquiry or booking'),
    ('service.booking.manage', 'View and transition own organization''s service bookings/leads'),
    ('service.booking.admin',  'View/transition any service booking; resolve disputes (admin)')
ON CONFLICT (key) DO NOTHING;

-- Role grants. package.create + booking.manage mirror service.create's
-- grant set (member tiers + admins); booking.create is member+;
-- booking.admin is admin-only (mirrors service.verify).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('member', 'pro_member', 'business_member', 'super_admin', 'super_admin_role', 'admin-group-role')
  AND p.key IN ('service.package.create', 'service.booking.manage')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('member', 'pro_member', 'business_member', 'super_admin', 'super_admin_role', 'admin-group-role')
  AND p.key = 'service.booking.create'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role', 'admin-group-role')
  AND p.key = 'service.booking.admin'
ON CONFLICT DO NOTHING;

-- System settings (admin-editable; code falls back to the same defaults
-- via GetIntOr/GetBoolOr per the settings catalog convention).
INSERT INTO system_settings (key, value) VALUES
    ('service_packages.fee_enabled',           to_jsonb(true)),
    ('service_packages.fee_type',              to_jsonb('percent'::text)),
    ('service_packages.fee_pct',               to_jsonb(10)),
    ('service_packages.fee_fixed',             to_jsonb(0)),
    ('service_packages.max_items',             to_jsonb(25)),
    ('service_packages.require_verified_org',  to_jsonb(true)),
    ('service_packages.review_before_publish', to_jsonb(true)),
    ('service_packages.org_response_sla_hours', to_jsonb(72)),
    ('service_packages.refund_window_days',    to_jsonb(7))
ON CONFLICT (key) DO NOTHING;

-- Notification types (§35 seed pattern). Booker-facing types default all
-- three channels on; org-facing defaults to in-app + email.
INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push, category) VALUES
    ('service_booking_new',        'New service booking',      'A new inquiry or booking arrived for your organization', true, true, false, 'services'),
    ('service_booking_confirmed',  'Booking confirmed',        'Your service booking was confirmed by the provider',     true, true, true,  'services'),
    ('service_booking_declined',   'Booking declined',         'Your service booking was declined by the provider',      true, true, true,  'services'),
    ('service_booking_fulfilled',  'Booking fulfilled',        'Your service booking was marked fulfilled',              true, true, true,  'services'),
    ('service_booking_cancelled',  'Booking cancelled',        'A service booking was cancelled',                        true, true, true,  'services'),
    ('service_booking_reminder',   'Booking response reminder','A pending booking is approaching its response deadline', true, true, false, 'services')
ON CONFLICT (key) DO NOTHING;

COMMIT;
