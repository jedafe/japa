-- Scrub a leaked super-admin credential that slipped into baseline seed
-- content (doc_pages row 'new-title'). Public docs must never carry secrets.
UPDATE public.doc_pages
SET content = '"## Getting around your account\n\nYour account settings live under your profile. Update your display name, avatar, and notification preferences there.\n\n- Profile → display name and avatar\n- Notifications → email and push preferences\n- Security → password and two-factor setup"'
WHERE slug = 'new-title'
  AND content::text ILIKE '%super_admin credential%';
