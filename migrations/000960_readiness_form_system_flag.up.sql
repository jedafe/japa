-- The Japa Readiness Assessment is a platform-managed form (form_type=
-- 'assessment', consumed by the bespoke scoring engine at /tools/readiness)
-- but was seeded is_system=false, so it appears as a plain custom form in
-- admin and — worse — stayed unprotected by the is_system delete guards on
-- forms/field groups. Flag it as system to match the other managed forms.
UPDATE forms
SET is_system = true
WHERE slug = 'japa-readiness-assessment';
