-- 000000_register_renumbered_migrations.sql
-- Renumber registry for duplicate version-prefix collisions.
--
-- Several migrations were committed with a version prefix already taken by
-- another file (parallel branches). The schema_migrations table tracks
-- applied work by FILENAME, so a straight rename would make the runner see
-- the renamed file as pending and re-apply it — fatal for DDL bodies and
-- noisy even for idempotent seeds.
--
-- This file runs first alphabetically and pre-records the new filename for
-- every renumbering whose ORIGINAL filename is already present in
-- schema_migrations. Databases that applied the original file therefore
-- skip the renamed copy; fresh databases (where the original filename is
-- absent) register nothing and apply the renamed bodies at their new tail
-- positions.
--
-- The custom runner (internal/migrate, cmd/migrate) applies this file — a
-- plain .sql forward migration — like any other; golang-migrate ignores it
-- (no .up/.down suffix), which keeps it out of the golang-migrate version
-- sequence entirely.

INSERT INTO schema_migrations (filename)
SELECT v.new_filename
FROM (VALUES
    ('000405_seed_ai_agent_phase9_tools.up.sql',              '000400_seed_ai_agent_phase9_tools.up.sql'),
    ('000688_ownership_claims_unique_pending.up.sql',         '000412_ownership_claims_unique_pending.up.sql'),
    ('000689_referral_use_all_roles.up.sql',                  '000415_referral_use_all_roles.up.sql'),
    ('000690_seed_reports_rbac_nodes.up.sql',                 '000417_seed_reports_rbac_nodes.up.sql'),
    ('000691_ip_blocks_blocked_by_set_null.up.sql',           '000420_ip_blocks_blocked_by_set_null.up.sql'),
    ('000692_studio_created_by_nullable.up.sql',              '000421_studio_created_by_nullable.up.sql'),
    ('000693_intel_sources_view_permission.up.sql',           '000435_intel_sources_view_permission.up.sql'),
    ('000694_seed_attestation_guide_feature_flag.up.sql',     '000454_seed_attestation_guide_feature_flag.up.sql'),
    ('000695_reseed_blog_edit_permission_nodes.up.sql',       '000458_reseed_blog_edit_permission_nodes.up.sql'),
    ('000696_seed_calendar_categories_setting.up.sql',        '000458_seed_calendar_categories_setting.up.sql'),
    ('000697_remove_blog_review_create_from_member.up.sql',   '000459_remove_blog_review_create_from_member.up.sql'),
    ('000698_seed_report_reasons_setting.up.sql',             '000466_seed_report_reasons_setting.up.sql'),
    ('000699_seed_visa_guide_request_form.up.sql',            '000509_seed_visa_guide_request_form.up.sql'),
    ('000700_remove_wallet_dashboard_rail.up.sql',            '000600_remove_wallet_dashboard_rail.up.sql'),
    ('000701_unify_user_tiers.up.sql',                        '000606_unify_user_tiers.up.sql'),
    ('000702_content_reports_source_entity.up.sql',           '000662_content_reports_source_entity.up.sql'),
    ('000703_restore_video_sources_constraints.up.sql',       '000671_restore_video_sources_constraints.up.sql'),
    ('000704_restore_push_referral_wallet_constraints.up.sql','000674_restore_push_referral_wallet_constraints.up.sql')
) AS v(new_filename, old_filename)
WHERE EXISTS (
    SELECT 1 FROM schema_migrations sm WHERE sm.filename = v.old_filename
)
ON CONFLICT (filename) DO NOTHING;
