-- 001172 down — drop the seeded newsletter_confirm template. The confirm
-- producer keeps its inline-body fallback, so removing the row only loses
-- admin-editability, not the email itself.
DELETE FROM email_templates WHERE slug = 'newsletter_confirm';
