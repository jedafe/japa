# JapaConnect V1.0.0 — Implementation Roadmap

> **For Claude Code.** This document is the authoritative implementation guide for the JapaConnect platform. Read it fully before writing any code. Every section has a Go module path of `github.com/jedafe/japa`. Frontend apps use SvelteKit 2.57.x on Bun 1.3.x. All environment variables are prefixed `JC_`. Valkey key namespace is `jc:`.
>
> **Section detail files** live at `ai-memory/sections/NN-*.md`. This roadmap is the build sequence + code patterns; section files are the schema authority.
>
> **RULES (never violate):**
> - Never use localStorage or sessionStorage in SvelteKit artifacts
> - Never hardcode role checks — all access resolves through `HasPermission(ctx, user, "permission.node")`
> - Never update a wallet balance as a column mutation — append to `wallet_ledger` only
> - Never write Points directly from feature code — emit a typed `ActivityEvent`
> - Never implement Credit spend as a Provider — it's a direct ledger debit
> - Never treat the Honcho erasure call as best-effort
> - Never automate UserGroup promotion from Points thresholds
> - Never label code as AI-generated in commits, PRs, or comments
> - Never accept auth via cookie for mutation endpoints — Bearer token only
> - Never accept a file upload without server-side MIME inspection + ClamAV scan
> - Enable `pg_stat_statements`, `pgvector`, `uuid-ossp`, `pg_trgm` in the FIRST migration
> - Wire `WriterPool`/`ReaderPool` from day one (both point to same DSN until replica)

---

## Stack Versions (authoritative — AGENTS.md is the single source of truth)

| Component | Version | Notes |
|---|---|---|
| Go | **1.26.x** (1.26.4 at time of writing) | Chi router, sqlc, pgx/v5, Asynq |
| Bun | **1.3.x** (1.3.14 at time of writing) | Runtime for all three SvelteKit apps |
| SvelteKit | **2.57.x** | TypeScript 6.0 support, remote functions |
| PostgreSQL | **18.x** (18.4 at time of writing) | pgvector extension. PG19 is in beta as of mid-2026 — do not use for production until GA. |
| Valkey (Redis Protocol cache/queue layer) | **Valkey 9.x** | |
| Meilisearch | **1.45.x** | |
| Nginx | latest stable from distro/official repo | |
| imgproxy | latest stable | |
| SeaweedFS | 4.x (weed mini for dev) — Apache 2.0 | |
| Tailwind CSS | v4 | CSS custom properties for theming tokens |
| Bits UI | latest stable | Headless, accessible Svelte component primitives (Dialog, Dropdown, Tooltip, Select, etc.) |
| Lucide Svelte | latest stable |
| AutoAnimate | @formkit/auto-animate latest |
| Docker Compose | v2.x (Compose Specification) | |


---

## Phase 1 — Core Foundation

**Sections: 01, 02, 03, 04, 37, 42**
Build this first — every other system depends on it.

### 01. Project Scaffold

```bash
# Go monolith
go mod init github.com/jedafe/japa
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5
go get github.com/sqlc-dev/sqlc
go get github.com/hibiken/asynq
go get github.com/meilisearch/meilisearch-go
go get github.com/oschwald/geoip2-golang
go get github.com/golang-jwt/jwt/v5
go get golang.org/x/crypto  # bcrypt

# Three SvelteKit apps + docs + learn
bun create svelte@latest apps/main    # adapter-node (SSR)
bun create svelte@latest apps/admin   # adapter-static (SPA)
bun create svelte@latest apps/control # adapter-static (SPA)
bun create svelte@latest apps/learn   # adapter-node (SSR)
bun create svelte@latest apps/docs    # adapter-node (SSR)

# First migration — enable all extensions
-- 00001_initial_setup.sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgvector";
CREATE EXTENSION IF NOT EXISTS "pg_stat_statements";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
```

### 02. Multi-Tenancy & DB Pool

```go
// internal/db/pool.go
type DB struct {
  Writer *pgxpool.Pool
  Reader *pgxpool.Pool // = Writer until read replica
}

func NewDB(ctx context.Context) (*DB, error) {
  cfg, _ := pgxpool.ParseConfig(os.Getenv("JC_DATABASE_URL"))
  cfg.MaxConns = 25
  cfg.MinConns = 5
  cfg.MaxConnLifetime = 30 * time.Minute
  cfg.MaxConnIdleTime = 5 * time.Minute
  pool, _ := pgxpool.NewWithConfig(ctx, cfg)
  return &DB{Writer: pool, Reader: pool}, nil
}

// All scoped tables have tenant_id — every query filters by it
// Tenant resolved from subdomain or host header in middleware
```

### 03. Authentication (Section 02)

