-- 001085_next_action_coverage.up.sql — §122 Next-Action Suggestion Engine.
-- The 000913 seed shipped six launch rules; every other predicate in the
-- registry (onboarding_completed, email_verified, profile_completeness,
-- has_draft, has_push_subscription, japa_stage, archetype, assessment_score,
-- feature_used/not_used, feature_enabled) and most journey stages, tools and
-- community features had no coverage. This seeds the remaining japa.to use
-- cases, repairs two dead CTA paths, and adds feature_enabled guards so a
-- rule never nudges a user into a feature the flag gate would 404.
-- Ref: .jeos/sections/122-next-action-suggestions.md

BEGIN;

-- ── Dead CTA repair + flag guards on the 000913 launch rules ────────────────
-- /tools/readiness-assessment → /tools/readiness; /register → /signup.
-- The readiness + CRS rules also get feature_enabled conditions so they
-- suppress while assessment.enabled / crs_calculator.enabled are off.
UPDATE next_action_rules
SET payload = jsonb_set(payload, '{cta_url}', '"/tools/readiness"'),
    conditions = '{"all":[{"predicate":"feature_enabled","key":"assessment.enabled"},{"predicate":"account_age_days","lt":30},{"predicate":"assessment_completed","eq":false}]}',
    audience = '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile","academy","docs"]}',
    updated_at = now()
WHERE slug = 'member-take-readiness-assessment';

UPDATE next_action_rules
SET conditions = '{"all":[{"predicate":"feature_enabled","key":"crs_calculator.enabled"},{"predicate":"destination_country","in":["CA"]},{"predicate":"tool_not_used","slug":"crs-calculator","within_days":3650}]}',
    completion_signal = '{"all":[{"predicate":"tool_used","slug":"crs-calculator","within_days":3650}]}',
    audience = '{"user_class":["regular","pro","business","guest"],"surfaces":["main","tools","mobile","academy","docs"]}',
    updated_at = now()
WHERE slug = 'canada-crs-calculator';

UPDATE next_action_rules
SET payload = jsonb_set(payload, '{cta_url}', '"/signup"'),
    audience = '{"user_class":["guest"],"surfaces":["main","tools","academy","docs"]}',
    updated_at = now()
WHERE slug IN ('guest-register-after-views', 'guest-register-save-results');

-- The upsell popups/bars render on academy + docs too — without those
-- surfaces the launch rules were unreachable from the two newest surfaces.
UPDATE next_action_rules
SET audience = '{"user_class":["regular"],"surfaces":["main","tools","mobile","academy","docs"]}',
    updated_at = now()
WHERE slug IN ('free-upgrade-gate-hit', 'free-upgrade-mature');

-- ── Coverage rules ───────────────────────────────────────────────────────────
-- payload.badge is an optional eyebrow label shown above the title (additive
-- key — renderers that predate it ignore it). Conditions use only
-- code-registered predicates; every rule whose CTA lands on a route wrapped
-- by features.Gate carries a matching feature_enabled condition, so disabling
-- a feature withdraws its nudges instead of pointing users at 404s.
-- feature_used/not_used slugs are the flag key minus ".enabled" — the
-- platform_events feed emits "feature."+flag (features.SetUsageTracker), so
-- feature.academy → slug "feature.academy".
-- japa_stage predicates match the canonical §87 vocabulary (considering|
-- planning|preparing|applied|approved|relocated|settled|traveled|returned).
-- The engine resolves it from user_profiles.japa_stage, then the VisitorContext
-- (legacy japa_status values, assessment answers and journey-cookie stages all
-- normalise into the same vocabulary); "" matches undeclared stages.
INSERT INTO next_action_rules
    (slug, name, description, enabled, priority, audience, conditions, payload, channels, completion_signal, frequency)
