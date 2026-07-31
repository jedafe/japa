-- 000212_populate_email_template_html_bodies.up.sql
-- Populates the html_body field for all 22 email templates.
-- The first 3 (password_reset, email_verification, welcome) use the exact
-- HTML from internal/email/html.go. The remaining 19 follow the same
-- design pattern with template-specific content and variables.
-- Design pattern: inline CSS, teal (#0f766e) header, 600px max-width,
-- responsive table layout, footer with copyright and support link.

BEGIN;

-- ============================================================
-- Auth: Password Reset (exact copy from internal/email/html.go)
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Reset Your Password</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">We received a request to reset the password for your account. Click the button below to choose a new password. The link is valid for <strong>{{.Expiry}}</strong>.</p>
            <table role="presentation" cellpadding="0" cellspacing="0" style="margin: 24px 0;">
              <tr>
                <td style="background-color: #0f766e; border-radius: 6px;">
                  <a href="{{.ActionURL}}" target="_blank" style="display: inline-block; padding: 12px 24px; color: #ffffff; font-size: 15px; font-weight: 600; text-decoration: none; border-radius: 6px;">Reset Password</a>
                </td>
              </tr>
            </table>
            <p style="margin: 0 0 8px 0; color: #666666; font-size: 13px;">If the button doesn't work, copy and paste this link into your browser:</p>
            <p style="margin: 0 0 16px 0; word-break: break-all;">
              <a href="{{.ActionURL}}" style="color: #0f766e; font-size: 13px; text-decoration: underline;">{{.ActionURL}}</a>
            </p>
            <p style="margin: 0; color: #666666; font-size: 13px;">If you didn't request a password reset, you can safely ignore this email. Your password will remain unchanged.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'password_reset'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Auth: Email Verification (exact copy from internal/email/html.go)
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Verify Your Email</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">Welcome to {{.SiteName}}! To complete your registration and unlock every feature, please verify your email address by clicking the button below. The link is valid for <strong>{{.Expiry}}</strong>.</p>
            <table role="presentation" cellpadding="0" cellspacing="0" style="margin: 24px 0;">
              <tr>
                <td style="background-color: #0f766e; border-radius: 6px;">
                  <a href="{{.ActionURL}}" target="_blank" style="display: inline-block; padding: 12px 24px; color: #ffffff; font-size: 15px; font-weight: 600; text-decoration: none; border-radius: 6px;">Verify Email Address</a>
                </td>
              </tr>
            </table>
            <p style="margin: 0 0 8px 0; color: #666666; font-size: 13px;">If the button doesn't work, copy and paste this link into your browser:</p>
            <p style="margin: 0 0 16px 0; word-break: break-all;">
              <a href="{{.ActionURL}}" style="color: #0f766e; font-size: 13px; text-decoration: underline;">{{.ActionURL}}</a>
            </p>
            <p style="margin: 0; color: #666666; font-size: 13px;">If you didn't create an account with us, you can safely ignore this email.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'email_verification'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Auth: Welcome (exact copy from internal/email/html.go)
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Welcome to {{.SiteName}}</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">Welcome to {{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">Your account is ready. {{.SiteName}} is your home for connecting with the Nigerian diaspora, discovering opportunities abroad, and sharing your journey with a community that understands.</p>
            <table role="presentation" cellpadding="0" cellspacing="0" style="margin: 24px 0;">
              <tr>
                <td style="background-color: #0f766e; border-radius: 6px;">
                  <a href="{{.ActionURL}}" target="_blank" style="display: inline-block; padding: 12px 24px; color: #ffffff; font-size: 15px; font-weight: 600; text-decoration: none; border-radius: 6px;">Go to Dashboard</a>
                </td>
              </tr>
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">If you have any questions, our support team is always here to help.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'welcome'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Auth: Login Alert
-- Variables: SiteName, UserName, DeviceInfo, Location, Timestamp, SupportURL
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>New Sign-In to Your {{.SiteName}} Account</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">A new sign-in to your {{.SiteName}} account was detected. Here are the details:</p>
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin: 16px 0;">
              <tr>
                <td style="padding: 8px 0; border-bottom: 1px solid #eeeeee;">
                  <span style="display: inline-block; width: 120px; color: #666666; font-size: 13px;">Device:</span>
                  <span style="color: #333333; font-size: 14px;">{{.DeviceInfo}}</span>
                </td>
              </tr>
              <tr>
                <td style="padding: 8px 0; border-bottom: 1px solid #eeeeee;">
                  <span style="display: inline-block; width: 120px; color: #666666; font-size: 13px;">Location:</span>
                  <span style="color: #333333; font-size: 14px;">{{.Location}}</span>
                </td>
              </tr>
              <tr>
                <td style="padding: 8px 0;">
                  <span style="display: inline-block; width: 120px; color: #666666; font-size: 13px;">Time:</span>
                  <span style="color: #333333; font-size: 14px;">{{.Timestamp}}</span>
                </td>
              </tr>
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">If this wasn't you, please reset your password immediately.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'login_alert'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Auth: Account Deletion Confirmed
-- Variables: SiteName, UserName, SupportURL
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Account Deletion Confirmed</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">We're writing to confirm that your {{.SiteName}} account has been permanently deleted.</p>
            <p style="margin: 0 0 16px 0; color: #666666; font-size: 13px;">All of your data, including posts, messages, and preferences, has been removed from our systems in accordance with our privacy policy.</p>
            <p style="margin: 0; color: #666666; font-size: 13px;">If this was a mistake, please contact our support team.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'account_deletion_confirmed'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Auth: Email Changed
-- Variables: SiteName, UserName, SupportURL
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Email Address Changed</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">The email address on your {{.SiteName}} account has been changed.</p>
            <p style="margin: 0 0 16px 0; color: #666666; font-size: 13px;">If you made this change, no further action is needed. If you didn't make this change, please contact our support team immediately.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'email_changed'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Payments: Payment Receipt
-- Variables: SiteName, UserName, Amount, Currency, Description, TransactionID, PaymentMethod
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Payment Receipt from {{.SiteName}}</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">Thank you for your payment. Here are the details:</p>
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin: 16px 0;">
              <tr>
                <td style="padding: 8px 0; border-bottom: 1px solid #eeeeee;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Amount:</span>
                  <span style="color: #333333; font-size: 14px;">{{.Amount}} {{.Currency}}</span>
                </td>
              </tr>
              <tr>
                <td style="padding: 8px 0; border-bottom: 1px solid #eeeeee;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Description:</span>
                  <span style="color: #333333; font-size: 14px;">{{.Description}}</span>
                </td>
              </tr>
              <tr>
                <td style="padding: 8px 0; border-bottom: 1px solid #eeeeee;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Transaction ID:</span>
                  <span style="color: #333333; font-size: 14px;">{{.TransactionID}}</span>
                </td>
              </tr>
              <tr>
                <td style="padding: 8px 0;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Payment method:</span>
                  <span style="color: #333333; font-size: 14px;">{{.PaymentMethod}}</span>
                </td>
              </tr>
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">A receipt has been sent to your email address.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'payment_receipt'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Payments: Payment Failed
-- Variables: SiteName, UserName, Amount, Currency, FailureReason
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Payment Failed</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">Your payment of <strong>{{.Amount}} {{.Currency}}</strong> was unsuccessful.</p>
            <p style="margin: 0 0 16px 0; color: #666666; font-size: 13px;">Reason: {{.FailureReason}}</p>
            <p style="margin: 0; color: #666666; font-size: 13px;">Please update your payment method or try again.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'payment_failed'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Payments: Subscription Created
-- Variables: SiteName, UserName, PlanName, NextBillingDate, SiteURL
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Subscription Started</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">Your <strong>{{.PlanName}}</strong> subscription is now active. Thank you for subscribing!</p>
            <table role="presentation" cellpadding="0" cellspacing="0" style="margin: 24px 0;">
              <tr>
                <td style="background-color: #0f766e; border-radius: 6px;">
                  <a href="{{.SiteURL}}" target="_blank" style="display: inline-block; padding: 12px 24px; color: #ffffff; font-size: 15px; font-weight: 600; text-decoration: none; border-radius: 6px;">Go to Dashboard</a>
                </td>
              </tr>
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">Next billing date: {{.NextBillingDate}}</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'subscription_created'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Payments: Subscription Cancelled
-- Variables: SiteName, UserName, PlanName, AccessUntil, SiteURL
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Subscription Cancelled</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">Your <strong>{{.PlanName}}</strong> subscription has been cancelled.</p>
            <p style="margin: 0 0 16px 0; color: #666666; font-size: 13px;">You will have access to premium features until <strong>{{.AccessUntil}}</strong>.</p>
            <table role="presentation" cellpadding="0" cellspacing="0" style="margin: 24px 0;">
              <tr>
                <td style="background-color: #0f766e; border-radius: 6px;">
                  <a href="{{.SiteURL}}" target="_blank" style="display: inline-block; padding: 12px 24px; color: #ffffff; font-size: 15px; font-weight: 600; text-decoration: none; border-radius: 6px;">Go to Dashboard</a>
                </td>
              </tr>
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">We hope to see you again soon.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'subscription_cancelled'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Payments: Subscription Expiring
-- Variables: SiteName, UserName, PlanName, ExpiryDate, SiteURL
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Subscription Expiring Soon</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">Your <strong>{{.PlanName}}</strong> subscription will expire on <strong>{{.ExpiryDate}}</strong>.</p>
            <p style="margin: 0 0 16px 0; color: #666666; font-size: 13px;">Renew now to maintain uninterrupted access to all features.</p>
            <table role="presentation" cellpadding="0" cellspacing="0" style="margin: 24px 0;">
              <tr>
                <td style="background-color: #0f766e; border-radius: 6px;">
                  <a href="{{.SiteURL}}" target="_blank" style="display: inline-block; padding: 12px 24px; color: #ffffff; font-size: 15px; font-weight: 600; text-decoration: none; border-radius: 6px;">Renew Now</a>
                </td>
              </tr>
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">Questions? Contact our support team.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'subscription_expiring'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Learning: Certificate Issued
-- Variables: SiteName, UserName, CourseName, CertificateURL
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Certificate Issued</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">Congratulations! Your certificate for <strong>{{.CourseName}}</strong> is ready.</p>
            <table role="presentation" cellpadding="0" cellspacing="0" style="margin: 24px 0;">
              <tr>
                <td style="background-color: #0f766e; border-radius: 6px;">
                  <a href="{{.CertificateURL}}" target="_blank" style="display: inline-block; padding: 12px 24px; color: #ffffff; font-size: 15px; font-weight: 600; text-decoration: none; border-radius: 6px;">View Certificate</a>
                </td>
              </tr>
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">You can also download your certificate at any time from your learning dashboard.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'certificate_issued'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Learning: Course Completed
-- Variables: SiteName, UserName, CourseName, SiteURL
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Course Completed</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">Congratulations! You have completed <strong>{{.CourseName}}</strong>.</p>
            <p style="margin: 0 0 16px 0; color: #666666; font-size: 13px;">Your dedication and effort have paid off. You can now view your certificate and share your achievement with others.</p>
            <table role="presentation" cellpadding="0" cellspacing="0" style="margin: 24px 0;">
              <tr>
                <td style="background-color: #0f766e; border-radius: 6px;">
                  <a href="{{.SiteURL}}" target="_blank" style="display: inline-block; padding: 12px 24px; color: #ffffff; font-size: 15px; font-weight: 600; text-decoration: none; border-radius: 6px;">View Certificate</a>
                </td>
              </tr>
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">Keep learning and exploring new opportunities.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'course_completed'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Support: Ticket Confirmation
-- Variables: SiteName, UserName, TicketID, Subject
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Ticket #{{.TicketID}} Received</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">Your support ticket has been received.</p>
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin: 16px 0;">
              <tr>
                <td style="padding: 8px 0; border-bottom: 1px solid #eeeeee;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Ticket ID:</span>
                  <span style="color: #333333; font-size: 14px;">{{.TicketID}}</span>
                </td>
              </tr>
              <tr>
                <td style="padding: 8px 0;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Subject:</span>
                  <span style="color: #333333; font-size: 14px;">{{.Subject}}</span>
                </td>
              </tr>
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">We will respond to your ticket as soon as possible.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'ticket_confirmation'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Support: Ticket Status Update
-- Variables: SiteName, UserName, TicketID, Subject, OldStatus, NewStatus, Message
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Ticket #{{.TicketID}} Updated</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">Your ticket <strong>#{{.TicketID}}</strong> has been updated.</p>
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin: 16px 0;">
              <tr>
                <td style="padding: 8px 0; border-bottom: 1px solid #eeeeee;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Subject:</span>
                  <span style="color: #333333; font-size: 14px;">{{.Subject}}</span>
                </td>
              </tr>
              <tr>
                <td style="padding: 8px 0; border-bottom: 1px solid #eeeeee;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Status:</span>
                  <span style="color: #333333; font-size: 14px;">{{.OldStatus}} → {{.NewStatus}}</span>
                </td>
              </tr>
              {{if .Message}}
              <tr>
                <td style="padding: 8px 0;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Message:</span>
                  <span style="color: #333333; font-size: 14px;">{{.Message}}</span>
                </td>
              </tr>
              {{end}}
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">You can reply to this email to add more information to your ticket.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'ticket_status_update'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Support: Ticket Response
-- Variables: SiteName, UserName, TicketID, Subject, Message
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>New Response to Your Ticket #{{.TicketID}}</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">A staff member has responded to your ticket <strong>#{{.TicketID}}</strong>.</p>
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin: 16px 0;">
              <tr>
                <td style="padding: 8px 0; border-bottom: 1px solid #eeeeee;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Subject:</span>
                  <span style="color: #333333; font-size: 14px;">{{.Subject}}</span>
                </td>
              </tr>
            </table>
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin: 16px 0; background-color: #f8fafc; border-radius: 6px;">
              <tr>
                <td style="padding: 16px; color: #333333; font-size: 14px; line-height: 1.6;">
                  {{.Message}}
                </td>
              </tr>
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">You can reply to this email to continue the conversation.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'ticket_response'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Digest: Daily Digest
-- Variables: SiteName, UserName, DigestContent, SiteURL
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Your Daily Digest from {{.SiteName}}</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">Here's your daily digest from {{.SiteName}}.</p>
            {{.DigestContent}}
            <table role="presentation" cellpadding="0" cellspacing="0" style="margin: 24px 0;">
              <tr>
                <td style="background-color: #0f766e; border-radius: 6px;">
                  <a href="{{.SiteURL}}" target="_blank" style="display: inline-block; padding: 12px 24px; color: #ffffff; font-size: 15px; font-weight: 600; text-decoration: none; border-radius: 6px;">View All</a>
                </td>
              </tr>
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">You can manage your notification preferences in your account settings.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'daily_digest'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Digest: Weekly Digest
-- Variables: SiteName, UserName, DigestContent, SiteURL
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Your Weekly Digest from {{.SiteName}}</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{if .UserName}}<p style="margin: 0 0 16px 0;">Hi {{.UserName}},</p>{{end}}
            <p style="margin: 0 0 16px 0;">Here's your weekly digest from {{.SiteName}}.</p>
            {{.DigestContent}}
            <table role="presentation" cellpadding="0" cellspacing="0" style="margin: 24px 0;">
              <tr>
                <td style="background-color: #0f766e; border-radius: 6px;">
                  <a href="{{.SiteURL}}" target="_blank" style="display: inline-block; padding: 12px 24px; color: #ffffff; font-size: 15px; font-weight: 600; text-decoration: none; border-radius: 6px;">View All</a>
                </td>
              </tr>
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">You can manage your notification preferences in your account settings.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'weekly_digest'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Admin: New User
-- Variables: SiteName, NewUsername, NewEmail, Timestamp
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>New User Registered on {{.SiteName}}</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            <p style="margin: 0 0 16px 0;">A new user has registered on {{.SiteName}}.</p>
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin: 16px 0;">
              <tr>
                <td style="padding: 8px 0; border-bottom: 1px solid #eeeeee;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Username:</span>
                  <span style="color: #333333; font-size: 14px;">{{.NewUsername}}</span>
                </td>
              </tr>
              <tr>
                <td style="padding: 8px 0; border-bottom: 1px solid #eeeeee;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Email:</span>
                  <span style="color: #333333; font-size: 14px;">{{.NewEmail}}</span>
                </td>
              </tr>
              <tr>
                <td style="padding: 8px 0;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Registered:</span>
                  <span style="color: #333333; font-size: 14px;">{{.Timestamp}}</span>
                </td>
              </tr>
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">Review the new registration in the admin panel.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'admin_new_user'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Admin: Content Reported
-- Variables: SiteName, ReporterUsername, Reason, ContentURL
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Content Reported on {{.SiteName}}</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            <p style="margin: 0 0 16px 0;">Content has been reported on {{.SiteName}}.</p>
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin: 16px 0;">
              <tr>
                <td style="padding: 8px 0; border-bottom: 1px solid #eeeeee;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Reported by:</span>
                  <span style="color: #333333; font-size: 14px;">{{.ReporterUsername}}</span>
                </td>
              </tr>
              <tr>
                <td style="padding: 8px 0;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Reason:</span>
                  <span style="color: #333333; font-size: 14px;">{{.Reason}}</span>
                </td>
              </tr>
            </table>
            <table role="presentation" cellpadding="0" cellspacing="0" style="margin: 24px 0;">
              <tr>
                <td style="background-color: #0f766e; border-radius: 6px;">
                  <a href="{{.ContentURL}}" target="_blank" style="display: inline-block; padding: 12px 24px; color: #ffffff; font-size: 15px; font-weight: 600; text-decoration: none; border-radius: 6px;">View Reported Content</a>
                </td>
              </tr>
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">Please review the reported content and take appropriate action.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'admin_content_reported'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Admin: Appeal Submitted
-- Variables: SiteName, AppellantUsername, ActionType, Reason
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>Moderation Appeal Submitted</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            <p style="margin: 0 0 16px 0;">A user has appealed a moderation decision on {{.SiteName}}.</p>
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin: 16px 0;">
              <tr>
                <td style="padding: 8px 0; border-bottom: 1px solid #eeeeee;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Appellant:</span>
                  <span style="color: #333333; font-size: 14px;">{{.AppellantUsername}}</span>
                </td>
              </tr>
              <tr>
                <td style="padding: 8px 0; border-bottom: 1px solid #eeeeee;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Action:</span>
                  <span style="color: #333333; font-size: 14px;">{{.ActionType}}</span>
                </td>
              </tr>
              <tr>
                <td style="padding: 8px 0;">
                  <span style="display: inline-block; width: 140px; color: #666666; font-size: 13px;">Reason:</span>
                  <span style="color: #333333; font-size: 14px;">{{.Reason}}</span>
                </td>
              </tr>
            </table>
            <p style="margin: 0; color: #666666; font-size: 13px;">Please review the appeal in the admin panel.</p>
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'admin_appeal_submitted'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- ============================================================
-- Marketing: Newsletter
-- Variables: SiteName, Subject, Content
-- ============================================================
UPDATE email_templates
SET html_body = $$<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<meta name="color-scheme" content="light">
<title>{{.Subject}}</title>
</head>
<body style="margin: 0; padding: 0; background-color: #f4f4f7; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color: #f4f4f7; padding: 32px 0;">
  <tr>
    <td align="center">
      <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width: 600px; width: 100%; background-color: #ffffff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 8px rgba(0,0,0,0.06);">
        <!-- Header -->
        <tr>
          <td style="background-color: #0f766e; padding: 24px 32px; text-align: center;">
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>
          </td>
        </tr>
        <!-- Body -->
        <tr>
          <td style="padding: 32px; color: #333333; font-size: 15px; line-height: 1.6;">
            {{.Content}}
          </td>
        </tr>
        <!-- Footer -->
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
</html>$$,
    updated_at = now()
WHERE type = 'newsletter'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

COMMIT;
