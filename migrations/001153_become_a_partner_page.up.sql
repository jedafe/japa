-- 001153_become_a_partner_page.up.sql
-- (a) Adds "Travel agency" to the partner-enquiry Organisation type options
--     (custom_fields ...f32, seeded by 001152) — travel agencies are a real
--     partner channel for a relocation platform and were missing.
-- (b) Seeds a "Become a Partner" Page Builder page (slug 'partners') whose
--     form_embed block posts to the partner-enquiry form's submissions
--     endpoint — the /contact card convention from 000885. The page surfaces
--     at /partners via the root /[slug] published-page route (§27).
-- (c) Wiring: /contact "Partnerships & press" card now points at /partners
--     (the curated landing page) instead of the bare /forms/partner-enquiry
--     renderer, and a "Become a Partner" footer link joins the footer before
--     the auth links (same convention as the Feedback insert in 001152).
--
-- Ref: .jeos/sections/27-page-builder.md, sections/06 (form_embed / CF3)

BEGIN;

-- ── 1. Organisation type options += Travel agency (before Other) ─────────

UPDATE custom_fields
SET options =
  '[{"value":"university","label":"University or school"},{"value":"employer","label":"Employer"},{"value":"community_org","label":"Community organisation"},{"value":"media","label":"Media or press"},{"value":"government_agency","label":"Government agency"},{"value":"travel_agency","label":"Travel agency"},{"value":"other","label":"Other"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-000000000f32';

-- ── 2. Become a Partner page ──────────────────────────────────────────────
-- form_embed.fields mirror the partner-enquiry custom_fields; select options
-- are display labels — the submissions endpoint normalizes them to canonical
-- values (CF3 NormalizeChoiceValue). submit_endpoint is the seeded
-- partner-enquiry form id (001152).

INSERT INTO pages (id, title, slug, description, page_type, status, blocks, meta, published_at)
VALUES (gen_random_uuid(), 'Become a Partner', 'partners',
	'Partnerships with universities, employers, community organisations, media, government agencies and travel agencies — tell us how we can work together.',
	'page', 'published',
	'[{"id": "partners-hero", "type": "hero", "visible": true, "props": {"eyebrow": "Partner with Japa.to", "headline": "Become a partner.", "subhead": "Universities, employers, community organisations, media, government agencies and travel agencies — if your people are planning, making or settling into an international move, let us work together.", "layout": "split", "media": {"media_id": "", "url": "/illustrations/dotted-world-map.png", "alt": "World map of the Japa.to community network"}, "ctas": [{"label": "Send a partner enquiry", "href": "#partner-enquiry", "variant": "primary"}, {"label": "Talk to us first", "href": "/contact", "variant": "ghost"}]}},
	  {"id": "partners-who", "type": "card_grid", "visible": true, "props": {"cards": [{"icon": "GraduationCap", "title": "Universities & schools", "description": "International offices, student services and alumni networks — bring verified relocation guidance to your students."}, {"icon": "Briefcase", "title": "Employers", "description": "Companies relocating staff or hiring internationally — onboarding content and community support for incoming hires."}, {"icon": "Users", "title": "Community organisations", "description": "Diaspora groups, cultural associations and NGOs — shared spaces and reach for the people you serve."}, {"icon": "Newspaper", "title": "Media & press", "description": "Publications and creators covering migration — data, stories and expert sources."}, {"icon": "Landmark", "title": "Government agencies", "description": "Consulates, trade offices and public bodies supporting citizens abroad."}, {"icon": "Plane", "title": "Travel agencies", "description": "Agencies booking relocation travel — extend your service past the ticket into settling-in support."}], "card_variant": "feature", "layout": "grid", "columns": 3, "responsive_columns": {"tablet": 2, "mobile": 1}}},
	  {"id": "partner-enquiry", "type": "form_embed", "visible": true, "props": {"preset": "custom", "form_slug": "partner-enquiry", "title": "Send a partner enquiry", "subtitle": "Tell us who you are and how we can work together — the partnerships team replies within 2 business days.", "submit_endpoint": "/api/v1/forms/00000000-0000-0000-0000-0000000000f3/submissions", "submit_label": "Send enquiry", "success_message": "Thanks — our partnerships team will be in touch.", "error_message": "Something went wrong sending your enquiry. Please try again — if it keeps failing, reach us via /contact.", "fields": [{"name": "organization_name", "label": "Organisation name", "type": "text", "required": true}, {"name": "organization_type", "label": "Organisation type", "type": "select", "required": true, "options": ["University or school", "Employer", "Community organisation", "Media or press", "Government agency", "Travel agency", "Other"]}, {"name": "contact_name", "label": "Contact name", "type": "text", "required": true}, {"name": "work_email", "label": "Work email", "type": "email", "required": true, "placeholder": "you@organisation.com"}, {"name": "website", "label": "Website", "type": "text", "required": false, "placeholder": "https://"}, {"name": "message", "label": "How can we work together?", "type": "textarea", "required": true}]}},
	  {"id": "partners-notes", "type": "content", "visible": true, "props": {"html": "\n<h3>What happens next</h3>\n<ul>\n  <li><strong>We reply within 2 business days</strong> — a real person from the partnerships team, not an autoresponder.</li>\n  <li><strong>We scope the fit:</strong> content licensing, referral flows, co-branded onboarding for your people, or something else entirely.</li>\n  <li><strong>No commitment:</strong> an enquiry starts a conversation — nothing is signed or scheduled until both sides want it.</li>\n</ul>\n", "prose_style": "minimal"}},
	  {"id": "partners-cta", "type": "cta_banner", "visible": true, "props": {"headline": "Not a partnership question?", "subhead": "Support, feedback and general questions live on the contact page.", "ctas": [{"label": "Contact us", "href": "/contact", "variant": "primary"}], "alignment": "center"}}]'::jsonb,
	'{"title": "Become a Partner — Japa.to", "description": "Partner with Japa.to: universities, employers, community organisations, media, government agencies and travel agencies."}'::jsonb, now())