```go
// JWT + refresh token pattern
type JWTClaims struct {
  jwt.RegisteredClaims
  UserID   string `json:"uid"`
  TenantID string `json:"tid"`
  Kid      string `json:"kid"` // key rotation — never mass-logout on secret rotation
}

func MintAccessToken(userID, tenantID uuid.UUID) (string, error) {
  kid := getCurrentKeyID()   // e.g. "key-2024-07"
  secret := getActiveSecret(kid)
  claims := JWTClaims{
    RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute))},
    UserID: userID.String(), TenantID: tenantID.String(), Kid: kid,
  }
  return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

// Refresh token in SameSite=Strict HttpOnly cookie — never in response body
// NEVER accept auth via cookie for mutation endpoints
// Bearer token in Authorization header only for mutations

// bcrypt cost 12 — explicit, not default
func HashPassword(password string) (string, error) {
  bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
  return string(bytes), err
}

// Idempotency keys for payment endpoints and destructive mutations
// jc:idempotency:{user_id}:{key} → {status_code, response_body}, TTL 24h
```

### 04. RBAC (Section 03)

```go
// Three system defaults: super_admin, members, guest
// All other groups are admin-created — no hardcoded role checks anywhere
func HasPermission(ctx context.Context, u *User, permKey string) bool {
  for _, g := range u.Groups {
    for _, p := range g.Permissions {
      if p.Key == permKey { return true }
    }
  }
  return false
}

// Permission nodes follow: {domain}.{action}
// Examples: blog.create, content.page_builder.create, ai.context.points_balance
// ai.context.* nodes: ALL default to off, including super_admin — manual grant only
// NEVER automate UserGroup promotion from Points thresholds
```

### 05. Feature Flags Middleware (Section 42)

```go
// Apply to every feature-gated route
func FeatureGate(key string) func(http.Handler) http.Handler {
  return func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
      if !IsFeatureEnabled(r.Context(), TenantFromCtx(r.Context()), key) {
        json.NewEncoder(w).Encode(map[string]string{"error": "feature_disabled"})
        w.WriteHeader(http.StatusNotFound)
        return
      }
      next.ServeHTTP(w, r)
    })
  }
}

// SvelteKit guard
// if (!$page.data.features['feature.blog']) throw redirect(302, '/');
```

### 06. IP Geolocation Middleware (Section 37)

```go
// Must be wired from day one — retrofitting is expensive
db, _ := geoip2.Open(os.Getenv("JC_GEOIP_DB_PATH"))
r.Use(GeoIPMiddleware(db))
// Updates weekly via maintenance:update_geoip_db Asynq task
```

---

## Phase 2 — Content Foundation

**Sections: 05, 06, 14, 15, 16, 45**

### 07. Universal Custom Field Architecture (Section 06)

```sql
-- Sequence: custom_field_groups → custom_fields → custom_field_group_assignments
-- ALL field values stored in custom_field_values + denormalized into entity's field_values JSONB
-- RBAC per field: custom_field_permissions (field_id, user_group_id, can_view, can_edit)
-- Platform embed type resolves YouTube/GitHub/Twitter/Vimeo/Spotify/Figma/CodePen/Maps
```

### 08. Geo-First Space System (Section 05)

```go
// Five permission axes: visibility, posting, replying, commenting, joining
// Seven presets seeded (open_community, geo_space, curated, private, read_only, verified_only, admin_only)
// Post bubbling: posts flow up max 2 hierarchy levels (configurable: spaces.bubble_depth)
// Space auto-population: space:auto_populate Asynq task (Wikipedia + flagcdn.com + Pexels)
// Space unread counter: jc:space_unread:{user_id}:{space_id} in Valkey, reset on visit

// NEW (session 5):
// spaces.creator_user_id — tracks who created each custom space (for quota enforcement)
// space_parent_spaces(space_id, parent_space_id) — multi-parent junction table (replaces single parent_id)
//   Custom spaces must belong to ≥1 primary space (country/state/city); enforced at handler
// space_members(space_id, user_id, joined_at, invited_by) — explicit join tracking (separate from follows)
//   Location selection in onboarding auto-joins all matching hierarchy levels
// space_moderators(space_id, user_id, assigned_by, assigned_by_user_id)
//   assigned_by: 'site_admin' (no membership required) | 'space_owner' (must be member)
//   Moderators can be bot users (users.is_bot = true)
// user_groups.max_spaces_creatable: quota per UserGroup (0=none, -1=unlimited, N=capped)
//   Permission node: spaces.create — gated + quota-checked at handler
// GET /api/v1/spaces/suggest-parents?q=... — Meilisearch, level IN [country,state,city]
// GET /api/v1/spaces/search?q=...&filter=... — powers <SpaceCombobox> across all forms
// content_spaces(content_id, space_id, is_primary) — multi-space post assignment (Section 14)
```

