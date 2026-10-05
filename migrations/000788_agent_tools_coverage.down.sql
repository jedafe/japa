-- 000788 down: remove the coverage seeds and restore the pre-migration
-- drift state (dead v1 rows re-enabled for parity with the baseline seed;
-- tools.list/read back to disabled).
BEGIN;
DELETE FROM ai_tools WHERE name IN ('forum.threads.list', 'forum.threads.read', 'forum.threads.create', 'forum.threads.update', 'forum.threads.delete', 'forum.threads.solve', 'forum.replies.list', 'forum.replies.create', 'forum.replies.update', 'forum.replies.delete', 'forum.vote', 'comments.list', 'comments.create', 'comments.update', 'comments.delete', 'comments.vote', 'crs.calculate', 'pof.calculate', 'fx.convert', 'crs_ruleset.create', 'crs_ruleset.update', 'crs_ruleset.delete', 'crs_ruleset.assign_geo', 'crs_draw.create', 'crs_draw.update', 'crs_draw.delete', 'crs_draw.assign_geo', 'pof_requirement.create', 'pof_requirement.update', 'pof_requirement.delete', 'pof_requirement.assign_geo', 'sop_template.create', 'sop_template.update', 'sop_template.delete', 'sop_template.assign_geo', 'sop_sample.create', 'sop_sample.update', 'sop_sample.delete', 'sop_sample.assign_geo', 'eca_guide.create', 'eca_guide.update', 'eca_guide.delete', 'eca_guide.assign_geo', 'eca_provider.create', 'eca_provider.update', 'eca_provider.delete', 'eca_provider.assign_geo');
UPDATE ai_tools SET is_enabled = true, config = config - 'is_blocked', updated_at = now()
WHERE name IN (
    'chat.japabot','platform.translate_string','moderation.flag_content',
    'moderation.classify_severity','moderation.detect_spam','content.editor_assist',
    'content.generate_draft','content.generate_og','content.improve',
    'content.suggest_tags','content.summarize'
);
UPDATE ai_tools SET required_permission = NULL, updated_at = now() WHERE name = 'moderation.detect_spam';
UPDATE ai_tools SET is_enabled = false, updated_at = now() WHERE name IN ('tools.list','tools.read');
UPDATE ai_tools SET is_enabled = true, updated_at = now()
WHERE name IN (
    'attestation_guide.update','attestation_guide.delete','attestation_guide.assign_geo',
    'embassy.delete','embassy.assign_geo','housing_guide.delete','housing_guide.assign_geo',
    'mentorship_connection.update','mentorship_connection.delete','mentorship_connection.assign_geo',
    'scam_report.delete','wiki_page.assign_geo',
    'appointment_alert.assign_geo','credential_profession.assign_geo','language_test.assign_geo',
    'mutual_aid.assign_geo','remittance_provider.assign_geo','remittance_rate.assign_geo',
    'tax_obligation_guide.assign_geo','tax_treaty_pair.assign_geo','travel_provider.assign_geo',
    'visa_timeline.assign_geo'
);
COMMIT;
