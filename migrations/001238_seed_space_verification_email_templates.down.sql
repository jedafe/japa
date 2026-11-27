-- 001238 down — remove the seeded space verification email templates.
DELETE FROM email_templates
WHERE slug IN (
    'space_verification_granted',
    'space_verification_revoked'
);