ON CONFLICT (slug) DO UPDATE SET
	title = EXCLUDED.title,
	description = EXCLUDED.description,
	page_type = EXCLUDED.page_type,
	status = EXCLUDED.status,
	blocks = EXCLUDED.blocks,
	meta = EXCLUDED.meta,
	published_at = COALESCE(pages.published_at, now()),
	updated_at = now();

-- ── 3. Wiring ────────────────────────────────────────────────────────────
-- /contact "Partnerships & press" card → the curated partners page (was the
-- bare /forms/partner-enquiry renderer since 001152).

UPDATE pages
SET blocks = jsonb_set(blocks, '{1,props,cards,3,cta}',
		'{"label": "Become a partner", "href": "/partners", "variant": "link"}'::jsonb, false),
	updated_at = now()
WHERE slug = 'contact'
  AND blocks->1->'props'->'cards'->3->>'title' = 'Partnerships & press';

-- Footer: 'Become a Partner' at 12, shifting Feedback/auth links down one
-- (same insert-convention as Feedback in 001152).

UPDATE menu_items SET sort_order = 15 WHERE menu_id = (SELECT id FROM menus WHERE slug = 'main_footer') AND url = '/login';
UPDATE menu_items SET sort_order = 14 WHERE menu_id = (SELECT id FROM menus WHERE slug = 'main_footer') AND url = '/signup';
UPDATE menu_items SET sort_order = 13 WHERE menu_id = (SELECT id FROM menus WHERE slug = 'main_footer') AND url = '/feedback';

INSERT INTO menu_items (menu_id, label, url, sort_order, visibility)
SELECT m.id, 'Become a Partner', '/partners', 12, 'all'
FROM menus m
WHERE m.slug = 'main_footer'
  AND NOT EXISTS (
	SELECT 1 FROM menu_items mi WHERE mi.menu_id = m.id AND mi.url = '/partners'
  );

COMMIT;
