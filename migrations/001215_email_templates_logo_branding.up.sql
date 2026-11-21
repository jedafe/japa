-- 001215 — brand every seeded email_templates row with the platform logo and
-- seed the `notification` + `community_digest` templates.
--
-- Part 1: every seeded template header renders {{.SiteName}} as plain text.
-- The worker now injects a resolved logo_url (appearance.logo_url_dark →
-- appearance.logo_url → {site.base_url}/logos/logo-dark.png) into every
-- template render; this UPDATE adds a guarded {{.LogoURL}} <img> above the
-- existing SiteName h1 so all stock templates show the japa.to brand mark.
-- The h1 stays — clients that block remote images still show the name.
-- Admin-customized bodies that no longer contain the stock header string
-- are untouched (replace() no-ops).

UPDATE email_templates
SET html_body = replace(
    html_body,
    $$<h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>$$,
    $${{if .LogoURL}}<img src="{{.LogoURL}}" alt="{{.SiteName}}" height="44" style="display: inline-block; max-height: 44px; margin: 0 auto 8px; border: 0; outline: none; text-decoration: none;" /><br />{{end}}<h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>$$
);

UPDATE email_templates
SET html_body = replace(
    html_body,
    $$<h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">Welcome to {{.SiteName}}</h1>$$,
    $${{if .LogoURL}}<img src="{{.LogoURL}}" alt="{{.SiteName}}" height="44" style="display: inline-block; max-height: 44px; margin: 0 auto 8px; border: 0; outline: none; text-decoration: none;" /><br />{{end}}<h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">Welcome to {{.SiteName}}</h1>$$
);

-- Part 2: `notification` — §35 email-channel notifications previously sent a
-- raw subject+body with no branding or CTA. The producer now passes
-- template_slug=notification + template_data {title, body, action_url}; this
-- row is the branded shell. Idempotent on slug.

INSERT INTO email_templates (type, name, slug, subject, html_body, plain_text_body, variables, is_active)
SELECT 'notification', 'Notification', '{{.Title}}',
 '<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>{{.Title}}</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, ''Segoe UI'', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            {{if .LogoURL}}<img src="{{.LogoURL}}" alt="{{.SiteName}}" height="44" style="display: inline-block; max-height: 44px; margin: 0 auto 8px; border: 0; outline: none; text-decoration: none;" /><br />{{end}}
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .Title}}<h2 style="margin: 0 0 16px 0; font-size: 18px; color: #111111;">{{.Title}}</h2>{{end}}
            {{if .Body}}<p style="margin: 0 0 16px 0; white-space: pre-wrap;">{{.Body}}</p>{{end}}
            {{if .ActionURL}}<table role="presentation" cellpadding="0" cellspacing="0" style="margin: 24px 0;">
              <tr>
                <td style="background-color: #0f766e; border-radius: 6px;">
                  <a href="{{.ActionURL}}" target="_blank" style="display: inline-block; padding: 12px 24px; color: #ffffff; font-size: 15px; font-weight: 600; text-decoration: none; border-radius: 6px;">View</a>
                </td>
              </tr>
            </table>{{end}}
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
 '{{.Title}}\n\n{{.Body}}{{if .ActionURL}}\n\nView: {{.ActionURL}}{{end}}',
 '[{"name": "SiteName", "required": true, "description": "Brand name"}, {"name": "Title", "required": true, "description": "Notification title/subject"}, {"name": "Body", "required": true, "description": "Notification body text"}, {"name": "ActionURL", "required": false, "description": "Deep link for the CTA button"}, {"name": "LogoURL", "required": false, "description": "Brand logo URL"}, {"name": "SupportURL", "required": false, "description": "Support contact URL"}]',
 true
WHERE NOT EXISTS (SELECT 1 FROM email_templates WHERE slug = 'notification');

-- Part 3: `community_digest` — the §84 digest:send_email body renders the
-- edition content through this row ({{.DigestContent}}) so the branded shell
-- wraps it; the worker-side inline shell remains the fallback.
-- Idempotent on slug.

INSERT INTO email_templates (type, name, slug, subject, html_body, plain_text_body, variables, is_active)
SELECT 'community_digest', 'Community Digest', 'Weekly Community Digest — {{.SiteName}}',
 '<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Weekly Community Digest from {{.SiteName}}</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, ''Segoe UI'', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            {{if .LogoURL}}<img src="{{.LogoURL}}" alt="{{.SiteName}}" height="44" style="display: inline-block; max-height: 44px; margin: 0 auto 8px; border: 0; outline: none; text-decoration: none;" /><br />{{end}}
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            <h2 style="margin: 0 0 16px 0; font-size: 18px;">Weekly Community Digest</h2>
            {{.DigestContent}}
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
 'Weekly Community Digest — {{.SiteName}}\n\nView online: {{.SiteURL}}',
 '[{"name": "SiteName", "required": true, "description": "Brand name"}, {"name": "DigestContent", "required": true, "description": "Rendered digest sections"}, {"name": "SiteURL", "required": false, "description": "Site home URL"}, {"name": "LogoURL", "required": false, "description": "Brand logo URL"}, {"name": "SupportURL", "required": false, "description": "Support contact URL"}]',
 true
WHERE NOT EXISTS (SELECT 1 FROM email_templates WHERE slug = 'community_digest');
