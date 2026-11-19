// Package worker — newsletter_tracking.go injects native open/click
// tracking into campaign bodies at send time for self-hosted MTA
// transport (§62, ADR-080). Provider transports emit engagement webhooks;
// a bare SMTP MTA emits nothing, so the worker wraps every http(s) link
// through the click endpoint and appends an HMAC-signed 1px open pixel.
// Injection is skipped entirely when no signing secret is configured —
// mail sends untracked rather than failing.
//
// Ref: japaconnect-engineering/sections/62-newsletter-digest.md
package worker

import (
	"net/url"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/email"
)

// newsletterTrackingSecret resolves the tracking-URL signing secret for
// the worker — mirrors newsletter.Service.TrackingSecret without
// importing the newsletter package (worker uses gen directly).
func newsletterTrackingSecret() string {
	if v := os.Getenv("JC_NEWSLETTER_TRACKING_SECRET"); v != "" {
		return v
	}

	return os.Getenv("JC_NEWSLETTER_WEBHOOK_SECRET")
}

// newsletterRecipientTag derives the opaque per-recipient identifier
// carried in tracking URLs' e param — hex(hmac(secret, "rid"|email))
// truncated to 32 chars. Stable per (secret, address) so signature
// verification and event dedupe work unchanged, but the raw address
// never appears in URLs, redirects, or access logs (§62 privacy).
func newsletterRecipientTag(secret, recipient string) string {
	return email.SignTracking(secret, "rid", strings.ToLower(strings.TrimSpace(recipient)))[:32]
}

// injectNewsletterTracking rewrites a campaign HTML body for the given
// recipient: every http(s) <a href> is wrapped through
// /newsletter/track/click (signed for that exact destination) and a 1px
// open pixel is appended. Run BEFORE appendUnsubscribeLink so the
// one-click unsubscribe URL is never wrapped (§62 self-hosted transport).
// Returns the input unchanged when the feature is unconfigured — a
// missing secret, API base, or campaign ID must never block delivery
// (a wrapped click with no campaign context would 400 on receipt).
func injectNewsletterTracking(apiBase string, campaignID pgtype.UUID, recipient, htmlBody string) string {
	secret := newsletterTrackingSecret()

	if secret == "" || apiBase == "" || !campaignID.Valid {
		return htmlBody
	}

	cid := campaignID.String()
	rid := newsletterRecipientTag(secret, recipient)
	esc := url.QueryEscape

	openURL := apiBase + "/newsletter/track/open" +
		"?c=" + esc(cid) +
		"&e=" + rid +
		"&s=" + email.SignTracking(secret, "open", cid, rid)

	return email.InjectEmailTracking(htmlBody, openURL, func(dest string) string {
		return apiBase + "/newsletter/track/click" +
			"?c=" + esc(cid) +
			"&e=" + rid +
			"&u=" + esc(dest) +
			"&s=" + email.SignTracking(secret, "click", cid, rid, dest)
	})
}
