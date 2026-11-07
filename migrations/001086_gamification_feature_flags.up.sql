-- 001086_gamification_feature_flags.up.sql
--
-- Feature-flag conformance for gamification (§42/§64): badges and challenges
-- tied to a gated feature must hide from public catalogue/collection surfaces
-- while that feature is unactivated — a "Country Explorer" (wiki.*) slot must
-- not render when wiki.enabled is off, matching the menu_items.feature_flag
-- contract (000953). NULL = always shown.
--
-- Backfill precedence: trigger_activity_key / activity_key mapping wins
-- (the flag that gates the emitting route); NULL-key manual badges fall back
-- to a category-level mapping where the category belongs to a single feature.
-- Mixed/core categories (getting_started, community, content, consistency,
-- financial, special) stay NULL.

ALTER TABLE public.badge_definitions
    ADD COLUMN IF NOT EXISTS feature_flag text;

ALTER TABLE public.challenge_definitions
    ADD COLUMN IF NOT EXISTS feature_flag text;

COMMENT ON COLUMN public.badge_definitions.feature_flag IS
    'Optional feature flag key (feature_flags.key). If set, the badge is hidden from public catalogue/collection surfaces and its trigger is skipped while the flag is disabled. Earned awards stay visible.';

COMMENT ON COLUMN public.challenge_definitions.feature_flag IS
    'Optional feature flag key (feature_flags.key). If set, the challenge is hidden from public challenge surfaces and progress is not incremented while the flag is disabled. Completed progress stays visible.';

-- -------------------------------------------------------------------------
-- 1. activity_key → flag backfill (the flag that gates the emitting route).
-- -------------------------------------------------------------------------
UPDATE public.badge_definitions bd
SET feature_flag = m.flag
FROM (VALUES
        ('follow.created',                    'feature.follow'),
        ('reaction.added',                    'feature.reactions'),
        ('forum.thread_created',              'feature.forum'),
        ('forum.solution_marked',             'feature.forum'),
        ('forum.poll_voted',                  'feature.forum'),
        ('forum.poll_created',                'feature.forum'),
        ('wiki.country_viewed',               'wiki.enabled'),
        ('wiki.city_viewed',                  'wiki.enabled'),
        ('wiki.page_viewed',                  'wiki.enabled'),
        ('comparison.completed',              'feature.comparison'),
        ('ai.conversation_started',           'feature.japabot'),
        ('mentorship.requested',              'mentorship.enabled'),
        ('mentorship.connected',              'mentorship.enabled'),
        ('mentorship.completed',              'mentorship.enabled'),
        ('mentorship.mentor_completed',       'mentorship.enabled'),
        ('referral.created',                  'feature.referral'),
        ('referral.completed',                'feature.referral'),
        ('scam_alert.confirmed',              'scam_alert.enabled'),
        ('scam_report.submitted',             'scam_alert.enabled'),
        ('story.created',                     'story.enabled'),
        ('event.rsvp',                        'calendar.enabled'),
        ('achievement.created',               'achievement.enabled'),
        ('service.saved',                     'service_directory.enabled'),
        ('service.review_submitted',          'service_directory.enabled'),
        ('tool.services_directory.used',      'service_directory.enabled'),
        ('visa_timeline.milestone',           'visa_timeline.enabled'),
        ('tool.processing_tracker.used',      'visa_timeline.enabled'),
        -- journey.milestone.* emits via POST /me/visa-timelines/{id}/milestones
        -- (nextactions body route rule) → visa_timeline.enabled.
        ('journey.milestone.application_submitted', 'visa_timeline.enabled'),
        ('journey.milestone.decision_received',     'visa_timeline.enabled'),
        ('journey.milestone.visa_approved',         'visa_timeline.enabled'),
        ('journey.milestone.arrived',               'visa_timeline.enabled'),
        ('journey.milestone.one_year',              'visa_timeline.enabled'),
        ('journey.milestone.university_accepted',   'visa_timeline.enabled'),
        ('journey.milestone.job_offer',             'visa_timeline.enabled'),
        ('journey.milestone.citizenship',           'visa_timeline.enabled'),
        ('checklist.item_checked',            'checklist.enabled'),
        ('tool.arrival_checklist.used',       'arrival_checklist.enabled'),
        ('tool.attestation_guide.used',       'attestation_guide.enabled'),
        ('tool.housing_guide.used',           'housing_guide.enabled'),
        ('tool.visa_routes.used',             'visa_routes.enabled'),
        ('tool.cost_of_living.used',          'col_calculator.enabled'),
        ('tool.remittance.used',              'remittance.enabled'),
        ('tool.travel.used',                  'travel.enabled'),
        ('tool.scholarships.used',            'scholarship_finder.enabled'),
        ('scholarship.saved',                 'scholarship_finder.enabled'),
        ('scholarship.application_started',   'scholarship_finder.enabled'),
        ('scholarship.application_submitted', 'scholarship_finder.enabled'),
        ('tool.jobs_board.used',              'jobs.enabled'),
        ('job.saved',                         'jobs.enabled'),
        ('tool.readiness_assessment.used',    'assessment.enabled'),
        ('assessment.completed',              'assessment.enabled'),
        ('academy.lesson_completed',          'feature.academy'),
        ('academy.quiz_passed',               'feature.academy'),
        ('academy.quiz_perfect',              'feature.academy'),
        ('academy.track_completed',           'feature.academy')
     ) AS m(activity_key, flag)
WHERE bd.trigger_activity_key = m.activity_key
  AND bd.feature_flag IS NULL;

-- -------------------------------------------------------------------------
-- 2. Category fallback — NULL-key (manual) badges in single-feature
--    categories inherit the category's flag. Activity-keyed rows are already
--    set above and are untouched.
-- -------------------------------------------------------------------------
UPDATE public.badge_definitions bd
SET feature_flag = m.flag
FROM (VALUES
        ('academy',     'feature.academy'),
        ('ai',          'feature.japabot'),
        ('mentorship',  'mentorship.enabled'),
        ('comparison',  'feature.comparison'),
        ('growth',      'feature.referral'),
        ('documents',   'attestation_guide.enabled'),
        ('housing',     'housing_guide.enabled'),
        ('services',    'service_directory.enabled'),
        ('trust',       'scam_alert.enabled'),
        ('career',      'jobs.enabled'),
        ('journey',     'visa_timeline.enabled'),
        ('countries',   'wiki.enabled'),
        ('education',   'scholarship_finder.enabled'),
        ('travel',      'travel.enabled'),
        ('readiness',   'assessment.enabled'),
        ('forum',       'feature.forum')
     ) AS m(category, flag)
WHERE bd.category = m.category
  AND bd.trigger_activity_key IS NULL -- keyed rows only take a flag via the
                                      -- activity map; a key the map doesn't
                                      -- cover means the emit route is ungated
                                      -- (comments, profile, universities,
                                      -- identity verification) → stay NULL
  AND bd.feature_flag IS NULL;
