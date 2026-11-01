-- 001013_verification_types.up.sql
-- Managed verification badge-type taxonomy (§07 amendment + §98): replaces the
-- compiled-in enums for org/user badges and service-listing verification with
-- an admin-managed table carrying a custom badge image per type.
-- Ref: .jeos/sections/07-organization-profiles-claim-ownership.md (managed badge types)
BEGIN;

CREATE TABLE IF NOT EXISTS verification_types (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scope          text NOT NULL CHECK (scope IN ('organization','user','service_listing')),
    value          text NOT NULL,        -- snake_case key stored on entity verification_type
    label          text NOT NULL,
    description    text,
    badge_media_id uuid REFERENCES media(id) ON DELETE SET NULL,
    position       int  NOT NULL DEFAULT 0,
    is_active      bool NOT NULL DEFAULT true,
    is_protected   bool NOT NULL DEFAULT false, -- seeded vocabulary: deactivate, never delete
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (scope, value)
);

-- §07 user/org badge vocabulary (seeded as protected rows on both scopes).
INSERT INTO verification_types (scope, value, label, position, is_protected) VALUES
    ('organization', 'business',           'Business',            10, true),
    ('organization', 'embassy',            'Embassy',             20, true),
    ('organization', 'notable',            'Notable',             30, true),
    ('organization', 'immigration_lawyer', 'Immigration Lawyer',  40, true),
    ('organization', 'professional_body',  'Professional Body',   50, true),
    ('user',         'business',           'Business',            10, true),
    ('user',         'embassy',            'Embassy',             20, true),
    ('user',         'notable',            'Notable',             30, true),
    ('user',         'immigration_lawyer', 'Immigration Lawyer',  40, true),
    ('user',         'professional_body',  'Professional Body',   50, true),
    ('service_listing', 'self',              'Self-verified',              10, true),
    ('service_listing', 'community',         'Community-verified',         20, true),
    ('service_listing', 'admin',             'Admin-verified',             30, true),
    ('service_listing', 'professional_body', 'Professional Body-verified', 40, true)
ON CONFLICT (scope, value) DO NOTHING;

-- RBAC node (permission-key-naming: <area>.<entity>.<action>).
INSERT INTO permissions (key, description) VALUES
    ('verification.types.manage', 'Manage verification badge types and badge artwork (admin)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role', 'admin-group-role')
  AND p.key = 'verification.types.manage'
ON CONFLICT DO NOTHING;

COMMIT;
