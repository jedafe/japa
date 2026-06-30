package email

// HTML email template constants for JapaConnect transactional emails.
//
// Every template uses inline CSS for maximum email client compatibility
// (Gmail, Outlook, Apple Mail, etc. strip <style> tags unpredictably).
// Templates are responsive via max-width containers and fluid tables.
//
// Template variables:
//   - {{.SiteName}}   — "JapaConnect" or the configured site name
//   - {{.SiteURL}}     — base URL of the site (e.g. https://japaconnect.com)
//   - {{.UserName}}    — display name of the recipient (optional, may be empty)
//   - {{.ActionURL}}   — the primary action link (reset password, verify email)
//   - {{.Expiry}}      — human-readable expiry string (e.g. "1 hour", "24 hours")
//   - {{.SupportURL}}  — link to the support/help page

// passwordResetHTML is the HTML body for the password reset email.
const passwordResetHTML = `<!DOCTYPE html>
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
</html>`

// emailVerifyHTML is the HTML body for the email verification email.
const emailVerifyHTML = `<!DOCTYPE html>
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
</html>`

// welcomeHTML is the HTML body for the welcome email sent after first successful login or registration.
const welcomeHTML = `<!DOCTYPE html>
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
</html>`
