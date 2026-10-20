# Design — Next-Action Suggestion Engine

**Date:** 2026-10-20
**Status:** Draft for review
**JEOS refs:** §09 (onboarding), §10 (realtime/WS), §11 (job queue), §35 (notifications), §40 (platform analytics), §42 (feature flags), §59 (sidebars), §71 (readiness assessment), ADR-064a (geo-aware recommendations), ADR-128 (intent personalization), unified user tiers design (2026-09-03)

## Goal

A dedicated, admin-configured engine that suggests the **next action** a person should
take, based on who they are and what they have (not) done — delivered proactively
through popups, announcement bars, sidebar widgets, in-article cards,
notifications, push, and email across all surfaces (main, tools, academy,
docs, mobile).

Examples it must serve out of the box:

- Newly registered member → "Take the readiness assessment"
- Canada-bound user who hasn't used the CRS calculator → "Calculate your Express Entry score"
- Guest who has viewed 3 pieces of content → "Create a free account"
- Free (`regular`) user who hit a tier gate or has been active 14 days → "Upgrade your account"

This is distinct from `internal/recommendations` (ADR-064a), which answers
"what might interest you" as a pull-based ranked list. Next-actions answer
"what should you do now" with stateful lifecycle (shown → clicked/dismissed/
snoozed/completed) and proactive delivery.

## Decisions from clarification

- **New dedicated module** `internal/nextactions` — the recommendations engine
  stays untouched; suggestions have their own rule model, state store, and
  delivery semantics.
- **Admin-configured rules.** Rule instances live in a `next_action_rules`
  table managed from the Control panel. The *predicate vocabulary* is
  code-registered (admins compose conditions from a fixed predicate set; new
  predicate types ship via deploys).
- **All surfaces + mobile.** One OptionalAuth endpoint; each surface renders
  pending suggestions in its own chrome. Mobile polls + receives push.
- **Hybrid evaluation.** Pull-at-request for in-app channels (popup, bar);
  a periodic Asynq sweep materializes `user_next_actions` rows for
  notification/push/email delivery.
- **Guests are first-class.** Anonymous visitors get register-nudges driven by
  `platform_events` session activity; state lives in Valkey, not Postgres.

## Data model

### `next_action_rules`

```sql
CREATE TABLE public.next_action_rules (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug             text NOT NULL UNIQUE,
    name             text NOT NULL,
    description      text NOT NULL DEFAULT '',
    enabled          boolean NOT NULL DEFAULT false,
    priority         integer NOT NULL DEFAULT 100,          -- lower = shown first
    audience         jsonb NOT NULL DEFAULT '{}',           -- see shape below
    conditions       jsonb NOT NULL DEFAULT '{}',           -- predicate DSL
    payload          jsonb NOT NULL,                        -- title/body/cta/icon
    channels         jsonb NOT NULL DEFAULT '[]',           -- popup|announcement_bar|sidebar|in_article|notification|push|email
    completion_signal jsonb,                                -- auto-complete predicate (nullable)
    frequency        jsonb NOT NULL DEFAULT '{}',           -- mode, caps, cooldowns
    valid_from       timestamptz,
    valid_until      timestamptz,
    created_by       uuid REFERENCES users(id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_next_action_rules_enabled
    ON public.next_action_rules (enabled, priority);
```

`audience` shape:

```jsonc
{
  "user_class": ["guest", "regular", "pro", "business"],  // resolved via tiers.Service; "guest" = anonymous
  "surfaces":   ["main", "tools", "academy", "docs", "mobile"], // empty = all
  "archetype":  ["aspiring", "planner"],   // optional VisitorContext filters
  "japa_stage": ["researching", "preparing"],
  "destination_country": ["CA"],
  "origin_country": ["NG"]
}
```

`payload` shape: `{ "title": "…", "body": "…", "cta_label": "…", "cta_url": "…", "icon": "…" }`
(`cta_url` must be internal-relative or an allowlisted surface URL.)

`frequency` shape:

```jsonc
{ "mode": "once" | "recurring",
  "max_impressions": 5,
  "cooldown_hours": 72,
  "snooze_default_hours": 168 }
```

### `user_next_actions` (registered users only)

```sql
CREATE TABLE public.user_next_actions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rule_id       uuid NOT NULL REFERENCES next_action_rules(id) ON DELETE CASCADE,
    status        text NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending','shown','dismissed','snoozed','completed','expired')),
    impressions   integer NOT NULL DEFAULT 0,
    first_shown_at timestamptz,
    last_shown_at  timestamptz,
    snoozed_until  timestamptz,
    dismissed_at   timestamptz,
    completed_at   timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, rule_id)               -- sweep idempotency key
);
```

**Guests get no rows.** Guest state lives in Valkey under
`jc:nextactions:guest:{session_key}` — a hash of `{rule_id: status, impressions,
snoozed_until}` with a 30-day TTL. On registration/login the guest state merges
into `user_next_actions` so dismissed nudges don't resurface.

### Analytics