### 09. Content Type System (Section 14)

```go
// content_scores table: vote_score, comment_count, view_count, bookmark_count,
//                       velocity_score, relevance_embedding vector(1536)
// read_time_minutes: computed at save time, ceil(word_count / 200)
// content_hash: SHA-256 of normalised body, UNIQUE per tenant (spam prevention)
// slug_history: written on slug change, 301 redirect handler reads from it
// canonical_url: emitted as <link rel="canonical">, displayed in /create sidebar
// pagination_style: infinite_scroll | load_more | pagination (admin-configurable, space override)
```

### 10. Block Editor — TipTap (Section 15)

```ts
// Full block catalog (V1):
// Text: Paragraph, Heading(H1-H6), Blockquote, CodeBlock, Divider, Callout, Details
// Media: Image, Video, YouTubeEmbed, GIF, FileAttachment
// Layout: Columns, TableOfContents, ComparisonEmbed
// Interactive: FAQAccordion, Steps, CountdownWidget, StatsCounter, Poll(forum only)
// Content: RelatedPosts, NewsletterCTA, SocialShare, FeedbackButton
// Advanced: AIGenerate(HITL), Embed(iFrame+SSRF)

// Trigger mapping (locked): / slash, @ mention, # tag, [[ interlink
// Table block deliberately excluded — Section 30 Comparison System covers tabular need
// Mention edge cases: DM privacy bypass, block suppression, erasure fallback, private Space gate

// Block access control: editor_block_permissions (block_type, user_group_id, enabled)
// Admin configures in Settings → Content → Editor Blocks
```

### 11. Post Create Page (Section 45)

```
Route: /create → type selector → /create/[content_type] → full-page Notion-style editor
Right sidebar: Tags, Featured image, Space assignment, Schedule, Custom fields, Visibility, SEO, Canonical URL
Autosave: PATCH every 30s to draft revision
Edit lock: jc:editor_lock:{content_type}:{content_id} Valkey key, TTL 120s
```

---

## Phase 3 — Social & Real-Time

**Sections: 10, 31, 32, 34, 35, 48**

### 12. Follow System (Section 31)

```sql
CREATE TABLE follows (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id   uuid NOT NULL REFERENCES tenants(id),
  follower_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  entity_type text NOT NULL,  -- 'user'|'space'|'tag'|'organization'|'professional_profile'
  entity_id   uuid NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE(follower_id, entity_type, entity_id)
);
-- Joining a space auto-follows it; following does not require joining
-- Mutual follow (A→B + B→A) = connection — no separate connections table
-- follower_count on users/spaces/tags/organizations maintained via DB triggers
```

### 13. Activity Feed (Section 32)

```go
// Timelines: explore (guests/logged-out → Explore page), primary (logged-in → Home/default),
//            space (Space feed), profile (User Timeline)
// Homepage routing: authenticated → primary; unauthenticated → explore (server-rendered, no redirect)
// Four sort modes: latest (keyset cursor), top (7-day vote_score), rising (HN formula), relevant (pgvector)
// Rising: velocity = engagement / (hours_old + 2)^1.8   — gravity constant in system_settings
// Sort bar: sticky on scroll (position: sticky, clears nav header); active mode in per-tab session state
// Multi-space fan-out (session 5): one post → fan-out fires per content_spaces row (one per assigned space)
//   content:fan_out_author_followers — author's followers
//   content:fan_out_space           — per-space push (≤5000 members) or pull_index
//   content:fan_out_bubble          — bubble up to parent primary spaces with 'from [Space]' label
// feeds(user_id, content_id, space_id, content_type, score, created_at) — UNIQUE(user_id, content_id)
// content_scores: maintained by Asynq tasks (maintenance:update_velocity_scores every 10 min)
// Onboarding bootstrap: wizard forces minimum follows before completion (Section 09)
```

### 14. Real-Time WebSockets (Section 10)

```go
// Seven event types: notification, presence, typing, space_activity, feed_update, dm_message, moderation_alert
// Auth: short-lived WS ticket (POST /auth/ws-ticket → single-use, 60s TTL in Valkey)
// Reconnection: exponential backoff client-side; missed events caught up via REST fetch
// Direct Messaging: dm_conversations + dm_messages, message requests for non-mutual-follow senders
// DM privacy: everyone | followers_only | nobody — mentions bypass DM privacy setting
```

### 15. Media System (Section 34)

