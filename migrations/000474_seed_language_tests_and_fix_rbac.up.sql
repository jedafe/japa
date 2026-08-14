-- 000474_seed_language_tests_and_fix_rbac.up.sql
-- Fix BUG-001: language_tests table was created by 000339 but never seeded.
-- The 8 canonical tests (IELTS, OET, TOEFL, PTE, CELPIP, Goethe, DELF, other)
-- must exist for GET /language-tests to return data and for test-slug lookups
-- to resolve. Without these rows the entire Language Test Hub (§99) is
-- non-functional — every /language-tests/:test_slug request 404s.
--
-- Fix BUG-005: language_test_hub.view was seeded by 000340 but never granted
-- to any role. §99 RBAC table says "all". Public GET routes are gated only by
-- the feature flag (not .view) in server.go, so this is non-blocking at
-- runtime, but the permission node must be granted for catalog conformance
-- and for any future route that enforces it.
--
-- Also fix BUG-005b: language_test_hub.manage was granted to super_admin only
-- by 000340, but §99 says "admin, super_admin". Peer features (housing_guide
-- 000470) applied the same correction.
--
-- Ref: japaconnect-engineering/sections/99-language-test-hub.md
--      japaconnect-engineering/conventions/permission-key-naming.md

BEGIN;

-- BUG-001: seed the canonical language tests.
INSERT INTO language_tests (slug, name, official_url, validity_months) VALUES
    ('ielts',   'IELTS',   'https://www.ielts.org',                     24),
    ('oet',     'OET',     'https://www.occupationalenglishtest.org',    24),
    ('toefl',   'TOEFL',   'https://www.toefl.org',                      24),
    ('pte',     'PTE',     'https://www.pearsonpte.com',                 24),
    ('celpip',  'CELPIP',  'https://www.celpip.ca',                      24),
    ('goethe',  'Goethe',  'https://www.goethe.de',                      24),
    ('delf',    'DELF',    'https://www.france-education-international.fr', 24),
    ('other',   'Other',   NULL,                                         NULL)
ON CONFLICT (slug) DO NOTHING;

-- BUG-005: grant language_test_hub.view to all member-level roles per §99 ("all").
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'professional', 'agent', 'moderator', 'admin', 'super_admin')
  AND p.key = 'language_test_hub.view'
ON CONFLICT DO NOTHING;

-- BUG-005b: grant language_test_hub.manage to the admin role (was super_admin only).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin')
  AND p.key = 'language_test_hub.manage'
ON CONFLICT DO NOTHING;

COMMIT;
