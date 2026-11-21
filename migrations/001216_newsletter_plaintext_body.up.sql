-- The seeded `newsletter` template's plain_text_body is '{{.Content}}' —
-- the campaign's rendered HTML dropped verbatim into the plaintext leg.
-- Replace it with a text-only version that names the campaign and points
-- at the site, so a plaintext render never emits literal HTML markup.
UPDATE email_templates
SET plain_text_body = '{{.Subject}}

You are receiving this campaign from {{.SiteName}}.

Read it online: {{.SiteURL}}
Manage your subscription: {{.SiteURL}}/unsubscribe',
    updated_at = now()
WHERE slug = 'newsletter';
