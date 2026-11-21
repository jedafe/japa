-- Restore the original seeded plain_text_body ('{{.Content}}').
UPDATE email_templates
SET plain_text_body = '{{.Content}}',
    updated_at = now()
WHERE slug = 'newsletter';