```go
// Two-step upload: pre-sign → client uploads direct to SeaweedFS → confirm (server moves temp/ → media/)
// imgproxy HMAC-SHA256 signed URL: ALWAYS generated server-side, NEVER cached in DB
// Video uploads: RBAC-gated (media.video.upload), ffmpeg processing via video_processing Asynq task
// ClamAV scan via media_scan task — BLOCKS persistence until clean (not best-effort)
// Media Library: /dashboard/media — browsable grid, searchable, per-user scoped (admin sees all)
// Modal Picker: shared Svelte store/portal, opens in TipTap and Page Builder without navigation
// CDN: admin-configurable via system_settings media.cdn.base_url
// S3-compatible: SeaweedFS default, configurable to R2/Backblaze/AWS in Admin Settings → Media
```

### 16. Notification System (Section 35)

```go
// notifications(id, tenant_id, recipient_id, notification_type, actor_type, actor_id,
//              subject_type, subject_id, title, body, action_url, read_at, created_at)
// notification_batch_windows: 5-minute dedup window per (recipient, type, subject)
// "Alice and 9 others reacted" — not 10 separate notifications
// Delivery: in_app (WS notification event) + optional email + push per user preference
// Admin Panel: add/remove notification_types without code deploy
```

### 17. Direct Messages (Section 48)

```go
// feature.dm flag (Default: true) — full DM UI hidden + routes 404 when disabled
// dm_conversations(participant_a, participant_b) — CHECK(a < b) + UNIQUE prevents duplicate rows
// dm_messages(conversation_id, sender_id, body jsonb, media_id, edited_at, deleted_at)
// dm_requests(requester_id, recipient_id, preview_message, status) — pending approval flow
// users.dm_privacy: 'everyone'|'followers_only'|'nobody' — gates who can open new conversations
// Mutual followers bypass dm_requests and go direct; mentions fire regardless of dm_privacy
// Unread: jc:dm_total_unread:{user_id} in Valkey + unread_a/unread_b columns in DB
// WS: channel dm:{conversation_id}; events dm_message + typing (existing event type, scoped to conv)
// Routes: GET/POST /dm/conversations, GET/POST /dm/conversations/:id/messages
//         POST /dm/conversations/:id/read, GET/POST /dm/requests, POST /dm/requests/:id/accept|decline|block
// Edit window: 15 minutes after send; soft delete (body nulled)
// Frontend: /messages list + /messages/:id thread (sidebar on desktop, full-page on mobile)
```

---

## Phase 4 — Forum, Blog & Content Extensions

**Sections: 08, 24, 29, 30, 33**

### 17. Forum Q&A / Solved (Section 08)

```go
// Nested threads (parent/child replies), Solved status (QAPage structured data)
// Tags: first-class entities with moderators (can be AI agents), follower_count, wiki_content
// Polls: attached to thread, live vote count via WS space_activity/feed_update events
// Anonymous posting: identity withheld from users, always attributable to admin/moderators
// Upvote/downvote: one vote per user per thread/reply (changes update existing row)
// Bookmarks: bookmark_count on all content (polymorphic entity_type/entity_id)
// Reply view counts: admin-switchable, maintenance:archive_reply_views task on toggle
// Edit window: configurable (system_settings: forum.edit_window_minutes)
```

### 18. Blog Extension (Section 24)

```go
// Tags only (no categories). Workflow: draft → in_review → scheduled → published
// Blog Series: blog_series + blog_series_posts tables, /blog/series/[slug] route
// RSS feed: static XML at /blog/feed.xml, regenerated by rss_regenerate task on publish
// Granular RBAC: blog.create | blog.edit_own | blog.edit_any | blog.publish | blog.feature
// Universal RSS: also /feed.xml (sitewide), /spaces/:slug/feed.xml, /tags/:slug/feed.xml, /users/:username/feed.xml
// Featured posts: manually curated (is_featured, featured_order, optional featured_until auto-expiry)
```

### 19. Ratings & Reviews (Section 29)

```sql
-- reviews(entity_type, entity_id, reviewer_user_id, rating 1-5, title, body, status)
-- UNIQUE(entity_type, entity_id, reviewer_user_id) — one review per user per entity
-- review_aggregates(entity_type, entity_id, review_count, average_rating) — maintained by Asynq, never live AVG()
-- Moderation: flagged review → existing Reporting & Appeals queue in Section 22
-- JSON-LD source: Review/AggregateRating in Section 26
```

### 20. Comparison System (Section 30)

```sql
-- comparison_type_definitions: admin-CRUD registry of named comparison schemas
-- Each type has a custom_field_group (dimensions = fields in that group)
-- comparison_type_access: per-UserGroup + AI agent fill permissions
-- comparison_items, comparison_dimensions, comparison_values: existing schema
-- Route: /compare/[entity-type]/[slug-or-id-list]
-- AI-generatable via HITL pipeline (REQUIRED for immigration/financial comparison content)
-- Pre-seeded types: universities, visa_types, banks, courses
```

