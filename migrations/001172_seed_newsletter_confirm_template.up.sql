-- 001172 — seed the newsletter_confirm email_templates slug for the double
-- opt-in confirmation email (§62 Compliance hardening, migration 001170).
--
-- The confirm flow previously enqueued a raw HTML body, which bypassed the
-- ADR-055 DB-template chain — no admin branding edits via Control → Email
-- Templates. The producer now sends template_slug + template_data; this row
-- is the brand-styled template. The inline body remains the caller-body
-- fallback when the row is missing/inactive. Idempotent on slug.

INSERT INTO email_templates (type, name, slug, subject, html_body, plain_text_body, variables, is_active)
SELECT 'newsletter_confirm', 'Newsletter Subscription Confirmation', 'newsletter_confirm',
 'Confirm your {{.SiteName}} newsletter subscription',
 '<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Confirm your subscription</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, ''Segoe UI'', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            <p style="margin: 0 0 16px 0;">Please confirm your newsletter subscription by clicking the button below.</p>
            {{if .ActionURL}}<p style="margin: 0 0 24px 0; text-align: center;"><a href="{{.ActionURL}}" style="display: inline-block; background-color: #0f766e; color: #ffffff; padding: 12px 28px; border-radius: 6px; text-decoration: none; font-weight: 600;">Confirm subscription</a></p>{{end}}
            {{if .Expiry}}<p style="margin: 0 0 16px 0; color: #666666; font-size: 13px;">This link expires in {{.Expiry}}.</p>{{end}}
            <p style="margin: 0 0 16px 0; color: #666666; font-size: 13px;">If you did not sign up, you can safely ignore this email — you will not receive any further messages.</p>
          </td>
        </tr>
        <tr>
          <td style="padding: 24px 32px; text-align: center; font-size: 12px; color: #999999; border-top: 1px solid #eeeeee; background-color: #fafafa;">
            <p style="margin: 0 0 8px 0;">&copy; {{.SiteName}} &mdash; Your journey, connected.</p>
            <p style="margin: 0;">Need help? <a href="{{.SupportURL}}" style="color: #0f766e; text-decoration: none;">Contact support</a></p>
          </td>
        </tr>
      </table>
    </td>
  </tr>
</table>
</body>
</html>',
 'Please confirm your {{.SiteName}} newsletter subscription: {{.ActionURL}} — this link expires in {{.Expiry}}. If you did not sign up, ignore this email.',
 '[{"name": "SiteName", "required": true, "description": "Brand name"}, {"name": "ActionURL", "required": true, "description": "Subscription confirmation link"}, {"name": "SiteURL", "required": false, "description": "Site home URL"}, {"name": "Expiry", "required": false, "description": "Confirmation link expiry"}, {"name": "SupportURL", "required": false, "description": "Support contact URL"}]',
 true
WHERE NOT EXISTS (SELECT 1 FROM email_templates WHERE slug = 'newsletter_confirm');