Impressions, clicks, dismissals, and completions write `platform_events`
rows (`next_action.shown`, `next_action.clicked`, `next_action.dismissed`,
`next_action.completed`) with `meta.rule_slug` — no third table; §40
reporting picks them up automatically.

## Predicate vocabulary

Rule `conditions` are `{ "all": […], "any": […] }` groups over a code-registered
predicate set evaluated by a `UserStateResolver`. V1 vocabulary:

| Predicate | Args | Basis |
|---|---|---|
| `user_class` / `account_tier` | `{in: [...]}` | `tiers.ResolveUserTier`; `guest` when unauthenticated |
| `account_age_days` | `{lt,gt}` | `users.created_at` |
| `onboarding_completed` | `{eq}` | §09 onboarding status |
| `email_verified` | `{eq}` | users row |
| `profile_completeness` | `{lt}` | profile completeness calc |
| `japa_stage`, `archetype` | `{in}` | VisitorContext (works for guests via journey cookie) |
| `destination_country`, `origin_country`, `current_country` | `{in}` | VisitorContext / profile / GeoIP |
| `assessment_completed`, `assessment_score` | `{eq,lt}` | §71 readiness assessment store |
| `tool_used` / `tool_not_used` | `{slug}` (`"*"` = any tool) | `platform_events` `tool.used` / tool page views |
| `feature_used` / `feature_not_used` | `{slug, within_days}` | `platform_events` |
| `event_count` | `{type, gte, window: session\|Nd}` | `platform_events` by `session_id` (guest) or `user_id` |
| `feature_gate_hit` | `{feature, within_days}` | `platform_events` `feature.gate_hit` |
| `session_page_views`, `dwell_seconds` | `{gte}` | session analytics / Valkey counter |
| `has_push_subscription` | `{eq}` | §35 push subscriptions |
| `surface` | `{in}` | request `?surface=` param |

**`feature.gate_hit` does not exist today.** V1 includes emitting it from the
tier-gate enforcement points (`publicapi/tier.go` and the feature-limit
middleware) as a `platform_events` row, so upgrade-nudge rules have a real
trigger. Entity = the gated feature slug.

## API routes

Public (OptionalAuth — same pattern as `GET /recommendations`):

```
GET  /api/v1/next-actions?surface=main&limit=5   — ranked active suggestions (guest + user)
POST /api/v1/next-actions/{id}/dismiss           — body {snooze_hours?}; guests keyed by session
POST /api/v1/next-actions/{id}/complete          — mark action taken
POST /api/v1/next-actions/{id}/track             — {event: shown|clicked}; used by both guests and
                                                   users (explicit tracking keeps metrics uniform)
GET  /api/v1/me/next-actions?surface=…           — auth alias
```

Admin/Control (full CRUD per the DataTable spec):

```
GET|POST            /api/v1/admin/next-actions/rules
GET|PATCH|DELETE    /api/v1/admin/next-actions/rules/{id}
GET                 /api/v1/admin/next-actions/stats   — per-rule shown/click/dismiss/complete funnel
```

Response item shape:

```jsonc
{ "id": "…", "rule_slug": "take-readiness-assessment",
  "title": "…", "body": "…", "cta_label": "…", "cta_url": "…", "icon": "…",
  "channels": ["popup"], "priority": 10, "action_id": "…" /* user_next_actions id; null for guests */ }
```

## Evaluation flow

**Pull path** (`GET /api/v1/next-actions`):

1. Resolve VisitorContext + identity (user_id or guest session key).
2. Load enabled, in-window rules (Valkey-cached rule list).
3. Filter by `audience` (user_class, surface, geo/stage/archetype).
4. Evaluate `conditions` via `UserStateResolver` (gathers account age, tier,
   onboarding/assessment status, event counts — one batched resolver, not
   per-predicate queries).
5. Join state: `user_next_actions` for users, the guest Valkey hash for
   guests. Drop dismissed, snoozed-until-future, completed, expired, and
   impression-capped rules.
6. Evaluate `completion_signal` — if true, mark `completed` and drop.
7. Apply global caps (`nextactions.max_active_per_user`,
   `nextactions.max_upsell_per_day`) → rank by `priority` → slice to `limit`.
8. Cache result `jc:nextactions:user:{id}` / `jc:nextactions:guest:{key}`
   (~5 min TTL, setting-driven). Invalidate on dismiss/complete and on
   profile/stage/tier changes (same hook that invalidates recommendations).

**Sweep path** — Asynq `nextactions:sweep` (default queue, interval from
`nextactions.sweep_interval_minutes`, default 60):

1. Iterate users active in the last N days (setting
   `nextactions.sweep_activity_window_days`, default 7), batched.
2. Run the same engine for rules carrying `notification`, `push`, or `email`
   channels (guest-eligible channels are ignored — sweep is auth-only).
3. For newly eligible rules, `INSERT … ON CONFLICT (user_id, rule_id) DO NOTHING`
   into `user_next_actions`, then dispatch via §35
   `notifications.Service` as type `next_action.suggested` — which delivers
   in-app + push + email per the user's notification preferences and
   broadcasts the WS `notification` event for realtime display.
4. Re-evaluates `completion_signal` so auto-completed rules stop firing.

