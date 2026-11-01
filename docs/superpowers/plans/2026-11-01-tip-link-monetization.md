# Tip External Link Click Tracking + Affiliate Monetization — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use executing-plans. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route `tip_external_links` clicks through a platform redirect that dedup-records clicks and injects affiliate tracking params when the destination domain matches an active `affiliate_providers` row — platform keeps 100% of affiliate revenue.

**Spec:** `docs/superpowers/specs/2026-11-01-tip-external-link-monetization-design.md`

**Architecture:** New public route `GET /api/v1/tips/links/{id}/out` (mirrors §56 `/r/{code}`): Valkey SET NX dedup → fire-and-forget `tips:record_link_click` Asynq task → optional affiliate tracking injection via a new `coupons.Service.ResolveAffiliateLinkByURL` → 302. `PublicConfig.external_links[].url` returns the redirect URL, so zero frontend changes.

**Global Constraints:**
- JEOS updated BEFORE code (AGENTS.md). Tasks 1–4 are spec/catalog updates.
- sqlc: edit `internal/db/queries/*.sql` only, never `internal/db/gen` (Rule 2) — regenerate via `make sqlc-gen`.
- New Asynq task names must exist in the §11 catalog first (Rule 7).
- New Valkey keys must be `jc:`-prefixed and cataloged (Rule 4).
- Errors use `errpkg.WriteError` shape; no raw-PII in task payloads (ip/ua are SHA-256 hex before enqueue — §56 pattern).
- Commit to `main`. No AI attribution in commits/comments (Rule 17). Go code references sections via `// Ref:`/`§` comments.

---

### Task 1: JEOS — §125 amendment

**Files:**
- Modify: `.jeos/sections/125-donations-tips.md`

- [ ] Replace the "External links (display only)" block (~lines 184-188) with the amended behavior, add the route to the API Routes block, the task to the Asynq Tasks table, and the key to Valkey Keys table:

```markdown
### External links (click-tracked outbound)

`tip_external_links` rows render as outbound options in the tip dialog, but the
`url` returned in public tip-configs is a platform redirect —
`GET /api/v1/tips/links/{id}/out`. The endpoint dedups clicks
(`jc:tip_link_dedup`, 1h), records them async (`tips:record_link_click` →
`tip_external_link_clicks` + `tip_external_links.click_count`), and when the
destination host matches an active `affiliate_providers.base_url` host (§55)
appends the provider's `tracking_param=tracking_value` before the 302 —
platform affiliate revenue, no owner share. Owner-facing link lists still
return the raw stored URL plus `click_count`. Links are stored unmodified so
tracking-param rotation and provider additions apply at click time.
```

Route row to add in the public block of API Routes:

```
GET    /api/v1/tips/links/{id}/out                  [public] click-tracked redirect → destination (+ affiliate params on provider match)
```

Asynq table row:

```
| `tips:record_link_click` | `default` | Persist external-link click event + increment link click_count |
```

Valkey table row:

```
| `jc:tip_link_dedup:{link_id}:{ip_hash}:{hour}` | SET NX click dedup | 1h |
```

- [ ] Commit: `git add .jeos/sections/125-donations-tips.md && git commit -m "docs(jeos): §125 external tip links click tracking + affiliate injection"`

### Task 2: JEOS — §11 task catalog + Valkey catalog + route catalog

**Files:**
- Modify: `.jeos/sections/11-job-queue-asynq.md` — add `tips:record_link_click` (queue `default`, payload `{link_id, ip_hash, ua_hash, referrer_url}`, description "Persist external tip link click + increment click_count")
- Modify: `.jeos/conventions/valkey-keys-catalog.md` — add `jc:tip_link_dedup:{link_id}:{ip_hash}:{hour}` (SET NX, 1h TTL)
- Modify: `.jeos/conventions/api-routes-catalog.md` — add `GET /api/v1/tips/links/{id}/out` under the Section 125 block

