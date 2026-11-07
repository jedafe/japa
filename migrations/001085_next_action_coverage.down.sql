-- 001085_next_action_coverage.down.sql — removes the coverage seed rules and
-- restores the 000913 rows verbatim (payload cta_url + conditions — the up
-- migration's dead-CTA repair and feature_enabled guards are reverted).

BEGIN;

DELETE FROM next_action_rules WHERE slug IN (
    'member-finish-onboarding', 'member-verify-email',
    'member-resume-readiness-draft', 'canada-pof-calculator',
    'guest-try-readiness', 'member-arrival-checklist',
    'stage-compare-visa-routes', 'member-cost-of-living',
    'member-compare-universities', 'member-language-test', 'member-eca-guide',
    'member-check-credentials', 'member-find-scholarships',
    'member-strong-score-checklist', 'member-improve-score-academy',
    'member-prepare-documents', 'member-attestation-guide',
    'member-embassy-alerts', 'member-visa-checklist',
    'member-track-processing', 'member-japa-calendar', 'member-travel-prep',
    'member-join-planning-group', 'member-ask-ama', 'member-find-mentor',
    'member-complete-profile', 'member-browse-jobs', 'member-find-community',
    'member-mutual-aid', 'member-housing-guide', 'member-send-money-home',
    'member-currency-converter', 'member-tax-calculator',
    'member-browse-services', 'member-explore-academy', 'member-japa-roadmap',
    'member-enable-push', 'member-earn-achievements', 'member-read-wiki',
    'member-set-japa-stage', 'member-join-forum', 'member-scam-alert-check',
    'member-share-story', 'member-digest-subscribe'
);

UPDATE next_action_rules
SET payload = jsonb_set(payload, '{cta_url}', '"/tools/readiness-assessment"'),
    conditions = '{"all":[{"predicate":"account_age_days","lt":30},{"predicate":"assessment_completed","eq":false}]}',
    audience = '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
    updated_at = now()
WHERE slug = 'member-take-readiness-assessment';

UPDATE next_action_rules
SET conditions = '{"all":[{"predicate":"destination_country","in":["CA"]},{"predicate":"tool_not_used","slug":"crs-calculator"}]}',
    completion_signal = '{"all":[{"predicate":"tool_used","slug":"crs-calculator"}]}',
    audience = '{"user_class":["regular","pro","business","guest"],"surfaces":["main","tools","mobile"]}',
    updated_at = now()
WHERE slug = 'canada-crs-calculator';

UPDATE next_action_rules
SET payload = jsonb_set(payload, '{cta_url}', '"/register"'),
    audience = '{"user_class":["guest"],"surfaces":["main","tools"]}',
    updated_at = now()
WHERE slug IN ('guest-register-after-views', 'guest-register-save-results');

UPDATE next_action_rules
SET audience = '{"user_class":["regular"],"surfaces":["main","tools","mobile"]}',
    updated_at = now()
WHERE slug IN ('free-upgrade-gate-hit', 'free-upgrade-mature');

COMMIT;
