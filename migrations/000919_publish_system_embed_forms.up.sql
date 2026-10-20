-- Publish the system forms that public pages consume via
-- GET /api/v1/forms/{slug}/embed. The embed handler gates on
-- status='published' (CF13: only published forms expose a public schema),
-- but these is_system rows were seeded 'active', so their embeds 404'd —
-- on /jobs/[slug] this silently dropped CV/cover-letter attachments
-- because applicationFormId could never resolve.
--
-- Ref: sections/63-jobs-board.md (on-platform apply via §06 forms)
UPDATE forms
SET status = 'published'
WHERE slug IN ('job_application_form', 'claim-ownership', 'visa-guide-request')
  AND is_system = true
  AND status <> 'published';
