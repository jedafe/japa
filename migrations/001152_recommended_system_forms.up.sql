-- 001152_recommended_system_forms.up.sql
-- Seeds the recommended system forms (§06): onboarding intake, event RSVP,
-- partner/university enquiry, mentorship intake, space join application, and
-- the community check-in survey. All are published + is_system and render at
-- the generic /forms/{slug} public renderer (and inside Page Builder
-- form_embed blocks) — guest can_view on every field keeps the public embed
-- working (CF13), non-guest groups can edit.
-- Also wires two surfaces: the /contact "Partnerships & press" card self-linked
-- to /contact — it now points at the partner-enquiry form — and adds a footer
-- link to /feedback, which previously had no navigation entry.
--
-- Ref: .jeos/sections/06-custom-field-builder-form-builder.md (forms, §06
-- JapaStatusOptions, suggest {"source":"geo_countries"})

BEGIN;

-- ── 1. Forms ────────────────────────────────────────────────────────────

INSERT INTO forms (id, name, slug, description, is_multi_step, submit_label, success_message, status, is_system, form_type) VALUES
	('00000000-0000-0000-0000-0000000000f1', 'New Member Onboarding', 'onboarding',
		'Welcome intake for new members — origin, destination, journey stage and goals.',
		false, 'Continue', 'Welcome aboard — thanks for telling us about your move.', 'published', true, 'standard'),
	('00000000-0000-0000-0000-0000000000f2', 'Event / Meetup RSVP', 'event-rsvp',
		'RSVP intake for community-organised meetups and gatherings.',
		false, 'RSVP', 'Thanks — your RSVP has been recorded. See you there!', 'published', true, 'standard'),
	('00000000-0000-0000-0000-0000000000f3', 'Partner & University Enquiry', 'partner-enquiry',
		'Enquiries from universities, employers, community organisations and media.',
		false, 'Send enquiry', 'Thanks — our partnerships team will be in touch.', 'published', true, 'standard'),
	('00000000-0000-0000-0000-0000000000f4', 'Mentorship Intake Questionnaire', 'mentorship-intake',
		'Pre-application questionnaire for the mentorship programme (§102).',
		false, 'Submit questionnaire', 'Thanks — we will use your answers to find the right mentorship match.', 'published', true, 'standard'),
	('00000000-0000-0000-0000-0000000000f5', 'Space Join Application', 'space-application',
		'Application intake for spaces that screen new members.',
		false, 'Submit application', 'Thanks — the space moderators will review your application.', 'published', true, 'standard'),
	('00000000-0000-0000-0000-0000000000f6', 'Community Check-in', 'community-checkin',
		'Periodic pulse survey — how members are settling in and what they need.',
		false, 'Share update', 'Thanks for checking in — your answers help us improve the community.', 'published', true, 'standard')
ON CONFLICT (slug) DO UPDATE SET
	name = EXCLUDED.name,
	description = EXCLUDED.description,
	submit_label = EXCLUDED.submit_label,
	success_message = EXCLUDED.success_message,
	status = 'published';

-- ── 2. Field groups + assignments ────────────────────────────────────────

INSERT INTO custom_field_groups (id, name, slug, description, is_system) VALUES
	('00000000-0000-0000-0000-0000000000a1', 'Onboarding Form Fields',  'form-onboarding-fields',         'Fields for the new-member onboarding intake form.', true),
	('00000000-0000-0000-0000-0000000000a2', 'Event RSVP Form Fields',  'form-event-rsvp-fields',         'Fields for the event/meetup RSVP form.',            true),
	('00000000-0000-0000-0000-0000000000a3', 'Partner Enquiry Fields',  'form-partner-enquiry-fields',    'Fields for the partner & university enquiry form.', true),
	('00000000-0000-0000-0000-0000000000a4', 'Mentorship Intake Fields','form-mentorship-intake-fields',  'Fields for the mentorship intake questionnaire.',   true),
	('00000000-0000-0000-0000-0000000000a5', 'Space Application Fields','form-space-application-fields',  'Fields for the space join application form.',       true),
	('00000000-0000-0000-0000-0000000000a6', 'Community Check-in Fields','form-community-checkin-fields', 'Fields for the community check-in survey.',         true)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g, 'form', f, 0
