-- Restore the pre-publish status for the system embed forms.
UPDATE forms
SET status = 'active'
WHERE slug IN ('job_application_form', 'claim-ownership', 'visa-guide-request')
  AND is_system = true
  AND status = 'published';
