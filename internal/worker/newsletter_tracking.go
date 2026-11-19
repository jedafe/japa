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
	"context"
	"net/url"
	"os"

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

// injectNewsletterTracking rewrites a campaign HTML body for the given
// recipient: every http(s) <a href> is wrapped through
// /newsletter/track/click (signed for that exact destination) and a 1px
// open pixel is appended. Run BEFORE appendUnsubscribeLink so the
// one-click unsubscribe URL is never wrapped (§62 self-hosted transport).
// Returns the input unchanged when the feature is unconfigured — a
// missing secret or API base must never block delivery.
func injectNewsletterTracking(ctx context.Context, campaignID pgtype.UUID, recipient, htmlBody string) string {
	secret := newsletterTrackingSecret()
	apiBase := newsletterAPIBase(ctx)

	if secret == "" || apiBase == "" {
		return htmlBody
	}

	cid := campaignID.String()
	esc := url.QueryEscape

	openURL := apiBase + "/newsletter/track/open" +
		"?c=" + esc(cid) +
		"&e=" + esc(recipient) +
		"&s=" + email.SignTracking(secret, "open", cid, recipient)

	return email.InjectEmailTracking(htmlBody, openURL, func(dest string) string {
		return apiBase + "/newsletter/track/click" +
			"?c=" + esc(cid) +
			"&e=" + esc(recipient) +
			"&u=" + esc(dest) +
			"&s=" + email.SignTracking(secret, "click", cid, recipient, dest)
	})
}