FROM (VALUES
	('00000000-0000-0000-0000-0000000000a1'::uuid, '00000000-0000-0000-0000-0000000000f1'::uuid),
	('00000000-0000-0000-0000-0000000000a2'::uuid, '00000000-0000-0000-0000-0000000000f2'::uuid),
	('00000000-0000-0000-0000-0000000000a3'::uuid, '00000000-0000-0000-0000-0000000000f3'::uuid),
	('00000000-0000-0000-0000-0000000000a4'::uuid, '00000000-0000-0000-0000-0000000000f4'::uuid),
	('00000000-0000-0000-0000-0000000000a5'::uuid, '00000000-0000-0000-0000-0000000000f5'::uuid),
	('00000000-0000-0000-0000-0000000000a6'::uuid, '00000000-0000-0000-0000-0000000000f6'::uuid)
) AS v(g, f)
WHERE NOT EXISTS (
	SELECT 1 FROM custom_field_group_assignments a
	WHERE a.group_id = v.g AND a.entity_class = 'form' AND a.entity_id = v.f
);

-- ── 3. Fields ────────────────────────────────────────────────────────────
-- suggest fields use {"source":"geo_countries"} per §06; select/multiselect
-- options use the canonical [{value,label}] shape (CF3).

INSERT INTO custom_fields (id, group_id, label, key, field_type, options, placeholder, is_required, is_system, show_in_frontend, position) VALUES
	-- onboarding
	('00000000-0000-0000-0000-000000000f11', '00000000-0000-0000-0000-0000000000a1', 'Full name', 'full_name', 'text', NULL, NULL, true, true, true, 0),
	('00000000-0000-0000-0000-000000000f12', '00000000-0000-0000-0000-0000000000a1', 'Origin country', 'origin_country', 'suggest', '{"source":"geo_countries"}'::jsonb, NULL, true, true, true, 1),
	('00000000-0000-0000-0000-000000000f13', '00000000-0000-0000-0000-0000000000a1', 'Destination country', 'destination_country', 'suggest', '{"source":"geo_countries"}'::jsonb, NULL, true, true, true, 2),
	('00000000-0000-0000-0000-000000000f14', '00000000-0000-0000-0000-0000000000a1', 'Where are you in your japa journey?', 'japa_stage', 'select',
		'[{"value":"planning","label":"Planning to Relocate"},{"value":"about_to_japa","label":"About to Japa"},{"value":"looking_jobs","label":"Looking for Jobs Abroad"},{"value":"looking_study","label":"Looking to Study Abroad"},{"value":"newly_japa","label":"Newly Japa (< 1 year)"},{"value":"settled","label":"Settled Abroad"},{"value":"returning","label":"Considering Returning Home"},{"value":"remote_worker","label":"Remote Worker"}]'::jsonb,
		NULL, true, true, true, 3),
	('00000000-0000-0000-0000-000000000f15', '00000000-0000-0000-0000-0000000000a1', 'Primary goal', 'primary_goal', 'select',
		'[{"value":"work","label":"Work abroad"},{"value":"study","label":"Study abroad"},{"value":"family","label":"Join family"},{"value":"business","label":"Start a business"},{"value":"other","label":"Something else"}]'::jsonb,
		NULL, false, true, true, 4),
	('00000000-0000-0000-0000-000000000f16', '00000000-0000-0000-0000-0000000000a1', 'How did you hear about us?', 'how_heard', 'select',
		'[{"value":"friend_or_family","label":"Friend or family"},{"value":"social_media","label":"Social media"},{"value":"search","label":"Search engine"},{"value":"community","label":"Community member"},{"value":"other","label":"Other"}]'::jsonb,
		NULL, false, true, true, 5),
	('00000000-0000-0000-0000-000000000f17', '00000000-0000-0000-0000-0000000000a1', 'Anything else we should know?', 'anything_else', 'textarea', NULL, NULL, false, true, true, 6),
	-- event-rsvp
	('00000000-0000-0000-0000-000000000f21', '00000000-0000-0000-0000-0000000000a2', 'Event or meetup', 'event_name', 'text', NULL, 'e.g. Lagos Japa Meetup — March', true, true, true, 0),
	('00000000-0000-0000-0000-000000000f22', '00000000-0000-0000-0000-0000000000a2', 'Will you attend?', 'attendance', 'select',
		'[{"value":"attending","label":"Attending"},{"value":"maybe","label":"Maybe"},{"value":"cant_make_it","label":"Can''t make it"}]'::jsonb,
		NULL, true, true, true, 1),
	('00000000-0000-0000-0000-000000000f23', '00000000-0000-0000-0000-0000000000a2', 'Guests you are bringing', 'guests', 'number', NULL, NULL, false, true, true, 2),
	('00000000-0000-0000-0000-000000000f24', '00000000-0000-0000-0000-0000000000a2', 'Notes for the host', 'notes', 'textarea', NULL, 'Dietary needs, accessibility, questions...', false, true, true, 3),
	-- partner-enquiry
	('00000000-0000-0000-0000-000000000f31', '00000000-0000-0000-0000-0000000000a3', 'Organisation name', 'organization_name', 'text', NULL, NULL, true, true, true, 0),
	('00000000-0000-0000-0000-000000000f32', '00000000-0000-0000-0000-0000000000a3', 'Organisation type', 'organization_type', 'select',
		'[{"value":"university","label":"University or school"},{"value":"employer","label":"Employer"},{"value":"community_org","label":"Community organisation"},{"value":"media","label":"Media or press"},{"value":"government_agency","label":"Government agency"},{"value":"other","label":"Other"}]'::jsonb,
		NULL, true, true, true, 1),
	('00000000-0000-0000-0000-000000000f33', '00000000-0000-0000-0000-0000000000a3', 'Contact name', 'contact_name', 'text', NULL, NULL, true, true, true, 2),
	('00000000-0000-0000-0000-000000000f34', '00000000-0000-0000-0000-0000000000a3', 'Work email', 'work_email', 'email', NULL, NULL, true, true, true, 3),
	('00000000-0000-0000-0000-000000000f35', '00000000-0000-0000-0000-0000000000a3', 'Website', 'website', 'url', NULL, 'https://', false, true, true, 4),
	('00000000-0000-0000-0000-000000000f36', '00000000-0000-0000-0000-0000000000a3', 'How can we work together?', 'message', 'textarea', NULL, NULL, true, true, true, 5),
	-- mentorship-intake
	('00000000-0000-0000-0000-000000000f41', '00000000-0000-0000-0000-0000000000a4', 'I want to...', 'role', 'select',
		'[{"value":"find_mentor","label":"Find a mentor"},{"value":"become_mentor","label":"Become a mentor"}]'::jsonb,
		NULL, true, true, true, 0),
	('00000000-0000-0000-0000-000000000f42', '00000000-0000-0000-0000-0000000000a4', 'Profession or field', 'profession', 'text', NULL, NULL, false, true, true, 1),
	('00000000-0000-0000-0000-000000000f43', '00000000-0000-0000-0000-0000000000a4', 'Current stage', 'current_stage', 'select',
		'[{"value":"planning","label":"Planning the move"},{"value":"applying","label":"Applying / in process"},{"value":"recently_arrived","label":"Recently arrived"},{"value":"settled","label":"Settled abroad"},{"value":"returning_home","label":"Returning home"}]'::jsonb,
		NULL, false, true, true, 2),
	('00000000-0000-0000-0000-000000000f44', '00000000-0000-0000-0000-0000000000a4', 'Areas you need help with', 'areas', 'multiselect',
		'[{"value":"visas_immigration","label":"Visas & immigration"},{"value":"jobs_careers","label":"Jobs & careers"},{"value":"housing","label":"Housing"},{"value":"education","label":"Education"},{"value":"finances","label":"Finances & banking"},{"value":"community_life","label":"Community & social life"},{"value":"other","label":"Other"}]'::jsonb,
		NULL, false, true, true, 3),
	('00000000-0000-0000-0000-000000000f45', '00000000-0000-0000-0000-0000000000a4', 'What would you like to get out of mentorship?', 'goals', 'textarea', NULL, NULL, true, true, true, 4),
	-- space-application
	('00000000-0000-0000-0000-000000000f51', '00000000-0000-0000-0000-0000000000a5', 'Which space or community?', 'space_name', 'text', NULL, NULL, true, true, true, 0),
	('00000000-0000-0000-0000-000000000f52', '00000000-0000-0000-0000-0000000000a5', 'Why do you want to join?', 'why_join', 'textarea', NULL, NULL, true, true, true, 1),
	('00000000-0000-0000-0000-000000000f53', '00000000-0000-0000-0000-0000000000a5', 'Relevant background or experience', 'relevant_experience', 'textarea', NULL, NULL, false, true, true, 2),
	-- community-checkin
	('00000000-0000-0000-0000-000000000f61', '00000000-0000-0000-0000-0000000000a6', 'Where are you in your japa journey?', 'japa_stage', 'select',
		'[{"value":"planning","label":"Planning to Relocate"},{"value":"about_to_japa","label":"About to Japa"},{"value":"looking_jobs","label":"Looking for Jobs Abroad"},{"value":"looking_study","label":"Looking to Study Abroad"},{"value":"newly_japa","label":"Newly Japa (< 1 year)"},{"value":"settled","label":"Settled Abroad"},{"value":"returning","label":"Considering Returning Home"},{"value":"remote_worker","label":"Remote Worker"}]'::jsonb,
		NULL, true, true, true, 0),
	('00000000-0000-0000-0000-000000000f62', '00000000-0000-0000-0000-0000000000a6', 'How are things going?', 'how_are_things', 'select',
		'[{"value":"thriving","label":"Thriving"},{"value":"settling_in","label":"Settling in"},{"value":"getting_by","label":"Getting by"},{"value":"struggling","label":"Struggling"}]'::jsonb,
		NULL, true, true, true, 1),
	('00000000-0000-0000-0000-000000000f63', '00000000-0000-0000-0000-0000000000a6', 'Biggest challenge right now', 'biggest_challenge', 'multiselect',
		'[{"value":"paperwork","label":"Paperwork & documents"},{"value":"jobs","label":"Finding work"},{"value":"housing","label":"Housing"},{"value":"money","label":"Money & banking"},{"value":"community","label":"Finding community"},{"value":"health","label":"Health & wellbeing"},{"value":"other","label":"Other"}]'::jsonb,
		NULL, false, true, true, 2),
	('00000000-0000-0000-0000-000000000f64', '00000000-0000-0000-0000-0000000000a6', 'What would help you most right now?', 'what_would_help', 'textarea', NULL, NULL, false, true, true, 3)