### 21. Professional Profiles (Section 33)

```sql
-- professional_profile_types: Organization, Agency, Consultancy, Brand, Practice
-- professional_profiles: owner_user_id, type_id, name, slug, verified, follower_count
-- professional_profile_members: profile_id, user_id, role (owner/admin/editor/member)
-- professional_profile_invitations: token-based invite flow
-- Authorship switching: contents has both author_user_id AND author_profile_id
-- Followable via Section 31 follow system (entity_type = 'professional_profile')
```

---

## Phase 5 — Commerce & AI

**Sections: 17, 20, 25, 28, 36**

### 22. Payment Core (Section 17)

```go
// Five providers: paystack | stripe | nowpayments | manual_transfer | credit
// Credit is NOT a Provider — direct synchronous ledger debit (no external call)
// purpose_types: pro_membership | org_boost | donation | course_access | credit_purchase
// Recurring billing: Stripe/Paystack ONLY — never NOWPayments, manual_transfer, or Credit
// Webhook-first: business effect applied in Asynq task (payment_webhook_process, critical tier)
//               never synchronously in the HTTP handler
// Idempotency: payment_webhook_events table with UNIQUE(provider, event_id)
// Discount codes: discount_codes + discount_code_uses (percent/fixed, group-scoped, date-ranged)
// Internal Discount Codes are DISTINCT from affiliate coupons — different tables, different purpose
```

### 23. Wallet & Credits (Section 28)

```go
// Two-tier: Points (activity-earned, Karma/reputation signal) and Credits (spendable)
// ONE immutable wallet_ledger — NOT two systems
// Points = Karma: displayed on profile, feeds onboarding reputation-fallback tier
// NO automatic UserGroup promotion from Points thresholds — EVER
// Activity Point Rules: admin-configurable registry (activity_key → signed point_value, cap, enabled)
// Every feature module emits typed ActivityEvent with idempotency key (reference)
// Consumer resolves rule → writes ledger entry (wallet:process_activity_event, default tier)
// Credit spend: direct ledger debit in same DB transaction as payments row — no Provider interface

func PayWithCredit(ctx context.Context, tx pgx.Tx, userID uuid.UUID, amount int64, purposeType, purposeID string) error {
  balance, err := getCurrentCreditBalance(ctx, tx, userID)
  if err != nil || balance < amount { return ErrInsufficientCredit }
  if err := insertLedgerEntry(ctx, tx, userID, "credits", "spend", -amount, balance-amount, purposeType, purposeID); err != nil { return err }
  return insertPaymentRow(ctx, tx, userID, "credit", purposeType, purposeID, amount, "succeeded")
}
```

### 24. AI Core (Section 20)

```go
// AI Gateway: Anthropic | OpenRouter | OpenAI | Gemini | Ollama
// Circuit-breaker fallback: if primary provider fails, try next in category order
// Tool Registry: RBAC-gated per UserGroup (ai.context.* nodes ALL default to off)
// Honcho: self-hosted, own 'honcho' DB on same Postgres 18.x instance
// HITL MANDATORY for: immigration/financial AI content, comparison auto-fill, doc page generation
// knowledge_articles table: seeded by content team, Meilisearch-indexed for JapaBot RAG
// Erasure: Honcho deletion API called as REQUIRED step in erasure job (not best-effort)
```

### 25. Learning Platform (Section 36)

```go
// learn.japaconnect.com — fourth SvelteKit SSR app
// learning_tracks → learning_modules → learning_lessons (article|video|quiz|checklist)
// Dual access: Pro subscription = all tracks | course_access payment = individual track
// Certificate: learning:issue_certificate Asynq task generates PDF, stored in SeaweedFS
// Quiz: lesson_quizzes table, quiz_passed flag on lesson_progress
```

---

## Phase 6 — Operations & Safety

**Sections: 11, 19, 21, 22, 38, 40, 41, 42, 43**

### 26. Job Queue (Section 11)

```go
// Three tiers: critical | default | low + dead-letter queue
// Critical: payment_webhook_process, notification:dispatch_batch, ticket:send_confirmation
// Default: email_send, search_index_sync, image_processing, content:fan_out_to_feeds,
//          ai:generate_content_embedding, review_aggregate_recalc, wallet:process_activity_event
// Low: sitemap_generate, rss_regenerate, data_export, maintenance:update_velocity_scores,
//      analytics:refresh_views, security:scan, maintenance:update_geoip_db, space:auto_populate
// Monitoring: Asynqmon inside Control Panel behind super_admin
// Alerting: cron health check independent of queue (avoids alert system failing with queue)
```

### 27. Ticket System (Section 38)

