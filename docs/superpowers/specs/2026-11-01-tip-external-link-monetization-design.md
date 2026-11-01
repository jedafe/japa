# Design: External Tip Link Click Tracking + Affiliate Monetization

**Date:** 2026-11-01
**Status:** Approved direction (Option A + §56-parity click storage)
**JEOS sections touched:** §125 (Donations & Tips) · §55/§56 (Affiliate providers/program — reused, unchanged) · §11 (Asynq catalog) · Valkey keys catalog · API routes catalog

## Problem

User-submitted external tip links (`tip_external_links` — PayPal.me, Ko-fi, Buy Me a Coffee, `other`) are display-only outbound links. The platform earns nothing from them: no click data, no affiliate monetization.

## Goal

1. Route every external tip link click through a platform redirect endpoint so clicks are counted (deduplicated) and recorded.
2. When the destination domain matches a registered, active `affiliate_providers` entry (§55), inject the provider's `tracking_param=tracking_value` into the redirect target so the platform earns the affiliate commission.
3. Surface `click_count` back to the link owner in their tip-settings dashboard.

Platform keeps 100% of affiliate revenue — no commission attribution to the link owner (decided).

## Non-Goals

- No commission sharing / author payout plumbing (`author_commission_pct` remains content-only).
- No admin analytics UI for clicks in this pass (counter + events table only).
- No changes to the §56 user affiliate program (`/r/{code}` stays untouched).
- No frontend changes — `link.url` is rendered verbatim into `<a href>` today; returning the redirect URL in that field is transparent.

## Design

### Route

```
GET /api/v1/tips/links/{id}/out      [public, RateLimit "loose"]  → 302 to destination
```

Mounted inside the existing `tips.enabled` feature-gated route group (server.go ~5274, alongside `/users/{username}/tip-config`).

**Handler flow** (`tips.Handlers.RedirectExternalLink`):

1. `chi.URLParam("id")` → scan UUID; on failure or link-not-found → `302 /` (same fallback as §56 `affiliate.Redirect` for unknown codes).
2. **Dedup**: `jc:tip_link_dedup:{link_id}:{ip_sha256}:{hour_bucket}` SET NX, 1h TTL. Duplicate → skip recording, still redirect.
3. **Record**: fire-and-forget `tips:record_link_click` Asynq enqueue `{link_id, ip_hash, ua_hash, referrer_url}` (Rule 8 — never inline).
4. **Monetize**: if `feature.affiliate_program` enabled and the destination host matches an active provider's `base_url` host (exact or subdomain suffix), append `?tracking_param=tracking_value` (`&` when the URL already has a query).
5. **Redirect**: scheme-check destination (http/https only, else `302 /`) → `http.StatusFound`.

### API payload change

`PublicConfig.ExternalLinks` (and `/donate` config) return:

```json
{ "id": "...", "kind": "paypal", "label": "...", "url": "{siteURL}/api/v1/tips/links/{id}/out", "position": 0 }
```

- `url` is rewritten to the platform redirect using the existing `s.siteURL` (`tipsService.SetSiteURL(cfg.SiteURL)` — same convention as the OC webhook URL at settings.go:33).
- Owner-facing `ListLinks` (dashboard) returns the **raw** stored URL plus `click_count` — owners still see/edit their real URLs.
- The stored `tip_external_links.url` is never mutated — tracking params resolve at click time, so rotating `tracking_value` takes effect immediately and links that predate a provider registration monetize automatically.

### Provider matching (new seam)

`coupons.Service` gains one method:

```go
// ResolveAffiliateLinkByURL matches rawURL's host against active providers'
// base_url hosts (exact or subdomain suffix) and returns the tracking-injected
// URL. Returns (nil, nil) when no provider matches.
ResolveAffiliateLinkByURL(ctx, rawURL) (*RenderedAffiliateLink, error)
```

Backed by a new sqlc query `ListActiveAffiliateProvidersForMatch` (id, base_url, tracking_param, tracking_value — small table, fine to scan).

`tips.Service` gets a seam, wired in server.go via an adapter (same pattern as `affiliateLinkRenderAdapter` — avoids a tips→coupons import dependency):

