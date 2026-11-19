// Package newsletter — tracking.go implements the service side of native
// open/click engagement tracking for self-hosted MTA transport (§62,
// ADR-080 §4). Provider transports emit engagement webhooks consumed by
// HandleWebhook; a bare SMTP MTA emits nothing, so campaign sends wrap
// links and append an open pixel whose URLs carry an HMAC signature, and
// these endpoints verify + count the events — deduped per
// (campaign, recipient[, link]) so scanner prefetch and re-opens don't
// inflate rates.
//
// Ref: japaconnect-engineering/sections/62-newsletter-digest.md
package newsletter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// TrackScopeOpen / TrackScopeClick are the signature scope discriminators
// for tracking URLs — bound into the HMAC input so an open signature can
// never verify as a click signature and vice versa (§62 self-hosted
// transport).
const (
	TrackScopeOpen  = "open"
	TrackScopeClick = "click"
)

// trackDedupeTTL bounds how long a (campaign, recipient[, link]) event is
// remembered — a second hit inside the window is acknowledged but not
// counted (§62 Compliance: scanner prefetch + re-open suppression).
const trackDedupeTTL = 7 * 24 * time.Hour

// TrackingSecret resolves the HMAC secret signing native tracking URLs.
// JC_NEWSLETTER_TRACKING_SECRET wins; JC_NEWSLETTER_WEBHOOK_SECRET is the
// fallback so a single-secret deployment tracks out of the box. Empty
// means tracking is off — send-time injection is skipped and the
// endpoints reject (fail closed, matching WebhookSecret).
func (s *Service) TrackingSecret() string {
	if v := os.Getenv("JC_NEWSLETTER_TRACKING_SECRET"); v != "" {
		return v
	}

	return os.Getenv("JC_NEWSLETTER_WEBHOOK_SECRET")
}

// RecordTrackEvent counts a verified open or click against a campaign.
// kind is TrackScopeOpen or TrackScopeClick. Dedupes via Valkey
// jc:newsletter_track:{kind}:{campaign_id}:{sha256} — the hash binds
// (recipient) for opens and (recipient|link) for clicks — so repeated
// hits within trackDedupeTTL count once. A nil Valkey client degrades to
// always-count (same contract as the webhook path's missing dedupe).
func (s *Service) RecordTrackEvent(ctx context.Context, kind, campaignID, recipient, link string) error {
	if kind != TrackScopeOpen && kind != TrackScopeClick {
		return fmt.Errorf("newsletter: unknown track kind %q", kind)
	}

	digestInput := strings.ToLower(strings.TrimSpace(recipient))
	if kind == TrackScopeClick {
		digestInput += "|" + link
	}

	sum := sha256.Sum256([]byte(digestInput))
	dedupeKey := fmt.Sprintf("jc:newsletter_track:%s:%s:%s", kind, campaignID, hex.EncodeToString(sum[:]))

	vk := s.WebhookDedupe()

	if vk != nil {
		set, err := vk.SetNX(ctx, dedupeKey, 1, trackDedupeTTL).Result()
		if err != nil {
			// Fail-open: count without dedupe rather than drop the event
			// (same contract as the webhook path) — but make it visible
			// so a Valkey outage is diagnosable from logs.
			slog.Warn("newsletter track: dedupe check failed, counting without dedupe",
				"kind", kind, "campaign_id", campaignID, "error", err)
		} else if !set {
			return nil // already counted inside the dedupe window
		}
	}

	var err error

	if kind == TrackScopeOpen {
		err = s.IncrementCampaignOpens(ctx, campaignID)
	} else {
		err = s.IncrementCampaignClicks(ctx, campaignID)
	}

	if err != nil && vk != nil {
		// Release the dedupe claim — otherwise a counted-but-failed event
		// would be suppressed for the full TTL and never counted.
		_, _ = vk.Del(ctx, dedupeKey).Result()
	}

	return err
}

// trackingScannerUAs are lowercase User-Agent substrings of known
// mailbox-provider image proxies and corporate link scanners. Their
// pixel/link hits are machine fetches, not recipient engagement — the
// endpoints still serve the pixel / follow the redirect, but the hit is
// not counted so prefetch doesn't inflate rates (§62 self-hosted
// transport). Matching is case-insensitive substring, so entries stay
// specific (never "mozilla") — over-matching only loses a count, while
// under-matching inflates.
var trackingScannerUAs = []string{
	"googleimageproxy", // Gmail image proxy
	"yahoomailproxy",   // Yahoo image proxy
	"outlook-ios",      // Outlook app image proxy fetcher
	"outlook-android",
	"barracuda",  // Barracuda link/image inspection
	"proofpoint", // Proofpoint URL Defense
	"mimecast",   // Mimecast URL protection
	"forcepoint", // Forcepoint email security
	"sophos",     // Sophos email gateway
	"mcafee",     // McAfee/Skyhigh secure web gateway
	"trendmicro", // Trend Micro HES
	"trend micro",
	"zscaler",  // Zscaler URL inspection
	"defender", // Microsoft Defender SafeLinks
	"safelinks",
}

// IsTrackingScanner reports whether a User-Agent belongs to a known
// prefetcher/scanner whose hits must not count as engagement. Deployments
// can append extra substrings via JC_NEWSLETTER_TRACKING_IGNORE_UA
// (comma-separated, e.g. a site-specific gateway).
func IsTrackingScanner(ua string) bool {
	if ua == "" {
		return false
	}

	l := strings.ToLower(ua)

	for _, sub := range trackingScannerUAs {
		if strings.Contains(l, sub) {
			return true
		}
	}

	for sub := range strings.SplitSeq(os.Getenv("JC_NEWSLETTER_TRACKING_IGNORE_UA"), ",") {
		if sub = strings.ToLower(strings.TrimSpace(sub)); sub != "" && strings.Contains(l, sub) {
			return true
		}
	}

	return false
}