```go
// ticket_types: admin-configurable, field_group_id from Section 06
// tickets: status machine (open→in_progress→waiting_on_user→resolved→closed|spam)
// Auto-assignment: least-loaded staff member in assigned UserGroup
// SLA: sla_breach_at computed at create time, maintenance:check_sla_breaches every 15 min
// Internal notes: ticket_comments.is_internal=true (hidden from submitter)
// ticket_events: full audit trail of status/priority/assignment changes
// Portal: /support (type selector → form → ticket thread)
// Admin: full queue, bulk status updates, filters, paginated (NO infinite scroll in admin)
```

### 28. Spam & Content Safety (Section 41)

```go
// Ten layers: Turnstile → honeypot → new-user link restriction → duplicate hash →
//             IP blocking → user blocking → censored words → NSFW detection →
//             AI moderation scan → manual moderation queue
// ip_blocks: CIDR with inet_ops GiST index for fast lookup
// user_blocks: mutual visibility suppression at query level (ExcludeBlockedUsers helper)
// censored_words: cached in Valkey jc:censored_words:{tenant_id}, TTL 1h
// content_hash: UNIQUE index on contents(tenant_id, content_hash) — prevents exact duplicates
```

### 29. Feature Flags (Section 42)

```go
// feature_flags(key, tenant_id, enabled, config) — seeded registry of all non-core features
// When a feature is disabled: API returns 404 (not 403), nav items hidden, no null-pointer errors
// Admin Settings: 13 categories (general, appearance, features, users, content, email,
//                 payment, media, seo, security, integrations, ai, advanced)
// Each category has its own dedicated page in Admin Panel — never a flat wall of settings
```

### 30. Bot & Crawler Management (Section 43)

```go
// known_bots: pre-seeded registry (Googlebot, GPTBot, ClaudeBot, etc.), admin-editable policy
// robots.txt: dynamically generated from bot registry, cached Valkey TTL 24h
// llms.txt: /llms.txt (brief) + /llms-full.txt (detailed), regenerated hourly
// Sitemap quality gates: IsSpaceIndexable() + IsContentIndexable() per Section 43
// UserGroup-level indexing gate: admin can exclude a UserGroup's content from sitemap + Meilisearch
```

### 31. Platform Analytics (Section 40)

```go
// Two layers: Umami (self-hosted, page views) + Platform Native (platform_events table)
// platform_events: event_type, user_id, entity_type, entity_id, meta, country_code, created_at
// Materialized views refreshed hourly by analytics:refresh_views task
// User analytics dashboard: /dashboard/analytics (post views, reactions, followers, ad performance, Credits earned)
// Security Scanner: security:scan task (daily 03:00 UTC), findings in security_scan_results
// Cache flush: Control Panel → Tools → Cache (flushes Valkey jc: namespace, Cloudflare tags)
// Site reports: all exportable as CSV/JSON via data_export Asynq task
```

---

## Phase 7 — Frontend Polish

**Sections: 09, 23, 26, 39, 44, 47**

### 32. Main Site Frontend (Section 23)

```svelte
<!-- UI stack: Tailwind CSS v4 + Bits UI (Dialog, Dropdown, Tooltip, Select, Combobox) + Lucide Svelte -->
<!-- Dark Mode: system-default, Light/Dark/System toggle, server-side preference (follows across devices) -->
<!-- Timezone: UTC storage, user-configured tz, browser-detected default, dual-tz tooltip for events -->
<!-- Discovery pages: /spaces, /tags, /members, /organisations — filterable, sortable, Follow button on each item -->
<!-- Pagination: admin-configurable per content type (infinite_scroll | load_more | pagination) -->
<!-- Universal RSS feeds: /feed.xml, /blog/feed.xml, /spaces/:slug/feed.xml, /tags/:slug/feed.xml -->
<!-- PWA: @vite-pwa/sveltekit, service worker, offline reading -->
<!-- Product tour (Driver.js): triggers after onboarding wizard, dismissible, replay in profile settings -->
<!-- Post create: /create (Section 45) — full-page Notion-style editor -->
```

### 33. University System (Section 39)

```go
// Universities are a space_type with university_fields custom field group
// Seeded by admin via CSV bulk import or manual CRUD in Admin Panel → Content → Universities
// Discovery page: /universities — filterable by country, ranking, tuition, programs, scholarship
// Single university page: tabs (Overview, Posts, Forums, Courses, Members, Reviews, Compare)
// Comparison integration: Compare button pre-selects university as Item 1 in Section 30
// JSON-LD: EducationalOrganization + AggregateRating from Section 29 reviews
```

### 34. SEO & Performance (Section 26)