```go
type AffiliateURLResolver interface {
    ResolveAffiliateLinkByURL(ctx context.Context, rawURL string) (resolved string, matched bool, err error)
}
func (s *Service) SetAffiliateResolver(r AffiliateURLResolver)
```

Nil seam → skip injection, plain redirect (unit-test friendly, mirrors existing nil-seam conventions).

### Schema (migration `001008_tip_link_click_tracking`, up/down pair)

```sql
ALTER TABLE tip_external_links
  ADD COLUMN click_count bigint NOT NULL DEFAULT 0;

CREATE TABLE tip_external_link_clicks (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  link_id         uuid NOT NULL REFERENCES tip_external_links(id) ON DELETE CASCADE,
  ip_hash         text,
  user_agent_hash text,
  referrer_url    text,
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON tip_external_link_clicks (link_id, created_at DESC);
```

Events table + counter = §56 `affiliate_link_events`/`total_clicks` parity. Hashed IP/UA only — no user_id, pseudonymous by construction; link deletion cascades clicks (GDPR clean).

### Asynq task

| Task | Queue | Description |
|---|---|---|
| `tips:record_link_click` | `default` | Insert `tip_external_link_clicks` row + increment `tip_external_links.click_count` |

- `worker.TypeTipsRecordLinkClick` constant + `HandleTipsRecordLinkClick` (tasks.go handler map).
- Added to the queue-routing table in `enqueuer.go` (`default`).
- NOT added to the dedupe-eligible task map — each click is a distinct event (dedup already happened at the Valkey layer).

### sqlc additions (`db/queries/tips.sql`, regenerate via `make sqlc-gen`)

```sql
-- name: GetTipExternalLinkByID :one
SELECT ... FROM tip_external_links WHERE id = $1;

-- name: InsertTipLinkClick :exec
INSERT INTO tip_external_link_clicks (link_id, ip_hash, user_agent_hash, referrer_url) VALUES ($1,$2,$3,$4);

-- name: IncrementTipLinkClickCount :exec
UPDATE tip_external_links SET click_count = click_count + 1 WHERE id = $1;
```

Plus `ListActiveAffiliateProvidersForMatch` in the coupons query file.

### Valkey key (catalog addition)

| Key | Pattern | TTL |
|---|---|---|
| `jc:tip_link_dedup:{link_id}:{ip_hash}:{hour}` | SET NX click dedup | 1h |

### DTO changes

`tips.ExternalLink` gains `ClickCount int64 json:"click_count"` (owner dashboard view; harmless on public payloads — it's already public-adjacent data).

### JEOS updates (Rule: JEOS before code)

1. `sections/125-donations-tips.md` — amend "External links (display only)" section: clicks route through `/tips/links/{id}/out`, provider-matched hosts get tracking injection, schema + route + task + key rows.
2. `sections/11-job-queue-asynq.md` — add `tips:record_link_click` to the task catalog.
3. `conventions/valkey-keys-catalog.md` — add `jc:tip_link_dedup:*` pattern.
4. `conventions/api-routes-catalog.md` — add `GET /api/v1/tips/links/{id}/out`.

### Edge cases

- Link not found / bad UUID → `302 /`.
- `tips.enabled` off → route unmounted (404) — acceptable, links disappear from configs anyway.
- `feature.affiliate_program` off → redirect still works, no injection.
- Provider with empty tracking_param/value → URL unchanged (existing `ResolveAffiliateLinkForRender` behavior).
- Destination fails http/https scheme check → `302 /` (open-redirect guard; stored links are https-validated at write time, this is defense-in-depth).

## Testing

- `internal/tips`: handler tests — dedup (second click same IP/hour doesn't double-record), provider-matched URL gets params appended, unmatched redirects clean, bad id → `/`, scheme rejection.
- `internal/coupons`: `ResolveAffiliateLinkByURL` — exact host, subdomain, no-match nil, inactive provider skipped.
- Worker: `HandleTipsRecordLinkClick` — row + counter increment, idempotent-safe.
- Manual: create link via API → hit `/tips/links/{id}/out` → 302 + `click_count` 1 → repeat → still 1.