- [ ] Commit: `git commit -m "docs(jeos): catalog tips:record_link_click, tip-link dedup key, /out route"`

### Task 3: Migration 001008

**Files:**
- Create: `migrations/001008_tip_link_click_tracking.up.sql`
- Create: `migrations/001008_tip_link_click_tracking.down.sql`

- [ ] up.sql:

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
CREATE INDEX idx_tip_link_clicks_link ON tip_external_link_clicks (link_id, created_at DESC);
```

- [ ] down.sql:

```sql
DROP TABLE IF EXISTS tip_external_link_clicks;
ALTER TABLE tip_external_links DROP COLUMN IF EXISTS click_count;
```

- [ ] Apply + commit: `migrate -path migrations -database "$DATABASE_URL" up 1` (or the project's make target) — verify `tip_external_links` gains `click_count`.

### Task 4: sqlc queries + regen

**Files:**
- Modify: `internal/db/queries/tips.sql` (append)
- Modify: `internal/db/queries/coupons.sql` (append)

- [ ] Append to `tips.sql`:

```sql
-- name: GetTipExternalLinkByID :one
SELECT id, tip_settings_id, kind, label, url, position, click_count, created_at
FROM tip_external_links WHERE id = $1;

-- name: InsertTipLinkClick :exec
INSERT INTO tip_external_link_clicks (link_id, ip_hash, user_agent_hash, referrer_url)
VALUES ($1, $2, $3, $4);

-- name: IncrementTipLinkClickCount :exec
UPDATE tip_external_links SET click_count = click_count + 1 WHERE id = $1;
```

NOTE: `ListTipExternalLinks` / `CreateTipExternalLink` / `UpdateTipExternalLink` column lists must also gain `click_count` — update their SELECT/RETURNING lists so the regenerated `TipExternalLink` scan doesn't fail on the new column.

- [ ] Append to `coupons.sql`:

```sql
-- name: ListActiveAffiliateProvidersForMatch :many
SELECT id, base_url, tracking_param, tracking_value
FROM affiliate_providers
WHERE is_active = true AND base_url IS NOT NULL AND base_url != '';
```

- [ ] Run `make sqlc-gen`; verify `gen.TipExternalLink` has `ClickCount`, `GetTipExternalLinkByID`, `InsertTipLinkClick`, `IncrementTipLinkClickCount`, `ListActiveAffiliateProvidersForMatch` exist. `go build ./internal/db/...`
- [ ] Commit.

### Task 5: `coupons.Service.ResolveAffiliateLinkByURL`

**Files:**
- Modify: `internal/coupons/service.go` (near `ResolveAffiliateLinkForRender` ~line 963)
- Test: `internal/coupons/` (existing test file or new `affiliate_match_test.go`)

- [ ] Failing test — exact host, subdomain, no-match, inactive-skipped, param appends `&` when `?` present:

```go
// fake store returns providers; assert:
// "https://amazon.com/dp/x"        → injected   (host match)
// "https://www.amazon.co.uk/dp/x"  → injected   (subdomain of amazon.co.uk)
// "https://paypal.me/u"            → unchanged / nil match
// URL with existing ?a=1           → "&tag=x" not "?tag=x"
```

- [ ] Implement (uses `ListActiveAffiliateProvidersForMatch` via `s.store`/queries; host match = exact or `.`-suffix):

```go
// ResolveAffiliateLinkByURL matches rawURL's host against active providers'
// base_url hosts (exact or subdomain suffix) and returns the tracking-injected
// URL. (nil, nil) when nothing matches — §125 external tip link monetization.
func (s *Service) ResolveAffiliateLinkByURL(ctx context.Context, rawURL string) (*RenderedAffiliateLink, error)
```

Reuse the param-append logic from `ResolveAffiliateLinkForRender` (extract a shared helper `appendTracking(original, param, value string) string` — both methods call it).

- [ ] `go test ./internal/coupons/ && go build ./...`
- [ ] Commit.

### Task 6: tips service — seam + DTO + PublicConfig URL rewrite

**Files:**
- Modify: `internal/tips/tips.go` — `ExternalLink.ClickCount int64`; `AffiliateURLResolver` seam + `SetAffiliateResolver`; `affResolver` field
- Modify: `internal/tips/settings.go` — `PublicConfig` rewrites link URLs; `listLinks` maps `ClickCount`
- Modify: `internal/tips/store.go` — `getLinkByID(ctx, id)` → `gen.GetTipExternalLinkByID`

- [ ] Add to `tips.go` (near other seams ~line 326):

```go
// AffiliateURLResolver injects platform affiliate tracking into an outbound
// URL when its host matches a registered provider (§55). Nil → skip.
type AffiliateURLResolver interface {
    ResolveAffiliateLinkByURL(ctx context.Context, rawURL string) (resolved string, matched bool, err error)
}
func (s *Service) SetAffiliateResolver(r AffiliateURLResolver) { s.affResolver = r }
```

(Adapter in server.go maps `coupons.RenderedAffiliateLink.ResolvedURL`/nil onto this.)

- [ ] `ExternalLink` gains `ClickCount int64 \`json:"click_count"\``; `listLinks` maps `ClickCount: r.ClickCount`.
- [ ] In `PublicConfig`, after `cfg.ExternalLinks = links`, rewrite each `URL`:

