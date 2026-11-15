-- 001156_intent_nav_items.up.sql
-- ADR-128: the intent_matrix pin_paths/hide_paths act on resolved menu items,
-- but several referenced surfaces had no main_sidebar entry — the rules were
-- dead config. Seed the missing items so intent shaping (visit/business pins,
-- study/work pins, visit/business hides) has real targets, and students can
-- discover /universities and /scholarships from the nav.
--
-- Each item carries its route's feature flag so the server-side flag filter
-- (menu_items.feature_flag, 000953) hides it when the tool is disabled.

BEGIN;

INSERT INTO menu_items (menu_id, label, url, icon, sort_order, visibility, feature_flag)
SELECT m.id, v.label, v.url, v.icon, v.sort_order, 'authenticated', v.feature_flag
FROM menus m
JOIN (VALUES
	('Universities',       '/universities',             'school',         35, NULL),
	('Scholarships',       '/scholarships',             'banknote',       36, 'scholarship_finder.enabled'),
	('Destination Wiki',   '/wiki',                     'map',            37, 'wiki.enabled'),
	('Travel',             '/tools/travel',             'globe',          38, 'travel.enabled'),
	('Community Locator',  '/tools/community-locator',  'map-pin',        39, 'community_locator.enabled'),
	('Credential Checker', '/tools/credential-checker', 'clipboard-list', 40, 'credential_checker.enabled'),
	('Language Test',      '/tools/language-test',      'languages',      41, 'language_test_hub.enabled'),
	('CV Formatter',       '/tools/cv-formatter',       'file-text',      42, 'cv_formatter.enabled'),
	('Tax Calculator',     '/tools/tax-calculator',     'dollar-sign',    43, 'tax_calculator.enabled')
) AS v(label, url, icon, sort_order, feature_flag) ON true
WHERE m.slug = 'main_sidebar'
  AND NOT EXISTS (
	SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = v.url
);

COMMIT;
