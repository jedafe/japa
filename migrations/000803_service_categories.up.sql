-- 000803_service_categories.up.sql
-- Services Marketplace — dynamic service_categories taxonomy replacing the
-- hardcoded §98 enum, plus seeds for the new marketplace categories.
-- Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md §2.1
BEGIN;

CREATE TABLE IF NOT EXISTS service_categories (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug         text NOT NULL UNIQUE,
    label        text NOT NULL,
    description  text,
    icon         text,          -- lucide icon name
    accent_color text,          -- one of the token-mapped palette names
    applies_to   text NOT NULL DEFAULT 'both'
        CHECK (applies_to IN ('service','package','both')),
    position     int  NOT NULL DEFAULT 0,
    is_active    bool NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

INSERT INTO service_categories (slug, label, description, icon, accent_color, applies_to, position) VALUES
    ('immigration_legal',  'Immigration & Legal',  'Visas, PR, citizenship and legal help',            'scale',        'violet', 'both',    10),
    ('study_abroad',       'Study Abroad',         'Admissions, scholarships and student visas',       'graduation-cap','green', 'package', 20),
    ('education',          'Education',            'Schools, courses and credential evaluation',       'book-open',    'green',  'service', 30),
    ('work_abroad',        'Work Abroad',          'Jobs, CV review and interview prep',               'briefcase',    'amber',  'both',    40),
    ('relocation',         'Relocation',           'Accommodation, airport pickup and settling in',    'home',         'blue',   'both',    50),
    ('travel',             'Travel',               'Flights, itineraries and travel documents',        'plane',        'sky',    'both',    60),
    ('finance',            'Finance',              'Banking, transfers and tax help',                  'landmark',     'emerald','both',    70),
    ('health',             'Health & Insurance',   'Health coverage and insurance plans',              'heart-pulse',  'rose',   'both',    80),
    ('family_sponsorship', 'Family Sponsorship',   'Sponsor loved ones to join you abroad',            'users',        'orange', 'package', 90),
    ('documents',          'Documents',            'Translation, attestation and notarization',        'file-text',    'slate',  'service', 100),
    ('community',          'Community',            'Diaspora community services',                      'users-round',  'indigo', 'service', 110),
    ('other',              'Other',                'Everything else',                                  'grid-2x2',     'gray',   'both',    999)
ON CONFLICT (slug) DO NOTHING;

-- RBAC node (permission-key-naming: <area>.<entity>.<action>)
INSERT INTO permissions (key, description) VALUES
    ('service.categories.manage', 'Create/edit/delete service marketplace categories (admin)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role', 'admin-group-role')
  AND p.key = 'service.categories.manage'
ON CONFLICT DO NOTHING;

COMMIT;
