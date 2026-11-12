-- 001124 — seed five additional email_templates slugs for custom flows that
-- previously bypassed the DB-template system (ADR-055 rendering chain):
--   organization_invitation   — org invite email (internal/organizations)
--   verification_granted      — user profile badge granted (internal/users)
--   verification_revoked      — user profile badge revoked (internal/users)
--   org_verification_granted  — organization badge granted (internal/organizations)
--   verification_review       — verification request outcome (internal/verification)
--
-- Each producer now enqueues template_slug + template_data so admin edits
-- via Control → Email Templates take effect; the inline body remains the
-- ADR-055 caller-body fallback. Idempotent on slug.

INSERT INTO email_templates (type, name, slug, subject, html_body, plain_text_body, variables, is_active)
SELECT 'organization_invitation', 'Organization Invitation', 'organization_invitation',
 'You have been invited to join {{.OrgName}} on {{.SiteName}}',
 '<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Organization Invitation</title>
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
            <p style="margin: 0 0 16px 0;">You have been invited to join <strong>{{.OrgName}}</strong>{{if .Role}} as a <strong>{{.Role}}</strong>{{end}} on {{.SiteName}}.</p>
            {{if .ActionURL}}<p style="margin: 0 0 24px 0; text-align: center;"><a href="{{.ActionURL}}" style="display: inline-block; background-color: #0f766e; color: #ffffff; padding: 12px 28px; border-radius: 6px; text-decoration: none; font-weight: 600;">Accept Invitation</a></p>{{end}}
            <p style="margin: 0 0 16px 0; color: #666666; font-size: 13px;">If you did not expect this invitation, you can safely ignore this email.</p>
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
 'You have been invited to join {{.OrgName}}{{if .Role}} as a {{.Role}}{{end}} on {{.SiteName}}. Accept: {{.ActionURL}} — if you did not expect this, ignore this email.',
 '[{"name": "SiteName", "required": true, "description": "Brand name"}, {"name": "UserName", "required": false, "description": "Recipient display name"}, {"name": "OrgName", "required": true, "description": "Organization name"}, {"name": "Role", "required": false, "description": "Invited role"}, {"name": "ActionURL", "required": true, "description": "Invitation accept link"}, {"name": "SupportURL", "required": false, "description": "Support contact URL"}]',
 true
WHERE NOT EXISTS (SELECT 1 FROM email_templates WHERE slug = 'organization_invitation');

INSERT INTO email_templates (type, name, slug, subject, html_body, plain_text_body, variables, is_active)
SELECT 'verification_granted', 'Verification Granted', 'verification_granted',
 'Your {{.SiteName}} profile has been verified',
 '<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Verification Granted</title>
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
            <p style="margin: 0 0 16px 0;">Your {{.SiteName}} profile has been verified — the badge is now shown publicly on your profile.</p>
            {{if .ActionURL}}<p style="margin: 0 0 24px 0; text-align: center;"><a href="{{.ActionURL}}" style="display: inline-block; background-color: #0f766e; color: #ffffff; padding: 12px 28px; border-radius: 6px; text-decoration: none; font-weight: 600;">View Profile</a></p>{{end}}
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
 'Your {{.SiteName}} profile has been verified — the badge is now shown publicly on your profile.{{if .ActionURL}} View it: {{.ActionURL}}{{end}}',
 '[{"name": "SiteName", "required": true, "description": "Brand name"}, {"name": "UserName", "required": false, "description": "Recipient display name"}, {"name": "ActionURL", "required": false, "description": "Profile link"}, {"name": "SupportURL", "required": false, "description": "Support contact URL"}]',
 true
WHERE NOT EXISTS (SELECT 1 FROM email_templates WHERE slug = 'verification_granted');