ON CONFLICT (group_id, key) DO NOTHING;

-- ── 4. Field permissions: every group can view (required for the public
-- embed — GetFormEmbed narrows to guest can_view + show_in_frontend);
-- non-guest groups can edit. Matches the 000859/000885 convention.

INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT f.id, ug.id, true, ug.slug <> 'guest'
FROM custom_fields f
CROSS JOIN user_groups ug
WHERE f.group_id IN (
	'00000000-0000-0000-0000-0000000000a1',
	'00000000-0000-0000-0000-0000000000a2',
	'00000000-0000-0000-0000-0000000000a3',
	'00000000-0000-0000-0000-0000000000a4',
	'00000000-0000-0000-0000-0000000000a5',
	'00000000-0000-0000-0000-0000000000a6'
)
ON CONFLICT (field_id, user_group_id) DO NOTHING;

-- ── 5. Wiring ────────────────────────────────────────────────────────────
-- /contact "Partnerships & press" card self-linked to /contact; point it at
-- the new partner-enquiry form.

UPDATE pages
SET blocks = jsonb_set(blocks, '{1,props,cards,3,cta}',
		'{"label": "Partner enquiry", "href": "/forms/partner-enquiry", "variant": "link"}'::jsonb, false),
	updated_at = now()
WHERE slug = 'contact'
  AND blocks->1->'props'->'cards'->3->>'title' = 'Partnerships & press';

-- /feedback had no navigation entry — add it to the footer before the auth
-- links (same convention as 000885).

UPDATE menu_items SET sort_order = 14 WHERE menu_id = (SELECT id FROM menus WHERE slug = 'main_footer') AND url = '/login';
UPDATE menu_items SET sort_order = 13 WHERE menu_id = (SELECT id FROM menus WHERE slug = 'main_footer') AND url = '/signup';

INSERT INTO menu_items (menu_id, label, url, sort_order, visibility)
SELECT m.id, 'Feedback', '/feedback', 12, 'all'
FROM menus m
WHERE m.slug = 'main_footer'
  AND NOT EXISTS (
	SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/feedback'
  );

COMMIT;
