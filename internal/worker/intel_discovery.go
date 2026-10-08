package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/intelligence/fetcher"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// intel_discovery.go — Phase-4 outbound source discovery
// (intel-channels spec §8.2). intel:detect_changes enqueues
// intel:harvest_outbound for every changed/first-crawl raw doc: the handler
// turns the page's external links into candidate source proposals —
// status='proposed' rows that join the normal admin-review →
// probe_verify → channel-materialization flow. External links are never
// fetched and discovered sources never auto-activate, so the registry
// stays curated while the crawl gains spider-style discovery reach.
//
// Dedupe is by exact hostname against intel_sources (any status) plus
// the exact source_url — a re-run over the same doc is a no-op, and a
// thousand pages linking to the same external host produce one row.
// Granularity is per-host, not per-registrable-domain: a.blog.example
// and b.blog.example are distinct candidates (subdomain sites differ).

type intelHarvestOutboundPayload struct {
	Version   string `json:"version"`
	SourceID  string `json:"source_id"`
	ChannelID string `json:"channel_id,omitempty"`
	RawDocID  string `json:"raw_doc_id"`
}

// intelDiscoveryScanCap bounds how many external links one document may
// contribute before parsing stops — a link-farm page cannot turn one task
// into thousands of registry writes.
const intelDiscoveryScanCap = 200

// intelDiscoveryDenylist is the built-in set of hosts never auto-proposed
// as sources: share-button and social platforms (a footer share link is
// not an editorial reference), plus spec/CDN/utility hosts that appear in
// every page's markup. Matching is by suffix so www.facebook.com and
// m.facebook.com both fall under facebook.com. Admins can still propose
// these hosts manually — the denylist only filters automatic candidates.
var intelDiscoveryDenylist = []string{
	// Social / share-button targets
	"facebook.com", "x.com", "twitter.com", "t.co", "instagram.com",
	"linkedin.com", "youtube.com", "youtu.be", "tiktok.com",
	"pinterest.com", "reddit.com", "t.me", "telegram.me", "telegram.org",
	"whatsapp.com", "wa.me", "threads.net", "snapchat.com",
	"discord.com", "discord.gg", "discordapp.com", "vimeo.com",
	"tumblr.com", "medium.com", "substack.com",
	// Spec / CDN / widget / utility hosts
	"schema.org", "w3.org", "w3c.org", "creativecommons.org",
	"google.com", "googleapis.com", "gstatic.com", "googleusercontent.com",
	"doubleclick.net", "googletagmanager.com", "google-analytics.com",
	"gravatar.com", "cloudflare.com", "jsdelivr.net", "unpkg.com",
	"cdnjs.cloudflare.com", "cdn.jsdelivr.net", "fontawesome.com",
	"wordpress.org", "wordpress.com", "wp.me",
	"apple.com", "itunes.apple.com", "apps.apple.com", "play.google.com",
	"microsoft.com", "amzn.to", "amazon.com",
	"japaconnect.com",
}

