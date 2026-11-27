-- 001238 — seed the two space-scoped verification email_templates slugs so
-- the §07 space grant/revoke notifications route through the ADR-055
-- DB-template chain (admin-editable via Control → Email Templates) instead
-- of borrowing the user-profile copy ("Your profile has been verified"):
--   space_verification_granted — space badge granted (internal/spaces)
--   space_verification_revoked — space badge revoked (internal/spaces)
--
-- Mirrors the org_verification_granted precedent from 001124. The
-- producer's inline subject/body remain the caller-body fallback when the
-- template row is missing/inactive. Idempotent on slug.

INSERT INTO email_templates (type, name, slug, subject, html_body, plain_text_body, variables, is_active)
SELECT 'space_verification_granted', 'Space Verification Granted', 'space_verification_granted',
 '{{.SpaceName}} has been verified on {{.SiteName}}',
 '<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Space Verified</title>
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
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">Your space <strong>{{.SpaceName}}</strong> has been verified — the badge is now shown publicly on the space.</p>
            {{if .ActionURL}}<p style="margin: 0 0 24px 0; text-align: center;"><a href="{{.ActionURL}}" style="display: inline-block; background-color: #0f766e; color: #ffffff; padding: 12px 28px; border-radius: 6px; text-decoration: none; font-weight: 600;">View Space</a></p>{{end}}
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
 'Your space {{.SpaceName}} has been verified on {{.SiteName}} — the badge is now shown publicly.{{if .ActionURL}} View it: {{.ActionURL}}{{end}}',
 '[{"name": "SiteName", "required": true, "description": "Brand name"}, {"name": "UserName", "required": false, "description": "Recipient display name"}, {"name": "SpaceName", "required": true, "description": "Space name"}, {"name": "ActionURL", "required": false, "description": "Space page link"}, {"name": "SupportURL", "required": false, "description": "Support contact URL"}]',
 true
WHERE NOT EXISTS (SELECT 1 FROM email_templates WHERE slug = 'space_verification_granted');

INSERT INTO email_templates (type, name, slug, subject, html_body, plain_text_body, variables, is_active)
SELECT 'space_verification_revoked', 'Space Verification Revoked', 'space_verification_revoked',
 'Verification removed from {{.SpaceName}} on {{.SiteName}}',
 '<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Space Verification Revoked</title>
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
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">The verification badge on your space <strong>{{.SpaceName}}</strong> has been removed by our review team.</p>
            <p style="margin: 0 0 16px 0; color: #666666; font-size: 13px;">If you believe this was done in error, please contact our support team.</p>
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
 'The verification badge on your space {{.SpaceName}} has been removed. If you believe this was done in error, contact support{{if .SupportURL}}: {{.SupportURL}}{{end}}.',
 '[{"name": "SiteName", "required": true, "description": "Brand name"}, {"name": "UserName", "required": false, "description": "Recipient display name"}, {"name": "SpaceName", "required": true, "description": "Space name"}, {"name": "SupportURL", "required": false, "description": "Support contact URL"}]',
 true
WHERE NOT EXISTS (SELECT 1 FROM email_templates WHERE slug = 'space_verification_revoked');