INSERT INTO email_templates (type, name, slug, subject, html_body, plain_text_body, variables, is_active)
SELECT 'verification_revoked', 'Verification Revoked', 'verification_revoked',
 'Your {{.SiteName}} verification was removed',
 '<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Verification Revoked</title>
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
            <p style="margin: 0 0 16px 0;">Your {{.SiteName}} profile verification badge has been removed.</p>
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
 'Your {{.SiteName}} profile verification badge has been removed. If you believe this was done in error, contact support{{if .SupportURL}}: {{.SupportURL}}{{end}}.',
 '[{"name": "SiteName", "required": true, "description": "Brand name"}, {"name": "UserName", "required": false, "description": "Recipient display name"}, {"name": "SupportURL", "required": false, "description": "Support contact URL"}]',
 true
WHERE NOT EXISTS (SELECT 1 FROM email_templates WHERE slug = 'verification_revoked');

INSERT INTO email_templates (type, name, slug, subject, html_body, plain_text_body, variables, is_active)
SELECT 'org_verification_granted', 'Organization Verification Granted', 'org_verification_granted',
 '{{.OrgName}} has been verified on {{.SiteName}}',
 '<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Organization Verified</title>
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
            <p style="margin: 0 0 16px 0;">Your organization <strong>{{.OrgName}}</strong> has been verified — the badge is now shown publicly on its profile.</p>
            {{if .ActionURL}}<p style="margin: 0 0 24px 0; text-align: center;"><a href="{{.ActionURL}}" style="display: inline-block; background-color: #0f766e; color: #ffffff; padding: 12px 28px; border-radius: 6px; text-decoration: none; font-weight: 600;">View Organization</a></p>{{end}}
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
 'Your organization {{.OrgName}} has been verified on {{.SiteName}} — the badge is now shown publicly.{{if .ActionURL}} View it: {{.ActionURL}}{{end}}',
 '[{"name": "SiteName", "required": true, "description": "Brand name"}, {"name": "UserName", "required": false, "description": "Recipient display name"}, {"name": "OrgName", "required": true, "description": "Organization name"}, {"name": "ActionURL", "required": false, "description": "Organization profile link"}, {"name": "SupportURL", "required": false, "description": "Support contact URL"}]',
 true
WHERE NOT EXISTS (SELECT 1 FROM email_templates WHERE slug = 'org_verification_granted');

INSERT INTO email_templates (type, name, slug, subject, html_body, plain_text_body, variables, is_active)
SELECT 'verification_review', 'Verification Request Reviewed', 'verification_review',
 'Your {{.SiteName}} verification request was {{.Status}}',
 '<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Verification Request Reviewed</title>
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
            <p style="margin: 0 0 16px 0;">Your verification request has been <strong>{{.Status}}</strong> by our review team.</p>
            {{if .Note}}<p style="margin: 0 0 16px 0; padding: 12px 16px; background-color: #f4f4f7; border-left: 3px solid #0f766e; color: #555555;">Note from the reviewer: {{.Note}}</p>{{end}}
            {{if .ActionURL}}<p style="margin: 0 0 24px 0; text-align: center;"><a href="{{.ActionURL}}" style="display: inline-block; background-color: #0f766e; color: #ffffff; padding: 12px 28px; border-radius: 6px; text-decoration: none; font-weight: 600;">Track Request</a></p>{{end}}
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
 'Your verification request has been {{.Status}} by our review team.{{if .Note}} Note from the reviewer: {{.Note}}{{end}}{{if .ActionURL}} Track it: {{.ActionURL}}{{end}}',
 '[{"name": "SiteName", "required": true, "description": "Brand name"}, {"name": "UserName", "required": false, "description": "Recipient display name"}, {"name": "Status", "required": true, "description": "Review outcome (approved/rejected)"}, {"name": "Note", "required": false, "description": "Reviewer note"}, {"name": "ActionURL", "required": false, "description": "Request tracking link"}, {"name": "SupportURL", "required": false, "description": "Support contact URL"}]',
 true
WHERE NOT EXISTS (SELECT 1 FROM email_templates WHERE slug = 'verification_review');