VALUES
    -- 1. Finish onboarding — the §122 spec use-case the launch seed missed.
    ('member-finish-onboarding', 'Finish setting up your account',
     'Members who never completed the onboarding flow',
     true, 5,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile","academy","docs"]}',
     '{"all":[{"predicate":"account_age_days","lt":30},{"predicate":"onboarding_completed","eq":false}]}',
     '{"badge":"Getting started","title":"Finish your setup","body":"Answer a few questions so JapaConnect can personalise your roadmap.","cta_label":"Continue setup","cta_url":"/onboarding","icon":"compass"}',
     '["popup","notification"]',
     '{"all":[{"predicate":"onboarding_completed","eq":true}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":24,"snooze_default_hours":72}'),

    -- 2. Verify email — unverified members past day 1.
    ('member-verify-email', 'Verify your email address',
     'Members older than one day with an unverified email',
     true, 15,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile","academy","docs"]}',
     '{"all":[{"predicate":"account_age_days","gt":1},{"predicate":"email_verified","eq":false}]}',
     '{"badge":"Account","title":"Confirm your email","body":"Verify your address to secure your account and get critical journey updates.","cta_label":"Verify email","cta_url":"/verify-email","icon":"mail"}',
     '["announcement_bar","notification"]',
     '{"all":[{"predicate":"email_verified","eq":true}]}',
     '{"mode":"recurring","max_impressions":10,"cooldown_hours":24,"snooze_default_hours":72}'),

    -- 3. Resume a readiness draft — members with a saved form_drafts row.
    ('member-resume-readiness-draft', 'Finish your readiness assessment',
     'Members with an unsubmitted readiness draft',
     true, 24,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile","academy","docs"]}',
     '{"all":[{"predicate":"feature_enabled","key":"assessment.enabled"},{"predicate":"has_draft","eq":true,"slug":"readiness"},{"predicate":"assessment_completed","eq":false}]}',
     '{"badge":"Pick up where you left off","title":"Your assessment is saved","body":"Your answers are waiting — finish the readiness assessment to get your personalised roadmap.","cta_label":"Resume assessment","cta_url":"/tools/readiness","icon":"file-text"}',
     '["popup"]',
     '{"all":[{"predicate":"assessment_completed","eq":true}]}',
     '{"mode":"recurring","max_impressions":5,"cooldown_hours":48,"snooze_default_hours":168}'),

    -- 4. Canada proof-of-funds — the natural follow-up to the CRS rule.
    ('canada-pof-calculator', 'Check your proof of funds',
     'Canada-bound users who have not used the POF calculator',
     true, 26,
     '{"user_class":["regular","pro","business","guest"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"pof_calculator.enabled"},{"predicate":"destination_country","in":["CA"]},{"predicate":"tool_not_used","slug":"pof-calculator","within_days":3650}]}',
     '{"badge":"Canada Express Entry","title":"Will your funds qualify?","body":"Check the proof-of-funds amount IRCC expects for your family size.","cta_label":"Check funds","cta_url":"/tools/pof-calculator","icon":"piggy-bank"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"tool_used","slug":"pof-calculator","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":8,"cooldown_hours":72,"snooze_default_hours":168}'),

    -- 5. Guest → flagship tool — let visitors experience the assessment
    -- before asking them to register (register rules then close the loop).
    ('guest-try-readiness', 'Try the readiness assessment',
     'Engaged guests who have not tried the readiness assessment',
     true, 35,
     '{"user_class":["guest"],"surfaces":["main","tools","academy","docs"]}',
     '{"all":[{"predicate":"feature_enabled","key":"assessment.enabled"},{"predicate":"assessment_completed","eq":false},{"predicate":"event_count","type":"content.viewed","gte":2,"window":"7d"}]}',
     '{"badge":"Free tool","title":"How ready are you to japa?","body":"Answer a few questions and get an instant readiness score — no account needed.","cta_label":"Try it free","cta_url":"/tools/readiness","icon":"target"}',
     '["announcement_bar","in_article"]',
     NULL,
     '{"mode":"recurring","max_impressions":4,"cooldown_hours":48,"snooze_default_hours":168}'),

    -- 6. Pre-departure — approved members need the arrival checklist.
    ('member-arrival-checklist', 'Prepare for arrival',
     'Approved-stage members who have not used the arrival checklist',
     true, 42,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile","academy","docs"]}',
     '{"all":[{"predicate":"feature_enabled","key":"arrival_checklist.enabled"},{"predicate":"japa_stage","in":["approved"]},{"predicate":"tool_not_used","slug":"arrival-checklist","within_days":3650}]}',
     '{"badge":"Pre-departure","title":"Visa approved — now what?","body":"Work through the arrival checklist so nothing is left for the airport.","cta_label":"Open checklist","cta_url":"/tools/arrival-checklist","icon":"plane"}',
     '["announcement_bar","sidebar","notification"]',
     '{"all":[{"predicate":"tool_used","slug":"arrival-checklist","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":8,"cooldown_hours":72,"snooze_default_hours":168}'),

    -- 7. Early-stage route discovery — considering/planning members who have
    -- never compared visa routes (non-Canada destinations get coverage too).
    ('stage-compare-visa-routes', 'Compare your visa options',
     'Considering/planning members who have not used visa routes',
     true, 45,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile","academy","docs"]}',
     '{"all":[{"predicate":"feature_enabled","key":"visa_routes.enabled"},{"predicate":"japa_stage","in":["considering","planning"]},{"predicate":"tool_not_used","slug":"visa-routes","within_days":3650}]}',
     '{"badge":"Journey planning","title":"Which route fits you?","body":"Compare visa routes side by side — requirements, timelines, and costs.","cta_label":"Compare routes","cta_url":"/tools/visa-routes","icon":"route"}',
     '["announcement_bar","sidebar","in_article"]',
     '{"all":[{"predicate":"tool_used","slug":"visa-routes","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":8,"cooldown_hours":72,"snooze_default_hours":168}'),

    -- 8. Cost of living — the deciding-destinations tool for early stages.
    ('member-cost-of-living', 'Compare cost of living',
     'Considering/planning users who have not compared cost of living',
     true, 46,
     '{"user_class":["regular","pro","business","guest"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"col_calculator.enabled"},{"predicate":"japa_stage","in":["considering","planning"]},{"predicate":"tool_not_used","slug":"cost-of-living","within_days":3650}]}',
     '{"badge":"Journey planning","title":"What will life cost there?","body":"Compare rent, food and transport across your shortlist destinations.","cta_label":"Compare costs","cta_url":"/tools/cost-of-living","icon":"calculator"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"tool_used","slug":"cost-of-living","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 9. Comparison engine — side-by-side schools/cities for early stages.
    -- The JCE group gates on BOTH feature.comparison and feature.jce.
    ('member-compare-universities', 'Compare options side by side',
     'Considering/planning members who have not run a comparison',
     true, 47,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"feature.comparison"},{"predicate":"feature_enabled","key":"feature.jce"},{"predicate":"japa_stage","in":["considering","planning"]},{"predicate":"tool_not_used","slug":"comparison-tool","within_days":3650}]}',
     '{"badge":"Journey planning","title":"See options side by side","body":"Compare universities, cities and routes on the factors that matter to you.","cta_label":"Start a comparison","cta_url":"/compare","icon":"list-checks"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"tool_used","slug":"comparison-tool","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 10. Language test — destinations where an English/French test gates the
    -- application, targeted at pre-application stages.
    ('member-language-test', 'Plan your language test',
     'Members heading to language-test destinations who have not used the hub',
     true, 55,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"language_test_hub.enabled"},{"predicate":"destination_country","in":["CA","GB","US","AU","IE","NZ","FR","DE"]},{"predicate":"japa_stage","in":["considering","planning","preparing"]},{"predicate":"tool_not_used","slug":"language-test","within_days":3650}]}',
     '{"badge":"Documents","title":"IELTS, PTE or TEF?","body":"Find the right language test for your route and the score you need.","cta_label":"Compare tests","cta_url":"/tools/language-test","icon":"languages"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"tool_used","slug":"language-test","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 11. Credential evaluation — preparing members headed to destinations
    -- that evaluate foreign credentials (CA, US).
    ('member-eca-guide', 'Get your credentials evaluated',
     'Preparing members bound for ECA destinations',
     true, 56,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"eca_guide.enabled"},{"predicate":"destination_country","in":["CA","US"]},{"predicate":"japa_stage","in":["planning","preparing"]},{"predicate":"tool_not_used","slug":"eca-guide","within_days":3650}]}',
     '{"badge":"Documents","title":"Are your degrees recognised?","body":"See which evaluation body your route needs and what it costs.","cta_label":"Open ECA guide","cta_url":"/tools/eca-guide","icon":"scroll-text"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"tool_used","slug":"eca-guide","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 12. Credential checker — same phase, quick eligibility probe before the
    -- full ECA guide.
    ('member-check-credentials', 'Check your credential eligibility',
     'Preparing members headed to CA/US who have not checked credentials',
     true, 57,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"credential_checker.enabled"},{"predicate":"destination_country","in":["CA","US"]},{"predicate":"japa_stage","in":["planning","preparing"]},{"predicate":"tool_not_used","slug":"credential-checker","within_days":3650}]}',
     '{"badge":"Documents","title":"Will your credentials count?","body":"Check how your qualifications map to your destination’s requirements.","cta_label":"Check credentials","cta_url":"/tools/credential-checker","icon":"scroll-text"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"tool_used","slug":"credential-checker","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 13. Scholarships — funding search for students still planning the move.
    ('member-find-scholarships', 'Find scholarships you qualify for',
     'Early-stage members who have not searched scholarships',
     true, 57,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"scholarship_finder.enabled"},{"predicate":"japa_stage","in":["considering","planning","preparing"]},{"predicate":"tool_not_used","slug":"scholarships","within_days":3650}]}',
     '{"badge":"Funding","title":"Don’t self-fund what you don’t have to","body":"Search scholarships matched to your destination, field and profile.","cta_label":"Find scholarships","cta_url":"/scholarships","icon":"graduation-cap"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"tool_used","slug":"scholarships","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 14. Strong assessment score → start the visa checklist now.
    ('member-strong-score-checklist', 'You are ready — start the checklist',
     'Members scoring 70+ who have not opened the visa checklist',
     true, 44,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile","academy","docs"]}',
     '{"all":[{"predicate":"feature_enabled","key":"checklist.enabled"},{"predicate":"assessment_score","gte":70},{"predicate":"japa_stage","in":["planning","preparing"]},{"predicate":"tool_not_used","slug":"visa-checklist","within_days":3650}]}',
     '{"badge":"Strong result","title":"Your score says go","body":"A 70+ readiness score means it is time to tick off your application documents.","cta_label":"Open checklist","cta_url":"/tools/visa-checklist","icon":"rocket"}',
     '["popup","sidebar"]',
     '{"all":[{"predicate":"tool_used","slug":"visa-checklist","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":4,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 15. Low assessment score → academy tracks build the weak areas.
    ('member-improve-score-academy', 'Build your weak areas first',
     'Members scoring under 50 who have not opened the academy',
     true, 44,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile","docs"]}',
     '{"all":[{"predicate":"feature_enabled","key":"feature.academy"},{"predicate":"assessment_completed","eq":true},{"predicate":"assessment_score","lt":50},{"predicate":"feature_not_used","slug":"feature.academy","within_days":3650}]}',
     '{"badge":"Learn","title":"Close the gaps before you apply","body":"Your assessment flagged weak areas — free academy tracks build exactly those.","cta_label":"Start a track","cta_url":"/academy","icon":"book-open"}',
     '["popup","sidebar"]',
     '{"all":[{"predicate":"feature_used","slug":"feature.academy","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":4,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 16. Document toolkit — preparing/applying members who have touched no
    -- document tool yet; completes when ANY document tool is used.
    ('member-prepare-documents', 'Prepare your documents',
     'Preparing/applying members who have used no document tool',
     true, 58,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"cv_formatter.enabled"},{"predicate":"japa_stage","in":["preparing","applied"]},{"predicate":"tool_not_used","slug":"cv-formatter","within_days":3650},{"predicate":"tool_not_used","slug":"sop-generator","within_days":3650}]}',
     '{"badge":"Documents","title":"Get application-ready","body":"Format your CV to destination standards and draft a statement of purpose that lands.","cta_label":"Format your CV","cta_url":"/tools/cv-formatter","icon":"file-text"}',
     '["sidebar","in_article"]',
     '{"any":[{"predicate":"tool_used","slug":"cv-formatter","within_days":3650},{"predicate":"tool_used","slug":"sop-generator","within_days":3650},{"predicate":"tool_used","slug":"eca-guide","within_days":3650},{"predicate":"tool_used","slug":"credential-checker","within_days":3650},{"predicate":"tool_used","slug":"attestation-guide","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":8,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 17. Attestation — the Nigeria-specific document step most applicants
    -- discover too late.
    ('member-attestation-guide', 'Get your documents attested',
     'Nigerian-origin members in document phase who have not read the guide',
     true, 59,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"attestation_guide.enabled"},{"predicate":"origin_country","in":["NG"]},{"predicate":"japa_stage","in":["preparing","applied"]},{"predicate":"tool_not_used","slug":"attestation-guide","within_days":3650}]}',
     '{"badge":"Documents","title":"The step most people miss","body":"Which documents need attestation, where to go, and how long it takes.","cta_label":"Read attestation guide","cta_url":"/tools/attestation-guide","icon":"scroll-text"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"tool_used","slug":"attestation-guide","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 18. Embassy appointment alerts — the wait after applying.
    ('member-embassy-alerts', 'Get embassy appointment alerts',
     'Applied-stage members who have not set an embassy alert',
     true, 62,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"appointment_alert.enabled"},{"predicate":"japa_stage","in":["applied","approved"]},{"predicate":"tool_not_used","slug":"embassy-alerter","within_days":3650}]}',
     '{"badge":"Application stage","title":"Never miss an appointment slot","body":"Get alerted the moment your embassy opens new slots.","cta_label":"Set an alert","cta_url":"/tools/embassy-alerter","icon":"landmark"}',
     '["sidebar"]',
     '{"all":[{"predicate":"tool_used","slug":"embassy-alerter","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 19. Visa checklist — document tracking while the application is in.
    ('member-visa-checklist', 'Track your application documents',
     'Applied-stage members who have not used the visa checklist',
     true, 63,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"checklist.enabled"},{"predicate":"japa_stage","in":["applied"]},{"predicate":"tool_not_used","slug":"visa-checklist","within_days":3650}]}',
     '{"badge":"Application stage","title":"Every document, accounted for","body":"Tick off each item your route needs before the decision lands.","cta_label":"Open checklist","cta_url":"/tools/visa-checklist","icon":"list-checks"}',
     '["sidebar"]',
     '{"all":[{"predicate":"tool_used","slug":"visa-checklist","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 20. Processing-time tracker — anxiety relief during the wait.
    ('member-track-processing', 'Track processing times',
     'Applied-stage members who have not used the processing tracker',
     true, 64,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"visa_timeline.enabled"},{"predicate":"japa_stage","in":["applied"]},{"predicate":"tool_not_used","slug":"processing-tracker","within_days":3650}]}',
     '{"badge":"Application stage","title":"How long is the wait?","body":"See real processing times reported by members on your route.","cta_label":"See wait times","cta_url":"/tools/processing-tracker","icon":"timer"}',
     '["sidebar"]',
     '{"all":[{"predicate":"tool_used","slug":"processing-tracker","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 21. Japa calendar — deadlines and events for the application phase.
    ('member-japa-calendar', 'Never miss a japa deadline',
     'Preparing/applied members who have not opened the calendar',
     true, 65,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"calendar.enabled"},{"predicate":"japa_stage","in":["preparing","applied"]},{"predicate":"feature_not_used","slug":"calendar","within_days":3650}]}',
     '{"badge":"Application stage","title":"Deadlines, in one place","body":"Application windows, embassy dates and community events on your calendar.","cta_label":"Open calendar","cta_url":"/calendar","icon":"timer"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"feature_used","slug":"calendar","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":120,"snooze_default_hours":168}'),

    -- 22. Travel prep — approved members booking the actual move.
    ('member-travel-prep', 'Plan the move itself',
     'Approved members who have not used the travel planner',
     true, 66,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"travel.enabled"},{"predicate":"japa_stage","in":["approved"]},{"predicate":"feature_not_used","slug":"travel","within_days":3650}]}',
     '{"badge":"Pre-departure","title":"Time to book the move","body":"Flights, luggage allowances and first-week logistics, planned in one place.","cta_label":"Plan travel","cta_url":"/tools/travel","icon":"plane"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"feature_used","slug":"travel","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":120,"snooze_default_hours":168}'),

    -- 23. Planning groups — community for people relocating together.
    ('member-join-planning-group', 'Join a planning group',
     'Early/mid-stage members who have not opened planning groups',
     true, 72,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"planning_group.enabled"},{"predicate":"japa_stage","in":["considering","planning","preparing"]},{"predicate":"feature_not_used","slug":"planning_group","within_days":3650}]}',
     '{"badge":"Community","title":"Don’t japa alone","body":"Join a cohort planning the same move — share timelines, tips and morale.","cta_label":"Find your group","cta_url":"/planning-groups","icon":"users"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"feature_used","slug":"planning_group","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":120,"snooze_default_hours":168}'),

    -- 24. Ask the community — AMA is where early-stage questions get answered.
    ('member-ask-ama', 'Ask people who already japa’d',
     'Early-stage members who have not opened AMA',
     true, 73,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"forum.ama.enabled"},{"predicate":"japa_stage","in":["considering","planning","preparing"]},{"predicate":"feature_not_used","slug":"forum.ama","within_days":3650}]}',
     '{"badge":"Community","title":"Your questions, answered","body":"Post your questions in AMA sessions with members on the other side.","cta_label":"See AMA sessions","cta_url":"/ama","icon":"users"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"feature_used","slug":"forum.ama","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":120,"snooze_default_hours":168}'),

    -- 25. Mentorship — matched guidance from members who already made it.
    ('member-find-mentor', 'Talk to someone who has done it',
     'Early-stage members who have not opened mentorship',
     true, 74,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"mentorship.enabled"},{"predicate":"japa_stage","in":["considering","planning","preparing"]},{"predicate":"feature_not_used","slug":"mentorship","within_days":3650}]}',
     '{"badge":"Community","title":"Learn from an expat","body":"Get matched with a mentor who already made the move you are planning.","cta_label":"Find a mentor","cta_url":"/mentors","icon":"graduation-cap"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"feature_used","slug":"mentorship","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":120,"snooze_default_hours":168}'),

    -- 26. Profile completeness — onboarded members with thin profiles.
    ('member-complete-profile', 'Complete your profile',
     'Onboarded members past week one with a profile under 60%',
     true, 76,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"onboarding_completed","eq":true},{"predicate":"account_age_days","gt":7},{"predicate":"profile_completeness","lt":60}]}',
     '{"badge":"Profile","title":"A stronger profile, better matches","body":"Complete your profile to unlock better recommendations and community trust.","cta_label":"Complete profile","cta_url":"/settings/profile","icon":"user-check"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"profile_completeness","gte":60}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 27. Jobs board — members past the visa stage looking for work. The
    -- jobs API isn't Gate-wrapped but the service checks jobs.enabled
    -- internally, so the flag guard is still required; usage rides the
    -- tool.used feed via the "jobs-board" slug in ToolUsageRoutes.
    ('member-browse-jobs', 'Find work in your new home',
     'Post-visa members who could use the jobs board',
     true, 90,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"jobs.enabled"},{"predicate":"japa_stage","in":["approved","relocated","settled"]},{"predicate":"tool_not_used","slug":"jobs-board","within_days":3650}]}',
     '{"badge":"Career","title":"Jobs that sponsor and hire","body":"Browse openings filtered for newcomers — including roles that understand your visa status.","cta_label":"Browse jobs","cta_url":"/jobs","icon":"target"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"tool_used","slug":"jobs-board","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":5,"cooldown_hours":120,"snooze_default_hours":168}'),

    -- 28. New arrival — relocated members find their people.
    ('member-find-community', 'Find your community',
     'Newly arrived members who have not used the community locator',
     true, 92,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"community_locator.enabled"},{"predicate":"japa_stage","in":["relocated","traveled"]},{"predicate":"tool_not_used","slug":"community-locator","within_days":3650}]}',
     '{"badge":"New arrival","title":"Your people are here","body":"Find diaspora communities, associations and events near you.","cta_label":"Locate community","cta_url":"/tools/community-locator","icon":"map-pin"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"tool_used","slug":"community-locator","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 29. Mutual aid — community support for people just landing.
    ('member-mutual-aid', 'Get help settling in',
     'Newly arrived members who have not opened mutual aid',
     true, 93,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"mutual_aid.enabled"},{"predicate":"japa_stage","in":["relocated","traveled"]},{"predicate":"feature_not_used","slug":"mutual_aid","within_days":3650}]}',
     '{"badge":"New arrival","title":"Members helping members","body":"Airport pickups, first-shop runs, document help — ask, or offer.","cta_label":"See mutual aid","cta_url":"/mutual-aid","icon":"users"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"feature_used","slug":"mutual_aid","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":120,"snooze_default_hours":168}'),

    -- 30. Housing — approved and newly arrived members need somewhere to live.
    ('member-housing-guide', 'Sort out your housing',
     'Approved/arrived members who have not read the housing guide',
     true, 94,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"housing_guide.enabled"},{"predicate":"japa_stage","in":["approved","relocated","traveled"]},{"predicate":"tool_not_used","slug":"housing-guide","within_days":3650}]}',
     '{"badge":"Settling in","title":"Rent without the horror stories","body":"What landlords ask for, typical rent ranges, and the scams to avoid.","cta_label":"Read housing guide","cta_url":"/tools/housing-guide","icon":"home"}',
     '["sidebar"]',
     '{"all":[{"predicate":"tool_used","slug":"housing-guide","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":96,"snooze_default_hours":168}'),

    -- 31. Remittance — settled members sending money home.
    ('member-send-money-home', 'Send money home for less',
     'Settled members who have not used the remittance tracker',
     true, 96,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"remittance.enabled"},{"predicate":"japa_stage","in":["settled","returned"]},{"predicate":"tool_not_used","slug":"remittance","within_days":3650}]}',
     '{"badge":"Settled life","title":"Compare remittance rates","body":"See which service gives the best naira rate before you send.","cta_label":"Compare rates","cta_url":"/tools/remittance","icon":"send"}',
     '["sidebar"]',
     '{"all":[{"predicate":"tool_used","slug":"remittance","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":120,"snooze_default_hours":168}'),

    -- 32. Currency converter — settled members tracking the rate daily.
    ('member-currency-converter', 'Check the exchange rate',
     'Settled members who have not used the currency converter',
     true, 97,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"fx.enabled"},{"predicate":"japa_stage","in":["settled","returned","relocated","traveled"]},{"predicate":"tool_not_used","slug":"currency-converter","within_days":3650}]}',
     '{"badge":"Settled life","title":"What is the rate today?","body":"Live conversion across the pairs that matter to home.","cta_label":"Convert now","cta_url":"/tools/currency-converter","icon":"calculator"}',
     '["sidebar"]',
     '{"all":[{"predicate":"tool_used","slug":"currency-converter","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":120,"snooze_default_hours":168}'),

    -- 33. Tax calculator — first tax season abroad confuses everyone.
    ('member-tax-calculator', 'Estimate your taxes abroad',
     'Relocated/settled members who have not used the tax calculator',
     true, 97,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"tax_calculator.enabled"},{"predicate":"japa_stage","in":["relocated","settled"]},{"predicate":"tool_not_used","slug":"tax-calculator","within_days":3650}]}',
     '{"badge":"Settled life","title":"What will you actually keep?","body":"Estimate take-home pay after tax in your new country.","cta_label":"Estimate taxes","cta_url":"/tools/tax-calculator","icon":"calculator"}',
     '["sidebar"]',
     '{"all":[{"predicate":"tool_used","slug":"tax-calculator","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":120,"snooze_default_hours":168}'),

    -- 34. Service directory — vetted movers, agents and lawyers near them.
    ('member-browse-services', 'Find services that get it',
     'Arrived members who have not browsed the service directory',
     true, 97,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"service_directory.enabled"},{"predicate":"japa_stage","in":["relocated","settled","traveled"]},{"predicate":"feature_not_used","slug":"service_directory","within_days":3650}]}',
     '{"badge":"Settled life","title":"Vetted services, no guesswork","body":"Immigration lawyers, movers, agents — reviewed by members like you.","cta_label":"Browse services","cta_url":"/services","icon":"bookmark"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"feature_used","slug":"service_directory","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":120,"snooze_default_hours":168}'),

    -- 35. Academy — assessed members have a roadmap; the academy teaches it.
    ('member-explore-academy', 'Learn the route in the academy',
     'Members with an assessment result who have not opened the academy',
     true, 98,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"feature.academy"},{"predicate":"assessment_completed","eq":true},{"predicate":"feature_not_used","slug":"feature.academy","within_days":3650}]}',
     '{"badge":"Learn","title":"Turn your roadmap into skills","body":"Free tracks walk you through every step your assessment flagged.","cta_label":"Browse tracks","cta_url":"/academy","icon":"book-open"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"feature_used","slug":"feature.academy","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":120,"snooze_default_hours":168}'),

    -- 36. Product roadmap — engaged members vote on what ships next (§113
    -- public board; not to be confused with a personal journey roadmap).
    ('member-japa-roadmap', 'Vote on what ships next',
     'Established members who have not opened the product roadmap',
     true, 99,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"roadmap.enabled"},{"predicate":"account_age_days","gt":7},{"predicate":"feature_not_used","slug":"roadmap","within_days":3650}]}',
     '{"badge":"Have a say","title":"Your vote shapes the platform","body":"See what is planned, in progress and shipped — and vote on what matters to you.","cta_label":"See the roadmap","cta_url":"/roadmap","icon":"route"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"feature_used","slug":"roadmap","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":120,"snooze_default_hours":168}'),

    -- 37. Push opt-in — members without a subscription miss deadline alerts.
    ('member-enable-push', 'Turn on push notifications',
     'Week-old members without a push subscription',
     true, 110,
     '{"user_class":["regular","pro","business"],"surfaces":["main","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"feature.notifications"},{"predicate":"account_age_days","gt":7},{"predicate":"has_push_subscription","eq":false}]}',
     '{"badge":"Stay updated","title":"Deadlines do not wait","body":"Enable push alerts for embassy slots, processing updates and community replies.","cta_label":"Enable alerts","cta_url":"/settings/notifications","icon":"bell"}',
     '["sidebar"]',
     '{"all":[{"predicate":"has_push_subscription","eq":true}]}',
     '{"mode":"recurring","max_impressions":4,"cooldown_hours":168,"snooze_default_hours":336}'),

    -- 38. Achievements — engaged members who never saw the gamified loop.
    ('member-earn-achievements', 'Your journey earns badges',
     'Active members who have not opened achievements',
     true, 111,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"achievement.enabled"},{"predicate":"account_age_days","gt":7},{"predicate":"event_count","type":"content.viewed","gte":3,"window":"30d"},{"predicate":"feature_not_used","slug":"achievement","within_days":3650}]}',
     '{"badge":"Progress","title":"You are already earning badges","body":"See the achievements your journey has unlocked — and what is next.","cta_label":"See achievements","cta_url":"/achievements","icon":"star"}',
     '["sidebar"]',
     '{"all":[{"predicate":"feature_used","slug":"achievement","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":4,"cooldown_hours":168,"snooze_default_hours":336}'),

    -- 39. Wiki — deep guides for researchers who keep reading.
    ('member-read-wiki', 'Go deeper in the wiki',
     'Engaged early-stage members who have not opened the wiki',
     true, 112,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"wiki.enabled"},{"predicate":"japa_stage","in":["considering","planning","preparing"]},{"predicate":"event_count","type":"content.viewed","gte":3,"window":"7d"},{"predicate":"feature_not_used","slug":"wiki","within_days":3650}]}',
     '{"badge":"Learn","title":"Every step, documented","body":"Community-maintained guides on routes, documents and life abroad.","cta_label":"Browse the wiki","cta_url":"/wiki","icon":"book-open"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"feature_used","slug":"wiki","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":4,"cooldown_hours":168,"snooze_default_hours":336}'),

    -- 40. Set japa stage — members who never declared where they are; the
    -- stage drives every other suggestion.
    ('member-set-japa-stage', 'Where are you in your journey?',
     'Onboarded members with no declared japa stage',
     true, 113,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"japa_stage.enabled"},{"predicate":"onboarding_completed","eq":true},{"predicate":"japa_stage","in":[""]}]}',
     '{"badge":"Personalise","title":"Tell us where you are","body":"Set your japa stage so suggestions, tools and your roadmap match your real progress.","cta_label":"Set my stage","cta_url":"/japa-stage/me","icon":"compass"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"japa_stage","in":["considering","planning","preparing","applied","approved","relocated","settled","traveled","returned"]}]}',
     '{"mode":"recurring","max_impressions":4,"cooldown_hours":168,"snooze_default_hours":336}'),

    -- 41. Forum — the community surface for members past day three.
    ('member-join-forum', 'Join the conversation',
     'Established members who have not opened the forum',
     true, 114,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"feature.forum"},{"predicate":"account_age_days","gt":3},{"predicate":"feature_not_used","slug":"feature.forum","within_days":3650}]}',
     '{"badge":"Community","title":"The forum is where members talk","body":"Route advice, document war stories, wins — ask and share.","cta_label":"Open the forum","cta_url":"/forum","icon":"users"}',
     '["sidebar"]',
     '{"all":[{"predicate":"feature_used","slug":"feature.forum","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":4,"cooldown_hours":168,"snooze_default_hours":336}'),

    -- 42. Scam-alert awareness — the safety feature everyone should know.
    ('member-scam-alert-check', 'Check reported scams',
     'Members who have not opened the scam alert board',
     true, 115,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"scam_alert.enabled"},{"predicate":"feature_not_used","slug":"scam_alert","within_days":3650}]}',
     '{"badge":"Stay safe","title":"Know the scams before they find you","body":"Community-verified alerts on visa, housing and job fraud targeting japa candidates.","cta_label":"See alerts","cta_url":"/scam-alerts","icon":"shield-alert"}',
     '["sidebar"]',
     '{"all":[{"predicate":"feature_used","slug":"scam_alert","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":4,"cooldown_hours":168,"snooze_default_hours":336}'),

    -- 43. Share your story — settled members are the proof others need.
    ('member-share-story', 'Share your japa story',
     'Settled members who have not opened stories',
     true, 116,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"story.enabled"},{"predicate":"japa_stage","in":["settled","returned"]},{"predicate":"feature_not_used","slug":"story","within_days":3650}]}',
     '{"badge":"Give back","title":"Someone needs to hear it worked","body":"Your story is the evidence a considering member is looking for.","cta_label":"Share your story","cta_url":"/stories","icon":"sparkles"}',
     '["sidebar","in_article"]',
     '{"all":[{"predicate":"feature_used","slug":"story","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":4,"cooldown_hours":168,"snooze_default_hours":336}'),

    -- 44. Community digest — the low-effort way to stay in the loop.
    ('member-digest-subscribe', 'Get the community digest',
     'Members who have not opened the digest',
     true, 118,
     '{"user_class":["regular","pro","business"],"surfaces":["main","mobile"]}',
     '{"all":[{"predicate":"feature_enabled","key":"digest.enabled"},{"predicate":"feature_not_used","slug":"digest","within_days":3650}]}',
     '{"badge":"Stay informed","title":"Your week in japa, summarised","body":"One digest with the posts, alerts and deadlines that matter to your journey.","cta_label":"See the digest","cta_url":"/digest","icon":"newspaper"}',
     '["sidebar"]',
     '{"all":[{"predicate":"feature_used","slug":"digest","within_days":3650}]}',
     '{"mode":"recurring","max_impressions":4,"cooldown_hours":168,"snooze_default_hours":336}')

-- Environments that received an earlier draft of a seeded slug (manual
-- inserts, dev/staging seed runs) hold stale definitions — reconcile them to
-- the canonical seed instead of failing on the slug unique key.
ON CONFLICT (slug) DO UPDATE SET
    name              = EXCLUDED.name,
    description       = EXCLUDED.description,
    enabled           = EXCLUDED.enabled,
    priority          = EXCLUDED.priority,
    audience          = EXCLUDED.audience,
    conditions        = EXCLUDED.conditions,
    payload           = EXCLUDED.payload,
    channels          = EXCLUDED.channels,
    completion_signal = EXCLUDED.completion_signal,
    frequency         = EXCLUDED.frequency,
    updated_at        = now();

COMMIT;