```go
// Generic metadata resolver: OG tags, noindex on private, canonical URL from contents.canonical_url
// Structured data: Article, QAPage (solved threads), Review/AggregateRating, Organization, Course
// Dynamic OG images: /api/v1/og-image, golang.org/x/image, cached in Valkey TTL 24h
// Sitemap: chunked (50k/file), quality-gated, includes image sitemaps
// hreflang: en (root) + fr (/fr/) — emitted on every public page
// Cloudflare: Cache-Tag purge on publish, if configured
// Lighthouse CI gate in GitHub Actions
// llms.txt / llms-full.txt at root, per llmstxt.org spec
```

### 35. Documentation System (Section 44)

```go
// docs.japaconnect.com — fifth SvelteKit SSR app
// doc_pages: audience-gated content (user/premium/org/moderator/developer/admin/superadmin)
// AI-generated with HITL review — same pipeline as all AI content
// API Playground at /docs/api/playground — live API execution with user's API key
// Deployed same Docker Compose setup, Nginx virtual host
```

### 36. Frontend Animation & Instant Navigation (Section 23 — ADR-051)

```svelte
<!-- AutoAnimate (@formkit/auto-animate, 2.5KB) — applied via `use:autoAnimate` on all reactive lists -->
<!-- Feed cards, notifications, comments, search results, space member lists — animate in/out/reorder -->
<!-- Svelte built-in transitions (svelte/transition, svelte/animate) — modals, drawers, toasts, skeletons -->
<!-- Motion (motion.dev) — per-component micro-interactions, tree-shaken, not in base bundle -->
<!-- Reduced motion: prefersReducedMotion store gates all durations to 0 globally -->

<!-- Instant navigation — SvelteKit native, no external library: -->
<!-- data-sveltekit-preload-data="hover"  → load() fires on hover, click is already done by click time -->
<!-- data-sveltekit-preload-code="viewport" → JS chunks preloaded as links enter viewport -->
<!-- View Transitions API via onNavigate() → 120ms cross-fade; named view-transition-name for hero morphs -->
<!-- Graceful fallback: startViewTransition not supported → instant DOM swap, no error -->
```

### 37. Public API & Developer Portal (Section 47)

```go
// api.japaconnect.com — sixth Nginx virtual host; same Go service, dedicated sub-router
// oapi-codegen: spec-first; api/openapi/api.yaml → go generate → api/openapi/generated/api.gen.go
// Middleware stack: APIKeyMiddleware → RateLimitMiddleware (Valkey) → ScopeMiddleware → OapiRequestValidator
// api_keys table: key_prefix (visible), key_hash (SHA-256), scopes (text[]), rate_limit_tier, status
// Key format: jc_live_<32hex> | jc_test_<32hex>; full key shown once, never stored plain
// Rate tiers: basic=100/min, standard=1000/min, premium=10000/min, unlimited (Valkey sliding window)
// Rate-limit key: jc:api_rate:{key_prefix}:{minute_bucket} — INCR+EXPIRE 70s
// Response headers: X-RateLimit-Limit, X-RateLimit-Remaining, Retry-After (on 429)
// Swagger UI: go:embed openapi/swagger-ui → served at api.japaconnect.com/docs; no CDN dependency
// CSP for /docs: 'self' + 'unsafe-inline'; production API routes: strict CSP
// Developer self-service: /dashboard/developer in main site (max keys = system_settings api.keys.max_per_user)
// Admin Control Panel: /control/api-keys — view/suspend/revoke/tier-change/create; rate tier hot-reload
// Permission nodes: api.keys.view.all | api.keys.manage | api.keys.analytics.view
// Error format: { "error": { "code": "...", "message": "...", "status": N, "docs": "..." } }
// V1 scope family: api.read.* (default); api.write.* (admin-approved per key)
```

---

## Phase 8 — V2 Groundwork (Deferred But Schema-Ready in V1)

**The following V2 features depend on V1 schema already being in place:**

```sql
-- Newsletter (V2): newsletter_subscribers, newsletter_campaigns, newsletter_lists
-- Referral (V2): referrals(code, referrer_id, referee_id, status, qualified_at), 30-day first-click attribution
-- Mentorship (V2): mentorship_profiles(bio, skills, goals, embedding vector(1536)), mentor_sessions, mentor_reviews
-- Japa Ads self-serve (V2): ads_campaigns(advertiser_id, budget, placement, status, payment via credit or card)
-- Content Extensions (V2): event, job_listing, service_listing content types + behavior packs
-- Booking System (V2): bookings, booking_slots, deposit_payments (on ServiceDirectory)
-- Travel Comparison (V3): external_data_sources table — add in V1 migrations, feature deferred
```

---

## Cross-Cutting Patterns

### Webhook Delivery (Section 12)

