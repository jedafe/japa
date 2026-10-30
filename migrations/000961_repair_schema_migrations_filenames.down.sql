-- Reverses the tracker repair: drops the renumbered filename rows and
-- restores the original filename rows. Does not undo the renamed files
-- themselves — git tracks the renames.
BEGIN;

DELETE FROM schema_migrations
WHERE filename IN (
  '000962_assessment_integrity_fixes.up.sql',
  '000963_seed_donate_menu_item.up.sql',
  '000964_tips_checkout_url.up.sql',
  '000965_scam_alert_platform_ref.up.sql'
);

INSERT INTO schema_migrations (filename)
VALUES
  ('000957_assessment_integrity_fixes.up.sql'),
  ('000958_seed_donate_menu_item.up.sql'),
  ('000959_tips_checkout_url.up.sql'),
  ('000960_scam_alert_platform_ref.up.sql')
ON CONFLICT (filename) DO NOTHING;

COMMIT;
