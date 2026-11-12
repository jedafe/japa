-- 001124 down — remove the five seeded custom-flow email templates.
DELETE FROM email_templates
WHERE slug IN (
    'organization_invitation',
    'verification_granted',
    'verification_revoked',
    'org_verification_granted',
    'verification_review'
);