## Delivery channels & renderers

| Channel | Guests | Users | Renderer |
|---|---|---|---|
| `popup` | ✓ | ✓ | `NextActionPopup.svelte` modal in each app shell; max `nextactions.popup_max_per_session` (default 1) per session |
| `announcement_bar` | ✓ | ✓ | `NextActionBar.svelte` dismissible bar under top nav; one visible, highest priority first |
| `sidebar` | ✓ | ✓ | New `next_action` type in the §59 sidebar widget registry, rendered by `SidebarWidgetRenderer.svelte` — admin-placeable per layout like `suggested_spaces`/`ad_slot` |
| `in_article` | ✓ | ✓ | `NextActionInline.svelte` card injected into article/reader bodies, same injection pattern as `InArticleAd.svelte` (§25) |
| `notification` | — | ✓ | §35 in-app notification entry with CTA link |
| `push` | — | ✓ | §35 web/mobile push via sweep |
| `email` | — | ✓ | §35 email via sweep |

Rule validation rejects `notification|push|email` for rules whose
`audience.user_class` includes `guest`. `sidebar`/`in_article` are pull-side
render channels — guests get them too.

Main app gets popup + bar components mounted in the app shell, the `next_action`
sidebar widget available in widget layouts, and the inline card in article/
guide reader views; tools/academy/docs get the bar (and popup where the shell
supports it); Flutter polls the endpoint on home load + receives push.

## Seed rules (shipped enabled, tunable in Control)

| slug | audience | conditions | channels |
|---|---|---|---|
| `member-take-readiness-assessment` | regular+ | `account_age_days < 7`, `assessment_completed = false` | popup, notification |
| `canada-crs-calculator` | regular+ | `destination_country in [CA]`, `tool_not_used crs-calculator` | announcement_bar |
| `guest-register-after-views` | guest | `event_count content.view >= 3` | popup (max 5 impressions) |
| `guest-register-save-results` | guest | `tool_used "*"` | announcement_bar |
| `free-upgrade-gate-hit` | regular | `feature_gate_hit within 7d` | popup (cooldown 72h) |
| `free-upgrade-mature` | regular | `account_age_days > 14` | announcement_bar (cooldown 168h) |

All upgrade rules carry `completion_signal: {"account_tier": {"in": ["pro","business"]}}`;
all register rules carry `completion_signal: {"user_class": {"in": ["regular","pro","business"]}}`
(moot post-merge for guests, but guards the merge window).

## Governance (per AGENTS.md)

- **Feature flag:** `next_actions` in `features.Registry` — single kill switch.
- **system_settings** (registered in the catalog):
  `nextactions.cache_ttl_seconds` (300),
  `nextactions.sweep_interval_minutes` (60),
  `nextactions.sweep_activity_window_days` (7),
  `nextactions.max_active_per_user` (5),
  `nextactions.max_upsell_per_day` (1),
  `nextactions.popup_max_per_session` (1),
  `nextactions.guest_state_ttl_seconds` (2592000).
- **Valkey keys** (catalog additions): `jc:nextactions:user:{user_id}`,
  `jc:nextactions:guest:{session_key}`, `jc:nextactions:rules`,
  `jc:nextactions:sweep:lock`.
- **Asynq task:** `nextactions:sweep` — added to the §11 catalog before code.
- **RBAC nodes:** `nextactions.manage` (rule CRUD), `nextactions.view`
  (stats read). Seeded to admin groups.
- **Notification type:** `next_action.suggested` seeded into
  `notification_types` (in-app + push + email toggles, all default-on for
  in-app, push/email default-off).
- **JEOS first:** new section file + ADR, plus §11 task, Valkey catalog, and
  api-routes catalog entries land before implementation code.
- **Tenancy:** single-tenant — no `platform_id` anywhere (multi-tenancy was
  removed 2026-09-14 per JEOS.md).

## Error handling & edge cases

- Resolver degrades gracefully: a failing signal source (assessment store
  down, platform_events lag) treats that predicate as false, never 500s the
  endpoint — suggestions are enhancement, not critical path.
- Guest session key rotation: state is best-effort; a lost key just resets
  caps, which the impression ceilings bound anyway.
- Rule edits take effect on next pull (rule-list cache invalidated on admin
  mutation); in-flight `user_next_actions` rows for a disabled rule are
  filtered at read, no migration needed.
- `cta_url` validated server-side: relative paths or allowlisted surface
  origins only — no open redirects.
- GDPR: `user_next_actions` rows cascade-delete with the user; guest state
  expires by TTL; `platform_events` already covered by §19 retention.

## Testing

- Unit: predicate evaluators per type (table-driven), state-machine
  transitions (pending→shown→dismissed/snoozed/completed), frequency caps,
  completion-signal handling, guest↔user merge.
- Integration: `GET /next-actions` for guest (session-keyed), new user, and
  tier-gated regular; sweep materialization + notification dispatch;
  rule CRUD RBAC; cache invalidation on dismiss/profile-change.
- Frontend: popup/bar render, snooze persistence, one-popup-per-session cap.
