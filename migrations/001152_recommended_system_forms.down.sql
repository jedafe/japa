-- 001152_recommended_system_forms.down.sql
-- Removes the six recommended system forms (groups, fields, permissions,
-- assignments), restores the /contact "Partnerships & press" card CTA to its
-- previous /contact self-link, and removes the footer Feedback link.

BEGIN;

-- Restore the contact card CTA.

UPDATE pages
SET blocks = jsonb_set(blocks, '{1,props,cards,3,cta}',
		'{"label": "Contact us", "href": "/contact", "variant": "link"}'::jsonb, false),
	updated_at = now()
WHERE slug = 'contact'
  AND blocks->1->'props'->'cards'->3->>'title' = 'Partnerships & press';

-- Remove the Feedback footer link and restore auth-link ordering.

DELETE FROM menu_items
WHERE menu_id = (SELECT id FROM menus WHERE slug = 'main_footer') AND url = '/feedback';
UPDATE menu_items SET sort_order = 12 WHERE menu_id = (SELECT id FROM menus WHERE slug = 'main_footer') AND url = '/signup';
UPDATE menu_items SET sort_order = 13 WHERE menu_id = (SELECT id FROM menus WHERE slug = 'main_footer') AND url = '/login';

-- Form submissions are kept (user data). Remove the schema rows.

DELETE FROM custom_field_permissions WHERE field_id IN (
	SELECT id FROM custom_fields WHERE group_id IN (
		'00000000-0000-0000-0000-0000000000a1',
		'00000000-0000-0000-0000-0000000000a2',
		'00000000-0000-0000-0000-0000000000a3',
		'00000000-0000-0000-0000-0000000000a4',
		'00000000-0000-0000-0000-0000000000a5',
		'00000000-0000-0000-0000-0000000000a6'
	)
);

DELETE FROM custom_field_group_assignments WHERE group_id IN (
	'00000000-0000-0000-0000-0000000000a1',
	'00000000-0000-0000-0000-0000000000a2',
	'00000000-0000-0000-0000-0000000000a3',
	'00000000-0000-0000-0000-0000000000a4',
	'00000000-0000-0000-0000-0000000000a5',
	'00000000-0000-0000-0000-0000000000a6'
);

DELETE FROM custom_fields WHERE group_id IN (
	'00000000-0000-0000-0000-0000000000a1',
	'00000000-0000-0000-0000-0000000000a2',
	'00000000-0000-0000-0000-0000000000a3',
	'00000000-0000-0000-0000-0000000000a4',
	'00000000-0000-0000-0000-0000000000a5',
	'00000000-0000-0000-0000-0000000000a6'
);

DELETE FROM custom_field_groups WHERE id IN (
	'00000000-0000-0000-0000-0000000000a1',
	'00000000-0000-0000-0000-0000000000a2',
	'00000000-0000-0000-0000-0000000000a3',
	'00000000-0000-0000-0000-0000000000a4',
	'00000000-0000-0000-0000-0000000000a5',
	'00000000-0000-0000-0000-0000000000a6'
);

DELETE FROM forms WHERE slug IN (
	'onboarding', 'event-rsvp', 'partner-enquiry',
	'mentorship-intake', 'space-application', 'community-checkin'
);

COMMIT;