```go
if s.siteURL != "" {
    base := strings.TrimRight(s.siteURL, "/")
    for i := range cfg.ExternalLinks {
        cfg.ExternalLinks[i].URL = base + "/api/v1/tips/links/" + cfg.ExternalLinks[i].ID.String() + "/out"
    }
}
```

(Owner-facing `ListLinks` stays raw — it's the edit surface.)

- [ ] `store.go`: `func (s *Store) getLinkByID(ctx context.Context, id pgtype.UUID) (gen.TipExternalLink, error)` — wrap `s.read.GetTipExternalLinkByID`, map `pgx.ErrNoRows → ErrNotFound` (match existing store error mapping).
- [ ] Failing unit test: `PublicConfig` returns `/api/v1/tips/links/{id}/out` URLs when siteURL set; `ListLinks` returns raw URLs.
- [ ] `go test ./internal/tips/`
- [ ] Commit.

### Task 7: Redirect handler + server wiring

**Files:**
- Modify: `internal/tips/handlers.go` — `RedirectExternalLink`, `Cache`/`AsynqClient` fields, `sha256Hex`, `sanitizeOutURL`
- Modify: `internal/server/server.go` — route + `tipsHandlers.Cache`/`.AsynqClient` + `tipsService.SetAffiliateResolver(&tipsAffAdapter{svc: couponService})`

- [ ] Handler (mirror §56 `affiliate.Redirect`, handlers.go:339-400 — same dedup shape, `loose` rate limit applied at route):

```go
// RedirectExternalLink handles GET /tips/links/{id}/out — dedups the click
// (jc:tip_link_dedup), records it async (tips:record_link_click), injects
// affiliate tracking on provider match, then 302s. Ref: §125 external links.
func (h *Handlers) RedirectExternalLink(w http.ResponseWriter, r *http.Request) {
    var linkID pgtype.UUID
    if err := linkID.Scan(chi.URLParam(r, "id")); err != nil {
        http.Redirect(w, r, "/", http.StatusFound)
        return
    }
    link, err := h.service.getLinkByID(r.Context(), linkID)
    if err != nil { http.Redirect(w, r, "/", http.StatusFound); return }

    ipHash := sha256Hex(clientip.FromRequest(r))
    dedupKey := valkey.MustKey("tip_link_dedup:%s:%s:%s", linkID.String(), ipHash, time.Now().Format("2006-01-02-15"))
    fresh := true
    if h.Cache != nil {
        if set, derr := h.Cache.SetNX(r.Context(), dedupKey, "1", time.Hour).Result(); derr == nil && !set { fresh = false }
    }
    if fresh { h.enqueueLinkClick(linkID, ipHash, sha256Hex(r.Header.Get("User-Agent")), r.Referer()) }

    dest := link.Url
    if res := h.service.affiliateResolver(); res != nil {
        if resolved, matched, rerr := res.ResolveAffiliateLinkByURL(r.Context(), dest); rerr == nil && matched {
            dest = resolved
        }
    }
    http.Redirect(w, r, sanitizeOutURL(dest), http.StatusFound)
}
```

Supporting pieces (copy conventions from `internal/affiliate/handlers.go:28-45,555` and `sanitizeRedirectURL:405`): `TaskEnqueueClient`, `CacheClient` (SetNX only), `Cache`/`AsynqClient` fields on `Handlers`, `sha256Hex`, `sanitizeOutURL` (http/https only else "/"), `enqueueLinkClick` marshals `{link_id, ip_hash, ua_hash, referrer_url}` → `asynq.NewTask(worker.TypeTipsRecordLinkClick, payload)` + `asynq.Queue(worker.QueueForTask(...))`. Service getter `affiliateResolver()` or export the resolver — pick whichever matches existing tips style (fields are unexported; add `func (s *Service) AffiliateResolver() AffiliateURLResolver` — actually simpler: add a service method `ResolveAffiliateURL(ctx, rawURL) (string, error)` that nil-checks the seam + checks `feature.affiliate_program` flag via `s.features`).

- [ ] server.go wiring (~line 994 after `tipsService.SetSiteURL(cfg.SiteURL)`):

```go
tipsService.SetAffiliateResolver(tipsAffAdapter{svc: couponService})
// ...
tipsHandlers.Cache = vkClient.Redis()
tipsHandlers.AsynqClient = asynqClient
```

plus adapter type near `affiliateLinkRenderAdapter` (~line 9970).

- [ ] Route — inside `tips.enabled` group after line 5283:

```go
r.With(mw.RateLimit(vkClient.Redis(), "loose")).Get("/tips/links/{id}/out", tipsHandlers.RedirectExternalLink)
```

- [ ] `go build ./... && go test ./internal/tips/`
- [ ] Commit.

### Task 8: Worker task `tips:record_link_click`

**Files:**
- Modify: `internal/worker/tasks.go` — `TypeTipsRecordLinkClick = "tips:record_link_click"` constant (line ~88) + `{TypeTipsRecordLinkClick, HandleTipsRecordLinkClick}` in handler map (~line 270)
- Modify: `internal/worker/enqueuer.go` — `{TypeTipsRecordLinkClick, "default", "Persist external tip link click + increment click_count"}` in queue table (~line 468)
- Modify: `internal/worker/handlers.go` — `HandleTipsRecordLinkClick` (mirror `HandleAffiliateRecordClick` at line 5890 — parse payload, `gen.InsertTipLinkClick`, `gen.IncrementTipLinkClickCount`; no member-click increment — no member row)

- [ ] `go build ./internal/worker/ && go test ./internal/worker/ -run Tips`
- [ ] Commit.

### Task 9: Tests + end-to-end verification

- [ ] tips handler test (`internal/tips/handlers_test.go` or new file): fake Cache (SetNX returns true then false) + fake AsynqClient — assert second click within hour doesn't enqueue; provider-matched URL redirects with injected param; bad id → `Location: /`.
- [ ] `go build ./... && go test ./internal/tips/ ./internal/coupons/ ./internal/worker/`
- [ ] Manual: server on :8080 — create a link via `POST /me/tip-settings/links` (kind `other`, a domain matching a seeded `affiliate_providers.base_url` if one exists), `GET /users/{username}/tip-config` → assert `url` is `/api/v1/tips/links/{id}/out`; `curl -sv` the out URL → 302 + injected param; repeat → `click_count` stays 1 (dedup); check `tip_external_link_clicks` row.
- [ ] `go vet`/`golangci-lint` on touched packages.