```go
type WebhookEnvelope struct {
  EventID       string          `json:"event_id"`       // UUID
  EventType     string          `json:"event_type"`     // e.g. "content.published"
  OccurredAt    time.Time       `json:"occurred_at"`
  SchemaVersion string          `json:"schema_version"` // "v1"
  Data          json.RawMessage `json:"data"`
}
// HMAC-SHA256 signed, exponential backoff retry (5 attempts), versioned envelope
// All webhook event types registered in system — admin can configure endpoints per event type
```

### Search (Section 13)

```go
// Indexes: posts, forum_threads, spaces, users, tags, knowledge_articles (JapaBot RAG)
// Grouped results: multi-index search returns results grouped by type (not cross-index merge)
// Geo-aware: Space search filters by geo_code
// Meilisearch backup: NOT needed separately — fully rebuildable from Postgres (in restore runbook)
```

### GDPR/NDPR Erasure (Section 19)

```go
func EraseUserAccount(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
  // 1. Anonymize the user row (name, email, phone, avatar, bio → null/placeholder)
  if err := queries.AnonymizeUser(ctx, tx, userID); err != nil { return err }
  // 2. REQUIRED: delete Honcho AI context — NOT best-effort, must succeed or retry
  if err := honchoClient.DeleteUser(ctx, userID.String()); err != nil { return err }
  // 3. Soft-delete related data per retention policy
  // Payment records: kept (legal obligation), PII disassociated
  return nil
}
```

### GlitchTip Error Tracking (Section 27)

```go
// Self-hosted GlitchTip, Sentry SDK-compatible
// Go: sentry.Init({Dsn: JC_GLITCHTIP_DSN, Release: JC_RELEASE_SHA})
// SvelteKit: Sentry.init({dsn: JC_GLITCHTIP_DSN}) in hooks.client.ts
// GlitchTip DB: separate logical database 'glitchtip' on same Postgres 18.x instance
```

### 46. Admin Impersonation (Section 46)

```go
// Permission node: admin.impersonate — not in any default role including super_admin; manually granted
// Valkey key: jc:impersonate:{admin_id} → {target_user_id}, TTL 30min (non-renewable)
// API: POST /api/v1/admin/impersonate, DELETE, GET /status
// Middleware injects impersonated identity into context; preserves real admin ID for audit
// audit_logs.impersonated_user_id — non-null only during impersonation; actor_user_id = real admin
// No admin-on-admin impersonation; no 2FA inheritance; no TTL renewal
// UI: yellow sticky banner "Viewing as [username]" with countdown + End Session button
// RULES.md: impersonation sessions MUST be logged — no silent impersonation
```

---

## Docker Compose Services

```yaml
services:
  postgres:    # PostgreSQL 18.x, databases: japa, honcho, glitchtip
  valkey:      # Valkey 9.x
  meilisearch: # Meilisearch 1.45.x
  seaweedfs:   # SeaweedFS (weed mini — all-in-one), S3 port 8333, admin UI port 23646
  imgproxy:    # Internal only, port 8099 (avoids conflict with go-api:8080)
  honcho:      # Plastic Labs, port 8001 (internal; avoids conflict with glitchtip:8000), uses 'honcho' DB
  glitchtip:   # Port 8000 (internal), uses 'glitchtip' DB
  go-api:      # Port 8080
  main:        # SvelteKit SSR, port 3000
  admin:       # SvelteKit SPA, port 3001
  control:     # SvelteKit SPA, port 3002
  learn:       # SvelteKit SSR, port 3003
  docs:        # SvelteKit SSR, port 3004
  nginx:       # Reverse proxy + SSL termination
  mailpit:     # Local dev only
```

## Environment Variables (partial — see full list in .env.example)

```bash
JC_DATABASE_URL=postgresql://...
JC_REDIS_ADDR=valkey:6379          # Valkey address
JC_MEILISEARCH_URL=http://meilisearch:7700
JC_MEILISEARCH_KEY=...
JC_GEOIP_DB_PATH=/data/GeoLite2-City.mmdb
JC_MAXMIND_LICENSE_KEY=...
JC_GLITCHTIP_DSN=http://...@glitchtip:8000/1
JC_IMGPROXY_KEY=...
JC_IMGPROXY_SALT=...
JC_IMGPROXY_BASE_URL=https://img.japaconnect.com
JC_IMGPROXY_INTERNAL_URL=http://imgproxy:8099  # internal Docker port (avoids conflict with go-api:8080)
JC_HONCHO_URL=http://honcho:8001
JC_RELEASE_SHA=...
JC_ENV=production
JC_S3_ENDPOINT=http://seaweedfs:8333  # SeaweedFS S3-compatible endpoint (weed mini)
JC_S3_ACCESS_KEY=...
JC_S3_SECRET_KEY=...
JC_S3_BUCKET=japaconnect
JC_S3_REGION=us-east-1              # Placeholder; SeaweedFS ignores region
```