// HandleIntelHarvestOutbound processes one changed raw doc: extract the
// outbound link candidates, apply the scope/trust/dedupe gates, and insert
// proposed intel_sources rows for the survivors.
func HandleIntelHarvestOutbound(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelHarvestOutbound")
	slog.Info("task:intel:harvest_outbound")

	if intelStores == nil || intelSourcesStore == nil {
		slog.Warn("task:intel:harvest_outbound: stores not wired, skipping")
		return nil
	}
	// intel.enabled is a feature flag (Task 13); intel.discovery.enabled
	// stays a system_setting — it gates a sub-behavior of the intel
	// pipeline, not whole-feature reachability.
	if !intelEnabled(ctx) {
		slog.Info("task:intel:harvest_outbound: intel.enabled=false, skipping")
		return nil
	}
	if appSettings != nil && !appSettings.GetBoolOr(ctx, "intel.discovery.enabled", true) {
		slog.Info("task:intel:harvest_outbound: intel.discovery.enabled=false, skipping")
		return nil
	}

	var p intelHarvestOutboundPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}

	rawDocID, err := scanUUID(p.RawDocID)
	if err != nil {
		return fmt.Errorf("task:intel:harvest_outbound: raw_doc_id: %w", err)
	}

	rawDoc, err := intelStores.GetIntelRawDocument(ctx, rawDocID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // doc pruned between enqueue and handling
		}
		return fmt.Errorf("task:intel:harvest_outbound: load raw doc: %w", err)
	}

	src, err := intelSourcesStore.Get(ctx, rawDoc.SourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("task:intel:harvest_outbound: load source: %w", err)
	}

	// Trust gate: only links from sources the registry already trusts
	// (official/trusted by default) are worth proposing — a community-tier
	// source's outbound links would flood the review queue.
	maxTrust := 2
	if appSettings != nil {
		maxTrust = appSettings.GetIntOr(ctx, "intel.discovery.max_trust_level", 2)
	}
	if int(src.TrustLevel) > maxTrust {
		return nil
	}

	candidates := intelOutboundCandidates(rawDoc)
	if len(candidates) == 0 {
		return nil
	}

	maxPerCrawl := 10
	if appSettings != nil {
		maxPerCrawl = appSettings.GetIntOr(ctx, "intel.discovery.max_per_crawl", 10)
	}
	if maxPerCrawl <= 0 {
		return nil
	}

	svc := sources.NewService(intelSourcesStore)
	categories := sources.SourceCategories(src)

	created, proposed := 0, 0
	for _, candidate := range candidates {
		if created >= maxPerCrawl {
			break
		}
		if !intelDiscoveryCandidateOK(ctx, candidate, src, rawDoc) {
			continue
		}

		proposed++
		_, ok, err := svc.ProposeDiscovered(ctx, sources.DiscoveredSourceInput{
			URL:          candidate,
			Categories:   categories,
			ReferrerName: src.Name,
			ReferrerURL:  rawDoc.Url,
		})
		if err != nil {
			slog.Warn("task:intel:harvest_outbound: propose failed", "raw_doc_id", rawDoc.ID.String(), "candidate", candidate, "error", err)
			continue
		}
		if ok {
			created++
		}
	}

	if created > 0 {
		slog.Info("task:intel:harvest_outbound: proposed sources", "raw_doc_id", rawDoc.ID.String(), "source_id", src.ID.String(), "channel_id", p.ChannelID, "created", created, "attempted", proposed)
	}

	return nil
}

// intelOutboundCandidates returns the external link set for a raw doc —
// the X-Intel-Outbound-Links header the spider adapter already extracted
// when present, else a bounded HTML parse of the body. Non-HTML docs
// (feeds, NDJSON, manifests) have no anchor graph and yield nothing.
func intelOutboundCandidates(rawDoc gen.IntelRawDocument) []string {
	if v := intelDocHeaders(rawDoc)[fetcher.OutboundLinksHeader]; v != "" {
		var links []string
		if err := json.Unmarshal([]byte(v), &links); err == nil && len(links) > 0 {
			return links
		}
	}

	ct := ""
	if rawDoc.ContentType != nil {
		ct = *rawDoc.ContentType
	}
	if !strings.Contains(ct, "html") {
		return nil
	}

	return fetcher.ExtractOutboundLinks(rawDoc.Body, rawDoc.Url, intelDiscoveryScanCap)
}

// intelDiscoveryCandidateOK applies the per-candidate gates: a valid
// http(s) URL on a real (dotted, non-private) host that is not the
// referring host itself, not under the parent source's domain, and not on
// the built-in or admin-configured blocklist.
func intelDiscoveryCandidateOK(ctx context.Context, candidate string, src gen.IntelSource, rawDoc gen.IntelRawDocument) bool {
	u, err := url.Parse(strings.TrimSpace(candidate))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}

	host := strings.ToLower(u.Hostname())
	if host == "" || !strings.Contains(host, ".") || sources.IsPrivateIPLiteral(host) {
		return false
	}

	// Skip links back at the referring host or the source's own domain —
	// those are the same site, not a discovery.
	if du, derr := url.Parse(rawDoc.Url); derr == nil && strings.EqualFold(du.Hostname(), host) {
		return false
	}
	if d := strings.ToLower(strings.TrimSpace(src.Domain)); d != "" && (host == d || strings.HasSuffix(host, "."+d)) {
		return false
	}

	return !intelDiscoveryBlocked(ctx, host)
}

// intelDiscoveryBlocked reports whether host falls under the built-in
// denylist or intel.discovery.domain_blocklist — suffix matches, so
// blocking facebook.com also blocks www.facebook.com.
func intelDiscoveryBlocked(ctx context.Context, host string) bool {
	blocked := intelDiscoveryDenylist
	if appSettings != nil {
		var extra []string
		raw := appSettings.GetStringOr(ctx, "intel.discovery.domain_blocklist", "[]")
		if err := json.Unmarshal([]byte(raw), &extra); err == nil {
			for _, d := range extra {
				blocked = append(blocked, strings.ToLower(strings.TrimSpace(d)))
			}
		}
	}

	for _, d := range blocked {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}

	return false
}
