// Package email provides HTML email template rendering for JapaConnect's
// transactional emails (password reset, email verification, welcome).
//
// Templates are compiled once at package init via html/template.Must and
// rendered with automatic HTML escaping. All template HTML constants live
// in html.go; this file owns the TemplateData struct and the per-template
// render functions.
//
// Every rendered email is self-contained: inline CSS, no external images
// or stylesheets, responsive via max-width table layout. This maximises
// compatibility across email clients (Gmail, Outlook, Apple Mail, etc.),
// most of which strip <style> tags and external resources.
package email

import (
	"bytes"
	"html/template"
)

// TemplateData carries the values every transactional email template needs.
// Fields are intentionally flat and always populated by the caller — templates
// never reach back into a service or database to resolve missing values.
type TemplateData struct {
	SiteName   string // Brand name shown in header and footer (e.g. "JapaConnect")
	SiteURL    string // Base URL of the site (e.g. "https://japaconnect.com")
	UserName   string // Display name of the recipient; empty string omits the greeting
	ActionURL  string // Primary action link (reset password, verify email, dashboard)
	Expiry     string // Human-readable expiry for the action link (e.g. "1 hour", "24 hours")
	SupportURL string // Link to the support/help page shown in the footer
}

// Parsed templates — compiled once at package init, safe for concurrent use.
// html/template escapes every interpolated value, so caller-supplied strings
// (user names, URLs) cannot inject markup.
var (
	passwordResetTmpl = template.Must(template.New("password_reset").Parse(passwordResetHTML))
	emailVerifyTmpl   = template.Must(template.New("email_verify").Parse(emailVerifyHTML))
	welcomeTmpl       = template.Must(template.New("welcome").Parse(welcomeHTML))
)

// RenderPasswordResetEmail renders the password reset HTML email body.
// Returns the raw HTML string ready to send, or an error if template
// execution fails (should not happen with well-formed data).
func RenderPasswordResetEmail(data TemplateData) (string, error) {
	var buf bytes.Buffer
	if err := passwordResetTmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// RenderEmailVerification renders the email verification HTML email body.
func RenderEmailVerification(data TemplateData) (string, error) {
	var buf bytes.Buffer
	if err := emailVerifyTmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// RenderWelcomeEmail renders the welcome HTML email body.
func RenderWelcomeEmail(data TemplateData) (string, error) {
	var buf bytes.Buffer
	if err := welcomeTmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
