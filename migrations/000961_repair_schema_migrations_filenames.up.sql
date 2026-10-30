-- Repairs schema_migrations after renumbering the duplicate-prefixed
-- migrations (two files each shared 000957/000958/000959/000960). The tracker
-- is filename-keyed, so a renamed file whose OLD name was already applied on
-- an environment must be pre-recorded under its NEW name or the tool would
-- re-run it. On fresh environments the WHERE EXISTS clause inserts nothing
-- and the renamed file applies normally.
BEGIN;

INSERT INTO schema_migrations (filename)
SELECT '000962_assessment_integrity_fixes.up.sql'
WHERE EXISTS (SELECT 1 FROM schema_migrations
              WHERE filename = '000957_assessment_integrity_fixes.up.sql');

INSERT INTO schema_migrations (filename)
SELECT '000963_seed_donate_menu_item.up.sql'
WHERE EXISTS (SELECT 1 FROM schema_migrations
              WHERE filename = '000958_seed_donate_menu_item.up.sql');

INSERT INTO schema_migrations (filename)
SELECT '000964_tips_checkout_url.up.sql'
WHERE EXISTS (SELECT 1 FROM schema_migrations
              WHERE filename = '000959_tips_checkout_url.up.sql');

INSERT INTO schema_migrations (filename)
SELECT '000965_scam_alert_platform_ref.up.sql'
WHERE EXISTS (SELECT 1 FROM schema_migrations
              WHERE filename = '000960_scam_alert_platform_ref.up.sql');

DELETE FROM schema_migrations
WHERE filename IN (
  '000957_assessment_integrity_fixes.up.sql',
  '000958_seed_donate_menu_item.up.sql',
  '000959_tips_checkout_url.up.sql',
  '000960_scam_alert_platform_ref.up.sql'
);

COMMIT;
