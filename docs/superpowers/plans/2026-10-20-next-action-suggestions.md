# Next-Action Suggestion Engine — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `internal/nextactions` — an admin-configured engine that suggests a person's next action (register, take assessment, use a tool, upgrade) based on rules evaluated against user state, delivered via popups, announcement bars, sidebar widgets, in-article cards, notifications, push, and email.

**Architecture:** Pull-path: `GET /api/v1/next-actions` (OptionalAuth) evaluates enabled rules from `next_action_rules` via a code-registered predicate vocabulary + `UserStateResolver`, filters by per-user state (`user_next_actions` for members, Valkey `jc:nextactions:guest:{key}` for guests), ranks, and returns render-ready payloads. Sweep-path: Asynq `nextactions:sweep` materializes `user_next_actions` rows for push-channel rules and dispatches through §35 `notifications.Service`.

**Tech Stack:** Go 1.x + chi + sqlc/pgx + Valkey + Asynq (backend); SvelteKit 2 + Svelte 5 runes + Bits UI + Tailwind v4 + bun (frontends); Flutter/Riverpod (mobile).

## Global Constraints

- **Rule 2:** Never hand-edit `internal/db/gen/` — edit `internal/db/queries/*.sql`, then `make sqlc-gen`.
- **Rule 3:** No hardcoded limits/URLs — every tunable comes from `system_settings` via `settings.GetIntOr`/`GetStringOr`.
- **Rule 4:** All Valkey keys use `jc:` prefix via `valkey.MustKey(pattern, args...)`.
- **Rule 5:** Errors use `errpkg.WriteJSON(w, status, payload)` with `error.code` snake_case.
- **Rule 7/§11:** Task name `nextactions:sweep` must be in the §11 catalog BEFORE code.
- **Rule 16:** All frontend data comes from the API — zero hardcoded suggestions.
- **Rule 17:** No AI attribution in commits, comments, or docs.
- **Rule 18:** `bun`/`bunx`/`bun run` only — never npm/npx/pnpm/yarn.
- Single-tenant: NO `platform_id` columns anywhere.
- New flag: `next_actions.enabled` (registry `{Key, Label, Description, Default, Category}`); gates routes via `features.Gate(featureChecker, "next_actions.enabled")`.
- New RBAC nodes: `nextactions.manage`, `nextactions.view` (seeded to `super_admin`/`super_admin_role`).
- Migration pair: `migrations/000913_next_action_suggestions.{up,down}.sql`.
- Verification: `make vet`, `make lint`, `go test ./internal/nextactions/...`, `bun run check` in each touched app.

---

### Task 1: JEOS spec corpus — section, ADR, catalog entries

Per AGENTS.md, JEOS artifacts land before code. This task creates the spec section, ADR, and all catalog registrations the code will reference.

**Files:**
- Create: `.jeos/sections/122-next-action-suggestions.md`
- Create: `.jeos/adr/ADR-132-next-action-suggestions.md`
- Modify: `.jeos/JEOS.md` (section index table — add row for 122)
- Modify: `.jeos/conventions/asynq-tasks.md` (add `nextactions:sweep` row)
- Modify: `.jeos/conventions/valkey-keys-catalog.md` (add 4 key patterns)
- Modify: `.jeos/conventions/api-routes-catalog.md` (add route block)
- Modify: `.jeos/conventions/system-settings-catalog.md` (add 7 keys)
- Modify: `.jeos/sections/11-job-queue-asynq.md` (add task row to its catalog)

**Interfaces:**
- Produces: catalog names every later task uses verbatim:
  - Task: `nextactions:sweep`, default queue, every 60 min (settings-driven), idempotent.
  - Valkey: `jc:nextactions:user:{user_id}`, `jc:nextactions:guest:{session_key}`, `jc:nextactions:rules`, `jc:nextactions:sweep:lock`.
  - Settings: `nextactions.cache_ttl_seconds` (300), `nextactions.sweep_interval_minutes` (60), `nextactions.sweep_activity_window_days` (7), `nextactions.max_active_per_user` (5), `nextactions.max_upsell_per_day` (1), `nextactions.popup_max_per_session` (1), `nextactions.guest_state_ttl_seconds` (2592000).
  - Routes: `GET /api/v1/next-actions`, `POST /api/v1/next-actions/{id}/dismiss|complete|track`, `GET /api/v1/me/next-actions`, `GET|POST /api/v1/admin/next-actions/rules`, `GET|PATCH|DELETE /api/v1/admin/next-actions/rules/{id}`, `GET /api/v1/admin/next-actions/stats`.
  - Flag: `next_actions.enabled`; RBAC: `nextactions.manage`, `nextactions.view`; notification type: `next_action.suggested`.

- [ ] **Step 1: Write the section file**

Create `.jeos/sections/122-next-action-suggestions.md` with the full spec content — copy the design doc `docs/superpowers/specs/2026-10-20-next-action-suggestions-design.md` sections "Data model", "Predicate vocabulary", "API routes", "Evaluation flow", "Delivery channels & renderers", "Seed rules", "Governance" into JEOS section format (match the structure of `sections/76-participation-orchestration.md`: Overview → tables/JSON shapes → engine loop → Valkey keys → system_settings → Asynq tasks → RBAC → API routes → See Also).

- [ ] **Step 2: Write ADR-132**

`.jeos/adr/ADR-132-next-action-suggestions.md` — decision record: dedicated module vs extending ADR-064a recommendations; admin-configured rules over code-registered predicate vocabulary; hybrid pull+sweep evaluation; guest state in Valkey not Postgres; channels split pull-render vs §35 push. Match existing ADR format (see `ADR-128-intent-experience-personalization.md`).

- [ ] **Step 3: Register catalogs**

- `asynq-tasks.md` + `sections/11-job-queue-asynq.md`: add `| nextactions:sweep | default | Every 60 min (nextactions.sweep_interval_minutes) — materializes user_next_actions rows and dispatches via §35 | Yes (UNIQUE(user_id,rule_id)) |`.
- `valkey-keys-catalog.md`: add the 4 `jc:nextactions:*` patterns with purpose + TTL columns matching existing rows.
- `api-routes-catalog.md`: add all routes under a `### Next Actions (§122)` heading, matching existing formatting.
- `system-settings-catalog.md`: add the 7 `nextactions.*` keys with defaults + descriptions.
- `JEOS.md`: add `| 122 | sections/122-next-action-suggestions.md | Next-Action Suggestion Engine — ... |` index row.

- [ ] **Step 4: Commit**

```bash
git add .jeos/
git commit -m "docs(jeos): §122 next-action suggestion engine — section, ADR-132, catalog entries"
```

---

### Task 2: Migration — tables + seeds

**Files:**
- Create: `migrations/000913_next_action_suggestions.up.sql`
- Create: `migrations/000913_next_action_suggestions.down.sql`

**Interfaces:**
- Produces: `next_action_rules`, `user_next_actions` tables; permission rows `nextactions.manage`/`nextactions.view`; notification type `next_action.suggested`; 6 seed rules (slugs listed below) consumed by Task 3+ code.

- [ ] **Step 1: Write the up migration**

```sql
-- 000913_next_action_suggestions.up.sql — §122 Next-Action Suggestion Engine.
-- Rule catalog + per-user state; seeds RBAC nodes, the next_action.suggested
-- notification type, and the six launch rules.

BEGIN;

CREATE TABLE public.next_action_rules (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug              text NOT NULL UNIQUE,
    name              text NOT NULL,
    description       text NOT NULL DEFAULT '',
    enabled           boolean NOT NULL DEFAULT false,
    priority          integer NOT NULL DEFAULT 100,
    audience          jsonb NOT NULL DEFAULT '{}',
    conditions        jsonb NOT NULL DEFAULT '{}',
    payload           jsonb NOT NULL,
    channels          jsonb NOT NULL DEFAULT '[]',
    completion_signal jsonb,
    frequency         jsonb NOT NULL DEFAULT '{}',
    valid_from        timestamptz,
    valid_until       timestamptz,
    created_by        uuid REFERENCES users(id),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_next_action_rules_enabled ON public.next_action_rules (enabled, priority);

CREATE TABLE public.user_next_actions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rule_id        uuid NOT NULL REFERENCES next_action_rules(id) ON DELETE CASCADE,
    status         text NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending','shown','dismissed','snoozed','completed','expired')),
    impressions    integer NOT NULL DEFAULT 0,
    first_shown_at timestamptz,
    last_shown_at  timestamptz,
    snoozed_until  timestamptz,
    dismissed_at   timestamptz,
    completed_at   timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, rule_id)
);
CREATE INDEX idx_user_next_actions_user ON public.user_next_actions (user_id, status);

-- RBAC nodes (granted to super admins; other roles via RBAC admin UI).
INSERT INTO permissions (key, description) VALUES
    ('nextactions.manage', 'Create, edit, and delete next-action suggestion rules'),
    ('nextactions.view', 'View next-action suggestion rules and stats')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role')
  AND p.key IN ('nextactions.manage', 'nextactions.view')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Notification type (in-app + push on, email off — matches §35 seed style).
INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push, category) VALUES
    ('next_action.suggested', 'Next step suggestion', 'Personalized next-step suggestions based on your journey', true, false, true, 'system')
ON CONFLICT (key) DO NOTHING;

-- Seed rules (§122). payload.cta_url values are internal paths.
INSERT INTO next_action_rules
    (slug, name, description, enabled, priority, audience, conditions, payload, channels, completion_signal, frequency)
VALUES
    ('member-take-readiness-assessment', 'Take the readiness assessment',
     'New members who have not completed the readiness assessment',
     true, 10,
     '{"user_class":["regular","pro","business"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"account_age_days","lt":30},{"predicate":"assessment_completed","eq":false}]}',
     '{"title":"How ready are you to japa?","body":"Take the readiness assessment to get a personalised roadmap.","cta_label":"Take assessment","cta_url":"/tools/readiness-assessment","icon":"clipboard-check"}',
     '["popup","notification"]',
     '{"all":[{"predicate":"assessment_completed","eq":true}]}',
     '{"mode":"once","max_impressions":5,"cooldown_hours":24,"snooze_default_hours":168}'),

    ('canada-crs-calculator', 'Calculate your CRS score',
     'Canada-bound users who have not used the CRS calculator',
     true, 20,
     '{"user_class":["regular","pro","business","guest"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"destination_country","in":["CA"]},{"predicate":"tool_not_used","slug":"crs-calculator"}]}',
     '{"title":"Estimate your Express Entry score","body":"Use the CRS calculator to see where you stand.","cta_label":"Calculate score","cta_url":"/tools/crs-calculator","icon":"calculator"}',
     '["announcement_bar","sidebar","in_article"]',
     '{"all":[{"predicate":"tool_used","slug":"crs-calculator"}]}',
     '{"mode":"once","max_impressions":8,"cooldown_hours":72,"snooze_default_hours":168}'),

    ('guest-register-after-views', 'Create a free account',
     'Guests who have viewed several pieces of content',
     true, 30,
     '{"user_class":["guest"],"surfaces":["main","tools"]}',
     '{"all":[{"predicate":"event_count","type":"content.viewed","gte":3,"window":"7d"}]}',
     '{"title":"Join JapaConnect","body":"Create a free account to save your progress and get personalised guidance.","cta_label":"Sign up free","cta_url":"/register","icon":"user-plus"}',
     '["popup"]',
     NULL,
     '{"mode":"once","max_impressions":5,"cooldown_hours":48,"snooze_default_hours":168}'),

    ('guest-register-save-results', 'Save your results',
     'Guests who just used a tool',
     true, 40,
     '{"user_class":["guest"],"surfaces":["main","tools"]}',
     '{"all":[{"predicate":"tool_used","slug":"*"}]}',
     '{"title":"Keep your results","body":"Register to save your tool results and pick up where you left off.","cta_label":"Create account","cta_url":"/register","icon":"bookmark"}',
     '["announcement_bar","in_article"]',
     NULL,
     '{"mode":"once","max_impressions":4,"cooldown_hours":72,"snooze_default_hours":168}'),

    ('free-upgrade-gate-hit', 'Unlock with Pro',
     'Free users who hit a tier-gated feature in the last 7 days',
     true, 50,
     '{"user_class":["regular"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"feature_gate_hit","within_days":7}]}',
     '{"title":"You hit a Pro feature","body":"Upgrade to unlock higher limits and member-only tools.","cta_label":"See plans","cta_url":"/pricing","icon":"zap"}',
     '["popup"]',
     '{"all":[{"predicate":"account_tier","in":["pro","business"]}]}',
     '{"mode":"recurring","max_impressions":6,"cooldown_hours":72,"snooze_default_hours":168}'),

    ('free-upgrade-mature', 'Upgrade your account',
     'Free users active for more than 14 days',
     true, 60,
     '{"user_class":["regular"],"surfaces":["main","tools","mobile"]}',
     '{"all":[{"predicate":"account_age_days","gt":14}]}',
     '{"title":"Get more from JapaConnect","body":"Pro members unlock unlimited tools and priority features.","cta_label":"View plans","cta_url":"/pricing","icon":"star"}',
     '["announcement_bar","sidebar"]',
     '{"all":[{"predicate":"account_tier","in":["pro","business"]}]}',
     '{"mode":"recurring","max_impressions":8,"cooldown_hours":168,"snooze_default_hours":336}');

COMMIT;
```

- [ ] **Step 2: Write the down migration**

```sql
-- 000913_next_action_suggestions.down.sql
BEGIN;

DELETE FROM role_permissions WHERE permission_id IN
    (SELECT id FROM permissions WHERE key IN ('nextactions.manage','nextactions.view'));
DELETE FROM permissions WHERE key IN ('nextactions.manage','nextactions.view');
DELETE FROM notification_types WHERE key = 'next_action.suggested';
DROP TABLE IF EXISTS public.user_next_actions;
DROP TABLE IF EXISTS public.next_action_rules;

COMMIT;
```

- [ ] **Step 3: Apply and verify**

```bash
make migrate   # or the project's migrate target — check Makefile first
```

Expected: migration 000913 applied; `\d next_action_rules` shows both tables; `SELECT count(*) FROM next_action_rules` = 6.

- [ ] **Step 4: Commit**

```bash
git add migrations/000913_next_action_suggestions.*
git commit -m "feat(db): next_action_rules + user_next_actions, RBAC/notification-type/seed rules (§122)"
```

---

### Task 3: sqlc queries

**Files:**
- Create: `internal/db/queries/next_actions.sql`
- Regenerate: `internal/db/gen/` (via `make sqlc-gen` — never hand-edit, Rule 2)

**Interfaces:**
- Produces (gen function names later tasks call):
  `ListEnabledNextActionRules`, `ListNextActionRules`, `GetNextActionRule`, `CreateNextActionRule`, `UpdateNextActionRule`, `DeleteNextActionRule`, `GetUserNextActions`, `UpsertUserNextAction`, `MarkNextActionShown`, `DismissNextAction`, `SnoozeNextAction`, `CompleteNextAction`, `CountUserEvents`, `CountSessionEvents`, `UserHasAssessmentResult`, `ListActiveUsersForSweep`, `NextActionStats`.

- [ ] **Step 1: Write the query file**

```sql
-- internal/db/queries/next_actions.sql — §122 queries.

-- name: ListEnabledNextActionRules :many
SELECT id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at
FROM next_action_rules
WHERE enabled = true
  AND (valid_from IS NULL OR valid_from <= now())
  AND (valid_until IS NULL OR valid_until > now())
ORDER BY priority ASC;

-- name: ListNextActionRules :many
SELECT id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at
FROM next_action_rules
ORDER BY priority ASC, created_at DESC;

-- name: GetNextActionRule :one
SELECT id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at
FROM next_action_rules
WHERE id = @id;

-- name: CreateNextActionRule :one
INSERT INTO next_action_rules
    (slug, name, description, enabled, priority, audience, conditions, payload,
     channels, completion_signal, frequency, valid_from, valid_until, created_by)
VALUES
    (@slug, @name, @description, @enabled, @priority, @audience, @conditions,
     @payload, @channels, @completion_signal, @frequency, @valid_from,
     @valid_until, @created_by)
RETURNING id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at;

-- name: UpdateNextActionRule :one
UPDATE next_action_rules SET
    name = @name, description = @description, enabled = @enabled,
    priority = @priority, audience = @audience, conditions = @conditions,
    payload = @payload, channels = @channels,
    completion_signal = @completion_signal, frequency = @frequency,
    valid_from = @valid_from, valid_until = @valid_until, updated_at = now()
WHERE id = @id
RETURNING id, slug, name, description, enabled, priority, audience, conditions,
       payload, channels, completion_signal, frequency, valid_from, valid_until,
       created_by, created_at, updated_at;

-- name: DeleteNextActionRule :exec
DELETE FROM next_action_rules WHERE id = @id;

-- name: GetUserNextActions :many
SELECT una.id, una.user_id, una.rule_id, una.status, una.impressions,
       una.first_shown_at, una.last_shown_at, una.snoozed_until,
       una.dismissed_at, una.completed_at
FROM user_next_actions una
WHERE una.user_id = @user_id;

-- name: UpsertUserNextAction :one
INSERT INTO user_next_actions (user_id, rule_id, status)
VALUES (@user_id, @rule_id, 'pending')
ON CONFLICT (user_id, rule_id) DO NOTHING
RETURNING id, user_id, rule_id, status;

-- name: MarkNextActionShown :exec
UPDATE user_next_actions
SET status = CASE WHEN status = 'pending' THEN 'shown' ELSE status END,
    impressions = impressions + 1,
    first_shown_at = COALESCE(first_shown_at, now()),
    last_shown_at = now(),
    updated_at = now()
WHERE user_id = @user_id AND rule_id = @rule_id;

-- name: DismissNextAction :exec
UPDATE user_next_actions
SET status = 'dismissed', dismissed_at = now(), updated_at = now()
WHERE user_id = @user_id AND rule_id = @rule_id;

-- name: SnoozeNextAction :exec
UPDATE user_next_actions
SET status = 'snoozed', snoozed_until = now() + make_interval(hours => @hours),
    updated_at = now()
WHERE user_id = @user_id AND rule_id = @rule_id;

-- name: CompleteNextAction :exec
UPDATE user_next_actions
SET status = 'completed', completed_at = now(), updated_at = now()
WHERE user_id = @user_id AND rule_id = @rule_id;

-- name: CompleteNextActionByRuleSlug :exec
UPDATE user_next_actions
SET status = 'completed', completed_at = now(), updated_at = now()
WHERE user_id = @user_id
  AND rule_id = (SELECT id FROM next_action_rules WHERE slug = @slug);

-- name: CountUserEvents :one
SELECT count(*) FROM platform_events
WHERE user_id = @user_id AND event_type = @event_type
  AND created_at > now() - make_interval(days => @days);

-- name: CountSessionEvents :one
SELECT count(*) FROM platform_events
WHERE session_id = @session_id AND event_type = @event_type
  AND created_at > now() - make_interval(days => @days);

-- name: UserHasAssessmentResult :one
SELECT EXISTS(
    SELECT 1 FROM assessment_results WHERE user_id = @user_id
) AS has_result;

-- name: GetLatestAssessmentScore :one
SELECT total_score FROM assessment_results
WHERE user_id = @user_id
ORDER BY created_at DESC LIMIT 1;

-- name: GetUserForNextActionState :one
SELECT id, created_at, email_verified_at FROM users WHERE id = @user_id;

-- name: ListActiveUsersForSweep :many
SELECT DISTINCT user_id FROM platform_events
WHERE user_id IS NOT NULL
  AND created_at > now() - make_interval(days => @days)
LIMIT @batch_size OFFSET @batch_offset;

-- name: NextActionStats :many
SELECT nar.slug,
       count(*) FILTER (WHERE pe.event_type = 'next_action.shown')     AS shown,
       count(*) FILTER (WHERE pe.event_type = 'next_action.clicked')   AS clicked,
       count(*) FILTER (WHERE pe.event_type = 'next_action.dismissed') AS dismissed,
       count(*) FILTER (WHERE pe.event_type = 'next_action.completed') AS completed
FROM next_action_rules nar
LEFT JOIN platform_events pe
       ON pe.meta->>'rule_slug' = nar.slug
      AND pe.event_type LIKE 'next_action.%'
GROUP BY nar.slug
ORDER BY nar.slug;
```

- [ ] **Step 2: Regenerate and compile**

```bash
make sqlc-gen && go build ./internal/db/gen/...
```

Expected: `internal/db/gen/next_actions.sql.go` generated; build clean.

- [ ] **Step 3: Commit**

```bash
git add internal/db/queries/next_actions.sql internal/db/gen/
git commit -m "feat(db): sqlc queries for next-action engine (§122)"
```

---

### Task 4: Module types + rule validation

**Files:**
- Create: `internal/nextactions/types.go`
- Create: `internal/nextactions/validate.go`
- Test: `internal/nextactions/validate_test.go`

**Interfaces:**
- Produces (used by every later task):
```go
type Audience struct {
    UserClass           []string `json:"user_class"`
    Surfaces            []string `json:"surfaces"`
    Archetype           []string `json:"archetype"`
    JapaStage           []string `json:"japa_stage"`
    DestinationCountry  []string `json:"destination_country"`
    OriginCountry       []string `json:"origin_country"`
}
type ConditionGroup struct {
    All []Predicate `json:"all"`
    Any []Predicate `json:"any"`
}
type Predicate struct {
    Predicate  string   `json:"predicate"`
    In         []string `json:"in,omitempty"`
    Eq         *bool    `json:"eq,omitempty"`
    Lt         *int     `json:"lt,omitempty"`
    Gt         *int     `json:"gt,omitempty"`
    Gte        *int     `json:"gte,omitempty"`
    Slug       string   `json:"slug,omitempty"`
    Type       string   `json:"type,omitempty"`
    Feature    string   `json:"feature,omitempty"`
    WithinDays int      `json:"within_days,omitempty"`
    Window     string   `json:"window,omitempty"` // "session" or "Nd"
}
type Payload struct {
    Title, Body, CTALabel, CTAURL, Icon string // json tags
}
type Frequency struct {
    Mode              string `json:"mode"` // once|recurring
    MaxImpressions    int    `json:"max_impressions"`
    CooldownHours     int    `json:"cooldown_hours"`
    SnoozeDefaultHours int   `json:"snooze_default_hours"`
}
type Rule struct { /* all next_action_rules columns, decoded jsonb into the above */ }
type Suggestion struct {
    ID       string   `json:"id"`        // rule id
    ActionID string   `json:"action_id"` // user_next_actions id; "" for guests
    RuleSlug string   `json:"rule_slug"`
    Title    string   `json:"title"`
    Body     string   `json:"body"`
    CTALabel string   `json:"cta_label"`
    CTAURL   string   `json:"cta_url"`
    Icon     string   `json:"icon"`
    Channels []string `json:"channels"`
    Priority int32    `json:"priority"`
}
const (
    StatusPending="pending"; StatusShown="shown"; StatusDismissed="dismissed"
    StatusSnoozed="snoozed"; StatusCompleted="completed"; StatusExpired="expired"
)
var PullChannels = []string{"popup","announcement_bar","sidebar","in_article"}
var PushChannels = []string{"notification","push","email"}
func ValidateRule(r Rule) error // checks channels vocabulary, guest-channel rule, cta_url safety, predicate names
```

- [ ] **Step 1: Write the failing test**

`validate_test.go` — table-driven cases: rule with `notification` channel + `user_class:["guest"]` → error; `cta_url:"https://evil.com"` → error (must be `/`-relative or allowlisted surface origin); unknown channel `smoke_signal` → error; unknown predicate `telepathy` → error; valid seed-shaped rule → nil.

```go
func TestValidateRuleRejectsGuestPushChannels(t *testing.T) {
    r := Rule{Audience: Audience{UserClass: []string{"guest"}},
              Channels: []string{"push"}, Payload: Payload{Title:"t", CTAURL:"/x"}}
    if err := ValidateRule(r); err == nil {
        t.Fatal("expected error for guest+push")
    }
}
```

- [ ] **Step 2: Run test — confirm failure**

`go test ./internal/nextactions/ -run TestValidateRule -v` → FAIL (package/files don't exist).

- [ ] **Step 3: Implement types.go + validate.go**

Write the types above; `ValidateRule` enforces: non-empty slug/name/payload.title; channels ⊆ {popup, announcement_bar, sidebar, in_article, notification, push, email}; `guest ∈ user_class` ⇒ channels ⊆ PullChannels; `cta_url` starts with `/` or matches allowlisted surface origins from `nextactions.allowed_cta_origins` setting (comma-separated; validated in service, validate.go checks shape only); every predicate name ∈ registered vocabulary (exported `PredicateNames()` from predicates.go — Task 5; for this task validate against a `knownPredicates` map literal that Task 5 keeps in sync, or move the registry here).

- [ ] **Step 4: Run tests**

`go test ./internal/nextactions/` → PASS.

- [ ] **Step 5: Commit**

`git commit -m "feat(nextactions): rule types + validation (§122)"`

---

### Task 5: Predicate engine + UserStateResolver

**Files:**
- Create: `internal/nextactions/predicates.go`
- Create: `internal/nextactions/resolver.go`
- Test: `internal/nextactions/predicates_test.go`

**Interfaces:**
- Consumes: `types.go` Predicate/ConditionGroup; `gen` queries from Task 3; `visitor.Context` (`visitor.FromContext`); `tiers.Service.ResolveUserTier(ctx, userID) (string, error)`.
- Produces:
```go
// Resolver gathers user state once per request.
type StateResolver struct { /* deps: gen queries, tiers svc, onboarding store iface, settings */ }
type UserState struct {
    UserClass        string // "guest"|"regular"|"pro"|"business"
    UserID           pgtype.UUID // invalid for guests
    SessionKey       string // session_id or ip key
    AccountAgeDays   int
    OnboardingDone   bool
    EmailVerified    bool // users.email_verified_at IS NOT NULL
    ProfilePct       int
    VCtx             visitor.VisitorContext // FromContext return type: .Archetype .JapaStage .DestinationCountry .OriginCountry .CurrentCountry
    Surface          string
}
func (r *StateResolver) Resolve(ctx context.Context, req *http.Request) (*UserState, error)
func EvalConditions(ctx context.Context, g ConditionGroup, s *UserState, deps EvalDeps) bool
type EvalDeps interface {
    CountEvents(ctx context.Context, state *UserState, eventType string, days int) (int64, error)
    ToolUsed(ctx context.Context, state *UserState, slug string) (bool, error)
    FeatureUsed(ctx context.Context, state *UserState, slug string, days int) (bool, error)
    GateHit(ctx context.Context, state *UserState, feature string, days int) (bool, error)
    AssessmentDone(ctx context.Context, userID pgtype.UUID) (bool, int, error) // done, score
    HasPushSub(ctx context.Context, userID pgtype.UUID) (bool, error)
}
```

- [ ] **Step 1: Failing test**

`predicates_test.go` — table-driven `EvalConditions` cases over a fake `UserState` + stub `EvalDeps`: `account_age_days{lt:30}` true at 10; `destination_country{in:[CA]}` matches VCtx; `assessment_completed{eq:false}`; `event_count{type:content.viewed,gte:3,window:7d}` via stub counter; `all` AND-semantics vs `any` OR-semantics; empty group → true.

- [ ] **Step 2: Run — FAIL** (files missing).

- [ ] **Step 3: Implement**

- `resolver.go`: `Resolve` reads `auth.FromContext` (nil → guest, class `guest`, session key = JWT JTI else `clientip.FromRequest`); for users: account age + email_verified + onboarding completion + `tiers.ResolveUserTier` → user_class; `visitor.FromContext` embedded.
- `predicates.go`: dispatch on `p.Predicate`; each evaluator a small func `(ctx, p, s, deps) bool`. `event_count`/`tool_used`/`feature_used`/`feature_gate_hit` route to `EvalDeps` (session-scoped counts for guests — `CountSessionEvents` when `UserID` invalid, else `CountUserEvents`). `assessment_completed`/`assessment_score` → `UserHasAssessmentResult` + `GetLatestAssessmentScore` (returns `total_score`; `user_id` is nullable in `assessment_results` but the predicate is user-scoped anyway). `account_age_days`/`email_verified` → `GetUserForNextActionState` (`email_verified_at IS NOT NULL`). `profile_completeness` v1: compute from users row basics (avatar+bio+location set → pct) or skip predicate if no field exists — pick the simplest real signal, document in code ref comment.
- `dwell_seconds`/`session_page_views`: read Valkey counters `jc:nextactions:guest:{key}:pv` incremented by the track endpoint (Task 7); absent → 0.

- [ ] **Step 4: Run tests** → PASS. `make vet` clean.

- [ ] **Step 5: Commit** — `"feat(nextactions): predicate engine + user state resolver (§122)"`

---

### Task 6: Service — evaluation pipeline + state transitions

**Files:**
- Create: `internal/nextactions/service.go`
- Create: `internal/nextactions/guest.go`
- Test: `internal/nextactions/service_test.go`

**Interfaces:**
- Consumes: Tasks 3–5.
- Produces:
```go
type Service struct { /* store *gen.Queries, cache valkeycompat.Cmdable, settings, resolver *StateResolver, deps EvalDeps impl, notifier Notifier iface */ }
func New(deps Deps) *Service
func (s *Service) Suggestions(ctx context.Context, r *http.Request, surface string, limit int) ([]Suggestion, error)
func (s *Service) Dismiss(ctx context.Context, r *http.Request, ruleID string, snoozeHours int) error
func (s *Service) Complete(ctx context.Context, r *http.Request, ruleID string) error
func (s *Service) Track(ctx context.Context, r *http.Request, ruleID, event string) error
func (s *Service) MergeGuestState(ctx context.Context, userID pgtype.UUID, guestKey string) error // called from auth/login+register wiring
type Notifier interface { NotifySuggestion(ctx context.Context, userID pgtype.UUID, rule Rule) error } // implemented by §35 adapter in Task 10
func (s *Service) Sweep(ctx context.Context) error // Task 10 calls this
```

- [ ] **Step 1: Failing test**

`service_test.go` with fake store + miniredis-style Valkey mock (check how existing tests fake `valkeycompat.Cmdable` — `recommendations/handlers_test.go` shows the pattern; reuse it): rule enabled + conditions pass + no state → suggestion returned; dismissed rule → filtered; `max_impressions` reached → filtered; snoozed_until future → filtered; guest → Valkey state consulted, suggestion carries `action_id:""`.

- [ ] **Step 2: Run — FAIL.**

- [ ] **Step 3: Implement**

- `service.go::Suggestions`: cache check `jc:nextactions:user:{uid}`/`guest:{key}` (TTL `nextactions.cache_ttl_seconds`) → load enabled rules (Valkey `jc:nextactions:rules`, invalidate on admin mutation — write `InvalidateRulesCache(ctx)`) → resolve state → filter audience (`user_class`, `surfaces`, geo/stage/archetype) → `EvalConditions` → load user state rows (or guest hash) → drop dismissed/snoozed/completed/expired/capped → eval `completion_signal` (true ⇒ `CompleteNextAction` write + drop) → apply `nextactions.max_active_per_user` + `max_upsell_per_day` (upsell = rules whose completion_signal mentions `account_tier`) → `UpsertUserNextAction` + `MarkNextActionShown` for members → sort by priority → slice `limit` → cache → return.
- `guest.go`: `guestState(ctx, key)` HGETALL `jc:nextactions:guest:{key}` decode `{ruleID: {status, impressions, snoozed_until}}`; `dismissGuest`, `snoozeGuest`, `completeGuest` write back with TTL `nextactions.guest_state_ttl_seconds`. `MergeGuestState`: copy terminal states (dismissed/completed) into `user_next_actions` rows at login/register — wire the call in Task 8.
- `Dismiss`/`Complete`: members → sqlc execs + cache bust; guests → guest.go. `Snooze`: hours = body or `frequency.snooze_default_hours` or 168.
- `Track`: writes via `analytics.Service.TrackEvent(ctx, TrackEventInput{EventType, UserID, SessionID, Meta: {"rule_slug": slug}})` — event types `next_action.shown|clicked|dismissed|completed` (registered in Task 9's ValidEventTypes extension). Do NOT insert into `platform_events` directly — `TrackEvent` enforces the `ValidEventTypes` allowlist.

- [ ] **Step 4: Tests pass** + `go build ./...` clean.

- [ ] **Step 5: Commit** — `"feat(nextactions): evaluation service, state machine, guest Valkey state (§122)"`

---

### Task 7: HTTP handlers — public + admin

**Files:**
- Create: `internal/nextactions/handlers.go`
- Create: `internal/nextactions/admin.go`
- Test: `internal/nextactions/handlers_test.go`

**Interfaces:**
- Consumes: Task 6 `Service`; `errpkg.WriteJSON`; `auth.FromContext`; `rbac` middleware applied at route level.
- Produces (mounted in Task 8):
```go
type Handlers struct{ svc *Service; settings *config.Settings }
func NewHandlers(svc *Service, settings *config.Settings) *Handlers
func (h *Handlers) GetSuggestions(w, r)     // GET /api/v1/next-actions + /me alias
func (h *Handlers) Dismiss(w, r)            // POST {id}/dismiss {snooze_hours?}
func (h *Handlers) Complete(w, r)           // POST {id}/complete
func (h *Handlers) Track(w, r)              // POST {id}/track {event}
func (h *Handlers) ListRules(w, r); CreateRule; GetRule; UpdateRule; DeleteRule; Stats
```

- [ ] **Step 1: Failing test** — httptest: `GetSuggestions` 200 + `{data:[...]}` envelope; invalid `{id}` → `next_action_not_found` 404; admin `CreateRule` with bad channel → 400 `invalid_rule`; unauthenticated create → handled by middleware (route test in Task 8).

- [ ] **Step 2: FAIL.**

- [ ] **Step 3: Implement** — handlers decode chi URLParam `{id}` (rule ID), parse JSON body, call service, `errpkg.WriteJSON`. Admin handlers call `ValidateRule` before store writes and call `svc.InvalidateRulesCache` after mutations. `Stats` → `NextActionStats` query → `{data:[{slug,shown,clicked,dismissed,completed}]}`.

- [ ] **Step 4: PASS** + vet/lint.

- [ ] **Step 5: Commit** — `"feat(nextactions): public + admin HTTP handlers (§122)"`

---

### Task 8: Wiring — feature flag, routes, DI, guest-state merge

**Files:**
- Modify: `internal/features/registry.go` (add flag entry)
- Modify: `internal/server/server.go` (DI + routes)
- Modify: `internal/auth/` login/register handler where post-auth hooks run (find the spot that already invalidates visitor caches — `users/handlers.go` `invalidateVisitorCache` precedent) to call `svc.MergeGuestState`
- Modify: `internal/features/features_test.go` registry test data if it asserts flag counts (check first)

**Interfaces:**
- Consumes: all backend tasks. Route registrations must match the catalog strings from Task 1 verbatim.

- [ ] **Step 1: Add flag**

```go
{Key: "next_actions.enabled", Label: "Next-Action Suggestions", Description: "Contextual next-step suggestions delivered via popups, bars, sidebar, in-article, and notifications", Default: true, Category: "Engagement"},
```

- [ ] **Step 2: Wire DI in server.go** — near `recommendationsHandlers` (~line 765): build `nextactions` Service (store from `gen.New`, Valkey `vkClient.Redis()`, settings, tiers svc, onboarding svc, assessment store, analytics) → `NewHandlers`. Follow the existing constructor-injection style.

- [ ] **Step 3: Register routes** — public routes sit inside the OptionalAuth route block near `/recommendations` (~line 4000); admin under the `/admin` group with `rbac.RequirePermission`:

```go
r.With(features.Gate(featureChecker, "next_actions.enabled")).Group(func(r chi.Router) {
    r.Get("/next-actions", nextActionsHandlers.GetSuggestions)
    r.Post("/next-actions/{id}/dismiss", nextActionsHandlers.Dismiss)
    r.Post("/next-actions/{id}/complete", nextActionsHandlers.Complete)
    r.Post("/next-actions/{id}/track", nextActionsHandlers.Track)
})
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "next_actions.enabled")).Get("/me/next-actions", nextActionsHandlers.GetSuggestions)
// admin group:
r.With(rbac.RequirePermission(rbacResolver, "nextactions.view")).Get("/admin/next-actions/stats", ...)
r.With(rbac.RequirePermission(rbacResolver, "nextactions.manage")).Route("/admin/next-actions/rules", func(r chi.Router) { ...CRUD... })
```

- [ ] **Step 4: Guest-state merge hook** — in the login/register success path, call `nextActionsSvc.MergeGuestState(ctx, userID, guestSessionKey)` (guest key = the `jc:visitor:ctx:ip:{ip}`-style key the request carried — same derivation as resolver; expose `nextactions.GuestKey(r)`).

- [ ] **Step 5: Build + smoke test**

`go build ./...` then run server, `curl localhost:PORT/api/v1/next-actions?surface=main` → `{data:[...]}` (seed rules eligible for your test context).

- [ ] **Step 6: Commit** — `"feat(nextactions): wire routes, flag, DI, guest-state merge (§122)"`

---

### Task 9: Analytics event types + `feature.gate_hit` / `tool.used` emission

`analytics.Service.TrackEvent` validates every `event_type` against the
`ValidEventTypes()` allowlist (`internal/analytics/types.go`) — new event types
must be registered there FIRST or every Track call fails.

**Files:**
- Modify: `internal/analytics/types.go` (add EventType constants + register in `ValidEventTypes()`)
- Modify: `internal/analytics/service_test.go` (extend `TestValidEventTypes` expectations)
- Modify: `internal/publicapi/tier.go` (or the actual tier-gate deny funnel — verify at implement time)
- Modify: tool completion points that should emit `tool.used` — minimum viable set: `internal/crscalculator` calculate handler. Other tools opt in later by firing `POST /api/v1/analytics/events {event_type:"tool.used", meta:{tool:"<slug>"}}` from their frontend primary action — the `tool_used` predicate lights up automatically as they adopt it; not required for v1.

- [ ] **Step 1: Register event types**

```go
// internal/analytics/types.go — append to the const block:
EventNextActionShown     EventType = "next_action.shown"
EventNextActionClicked   EventType = "next_action.clicked"
EventNextActionDismissed EventType = "next_action.dismissed"
EventNextActionCompleted EventType = "next_action.completed"
EventToolUsed            EventType = "tool.used"
EventFeatureGateHit      EventType = "feature.gate_hit"
```

Add all six to the `ValidEventTypes()` slice; extend `TestValidEventTypes` to assert them.

- [ ] **Step 2: Emit `feature.gate_hit`** — find the tier-gate deny funnel (`internal/publicapi/tier.go` and any feature-limit middleware). On deny, fire-and-forget `analytics.TrackEvent{EventType: EventFeatureGateHit, EntityType: "feature", Meta: {"feature": "<slug>", "required_tier": "<tier>"}}` — never block the 403 response.

- [ ] **Step 3: Emit `tool.used` server-side where tools persist** — minimum: the CRS calculate endpoint in `internal/crscalculator` (fires on every calculation, `Meta: {"tool": "crs-calculator"}`, session_id from request for guests). Add `Meta["tool"]` convention note — `tool_used` predicates match `event_type='tool.used' AND meta->>'tool'=@slug` (`slug="*"` matches any). Update the `CountUserEvents`/`CountSessionEvents` queries to filter `meta->>'tool'` when a slug is given — or add dedicated `CountToolEvents`/`CountSessionToolEvents` queries:

```sql
-- name: CountToolEvents :one
SELECT count(*) FROM platform_events
WHERE user_id = @user_id AND event_type = 'tool.used'
  AND (@tool_slug::text = '*' OR meta->>'tool' = @tool_slug)
  AND created_at > now() - make_interval(days => @days);

-- name: CountSessionToolEvents :one
SELECT count(*) FROM platform_events
WHERE session_id = @session_id AND event_type = 'tool.used'
  AND (@tool_slug::text = '*' OR meta->>'tool' = @tool_slug)
  AND created_at > now() - make_interval(days => @days);
```

- [ ] **Step 4: Test** — deny a regular-tier gated request → `feature.gate_hit` row written; calculate a CRS score → `tool.used` row with `meta.tool=crs-calculator`.

- [ ] **Step 5: Commit** — `"feat(analytics): next-action/tool/gate event types + emissions (§122)"`

---

### Task 10: Sweep task + notification dispatch

**Files:**
- Modify: `internal/worker/tasks.go` (add `TypeNextActionsSweep = "nextactions:sweep"`)
- Modify: `internal/worker/server.go` (schedule + mux registration)
- Modify: `internal/worker/handlers.go` (handler) — or new `internal/worker/nextactions.go` if handlers.go is unwieldy
- Create: `internal/nextactions/notify.go` (§35 adapter)

**Interfaces:**
- Consumes: Task 6 `Service.Sweep`, `notifications.Service.CreateNotification(ctx, recipientID, nType, actorType, actorID, subjectType, subjectID, title, body, actionURL)`.

- [ ] **Step 1: Register task + schedule**

```go
// tasks.go
TypeNextActionsSweep = "nextactions:sweep" // §122
// server.go, near the persona/orchestration entries:
schedule("*/60 * * * *", TypeNextActionsSweep, time.Hour) // interval re-checked inside handler via nextactions.sweep_interval_minutes
```

Follow the existing "handler enforces the settings interval" pattern used by `persona:run_activity`.

- [ ] **Step 2: notify.go adapter**

```go
type Notifier struct{ svc *notifications.Service }
func (n *Notifier) NotifySuggestion(ctx context.Context, userID pgtype.UUID, rule Rule) error {
    return n.svc.CreateNotification(ctx, userID, "next_action.suggested",
        notifications.ActorTypeSystem, pgtype.UUID{}, "next_action_rule",
        rule.ID, rule.Payload.Title, rule.Payload.Body, rule.Payload.CTAURL)
}
```

- [ ] **Step 3: Service.Sweep impl** (Task 6 file): `jc:nextactions:sweep:lock` SET NX EX ~55min → for each batch from `ListActiveUsersForSweep(days=nextactions.sweep_activity_window_days)` → run rule eval for rules whose channels ∩ PushChannels ≠ ∅ → `UpsertUserNextAction` → `Notifier.NotifySuggestion` → `MarkNextActionShown`.

- [ ] **Step 4: Worker handler**

```go
func HandleNextActionsSweep(ctx context.Context, _ *asynq.Task) error {
    if dbPool == nil || nextActionsSvc == nil { return nil }
    return nextActionsSvc.Sweep(ctx)
}
```

Worker deps: follow how other services are injected into `worker` (a package-level setter like `dbPool`); add `SetNextActionsService` called from server wiring.

- [ ] **Step 5: Test** — unit-test `Sweep` with fakes: eligible user gets row + one notification; second sweep → no duplicate (UNIQUE key); `feature_gate_hit`-style completion_signal rules skip completed users.

- [ ] **Step 6: Commit** — `"feat(nextactions): sweep task + §35 notification dispatch (§122)"`

---

### Task 11: Main app — API client + announcement bar + popup

**Files:**
- Create: `apps/main/src/lib/api/nextActions.ts`
- Create: `apps/main/src/lib/components/shell/NextActionBar.svelte`
- Create: `apps/main/src/lib/components/shell/NextActionPopup.svelte`
- Modify: `apps/main/src/routes/(app)/+layout.svelte` (mount components inside the shell)
- Modify: `apps/main/src/lib/types/index.ts` (NextAction type)
- Modify: `apps/main/src/lib/i18n/en.json` + `fr.json` (labels — match existing i18n style)

**Interfaces:**
- Consumes: `api` from `$lib/api/client`; endpoint contract `{data:[{id,action_id,rule_slug,title,body,cta_label,cta_url,icon,channels,priority}]}`.

- [ ] **Step 1: API client + types**

```ts
// api/nextActions.ts — §122
import { api } from '$lib/api/client';
import type { NextAction } from '$lib/types';

export async function getNextActions(surface: string): Promise<NextAction[]> {
	const res = await api.get<NextAction[]>(`/next-actions?surface=${surface}`);
	return res.data ?? [];
}
export const dismissNextAction = (id: string, snoozeHours?: number) =>
	api.post(`/next-actions/${id}/dismiss`, snoozeHours ? { snooze_hours: snoozeHours } : {});
export const completeNextAction = (id: string) => api.post(`/next-actions/${id}/complete`, {});
export const trackNextAction = (id: string, event: 'shown' | 'clicked') =>
	api.post(`/next-actions/${id}/track`, { event }).catch(() => {});
```

```ts
// types
export interface NextAction {
	id: string; action_id: string; rule_slug: string;
	title: string; body: string; cta_label: string; cta_url: string;
	icon: string; channels: string[]; priority: number;
}
```

- [ ] **Step 2: NextActionBar.svelte** — Svelte 5 runes (`$state`, `$derived`, `onMount`), dismissible bar matching the shell's banner styling (check `CtaBanner.svelte` for the house style); fetches via `getNextActions('main')`, shows the highest-priority action whose `channels` includes `announcement_bar`; `trackNextAction(id,'shown')` on render; CTA → `track('clicked')` + `completeNextAction` + `goto(cta_url)`; dismiss → `dismissNextAction`.

- [ ] **Step 3: NextActionPopup.svelte** — Bits UI Dialog, one-per-session (in-memory `$state` + sessionStorage flag to respect `popup_max_per_session`); same tracking.

- [ ] **Step 4: Mount in `+layout.svelte`** — both components near the shell top-level; popup only when `$page.data.user` context allows (component itself handles guest vs user uniformly — API does the filtering).

- [ ] **Step 5: Verify** — `bun run check` + `bun run build` in apps/main; manual: logged-out homepage shows guest popup after 3 content views.

- [ ] **Step 6: Commit** — `"feat(main): next-action bar + popup renderers (§122)"`

---

### Task 12: Sidebar widget type + renderer

**Files:**
- Modify: `internal/sidebars/widget_types.go` (add `next_action` widget type entry — copy the `suggested_content` entry shape)
- Create: `migrations/000914_next_action_sidebar_widget.up.sql` + `.down.sql` (seed `next_action` widget rows into main right sidebars — copy migration `000802`'s insert pattern; `visibility='all'`, positioned after `suggested_content` where present)
- Modify: `apps/main/src/lib/components/shell/SidebarWidgetRenderer.svelte` (data fetch + render branch)

- [ ] **Step 1: Widget type entry** — valid categories: `WidgetCategoryNavigation|Dashboard|Content|Social|Monetization|Academy|Marketing|Custom`:

```go
{
    Type: "next_action", Name: "Next Step", Icon: "compass",
    Description: "Personalised next-step suggestion — data fetched from /next-actions?surface=main. Guests and members alike; hidden while empty",
    Slot: WidgetSlotContent, Category: WidgetCategoryMarketing, Apps: appMainOnly,
    DefaultProps: map[string]any{"limit": 1, "title": "Your next step"},
    PropSchema: []PropField{limitProp(), titleProp()},
},
```

- [ ] **Step 2: Seed migration** — `000914`: `INSERT INTO sidebar_widgets (sidebar_id, widget_type, label, props, sort_order, visibility, status)` for each `sidebars s WHERE s.app='main' AND s.position='right'` lacking a `next_action` row — after `suggested_content` (or `suggested_spaces`) sort position. Down migration deletes `widget_type='next_action'` rows.

- [ ] **Step 3: Renderer branch** — mirror the `suggested_content` fetch pattern: `fetchNextActions()` calling `getNextActions('main')`, filter `channels` contains `sidebar`, render compact card (title/body/CTA/dismiss) in the `{:else if widget.widget_type === 'next_action'}` branch.

- [ ] **Step 4: `bun run check` + build** → commit `"feat(sidebar): next_action widget type + seed (§122)"`

---

### Task 13: In-article inline card

**Files:**
- Create: `apps/main/src/lib/components/shell/NextActionInline.svelte` (or `lib/components/content/` — match `InArticleAd.svelte` location)
- Modify: the article/guide reader component where `InArticleAd.svelte` is injected (find via `grep -rn "InArticleAd" apps/main/src`)

- [ ] **Step 1: Component** — card styled distinct from ads (icon + title + body + CTA + dismiss), same injection mechanics as `InArticleAd.svelte` (mid-content slot).

- [ ] **Step 2: Wire into reader** — fetch `getNextActions('main')`, pick first with `channels` containing `in_article`, inject after the first content block the same way ads inject; `trackNextAction` shown/clicked.

- [ ] **Step 3: Verify + commit** — `"feat(main): in-article next-action card (§122)"`

---

### Task 14: Control panel — rules management UI

**Files:**
- Create: `apps/control/src/routes/next-actions/+page.svelte` (DataTable list per the panel's DataTable spec — sort/search/filter/paginate/actions)
- Create: `apps/control/src/routes/next-actions/[id]/+page.svelte` (editor: name, enabled, priority, audience JSON fields, conditions builder or JSON textarea with validation, payload, channels checkboxes, frequency, validity window)
- Modify: `apps/control/src/lib` nav/menu file (add route entry — find how `permission-nodes`/`settings` are listed)
- Create: `apps/control/src/lib/api/nextactions.ts` (typed client matching control's api pattern)

- [ ] **Step 1: List page** — DataTable over `GET /admin/next-actions/rules`: columns slug/name/enabled/priority/channels/updated; row actions edit/toggle/delete; create button.

- [ ] **Step 2: Editor** — form fields → PATCH/POST; conditions + audience as validated JSON editors (json parse errors inline); channels checkbox group with guest-channel validation hint.

- [ ] **Step 3: Stats** — small funnel table via `GET /admin/next-actions/stats` on the list page or a stats tab.

- [ ] **Step 4: `bun run check` + build** → commit `"feat(control): next-action rules management UI (§122)"`

---

### Task 15: Mobile — polling + card

**Files:**
- Create: `apps/mobile/lib/features/next_actions/` (model + provider + card widget per the app's feature-module pattern — check an existing small feature module for shape)
- Modify: home screen to render the highest-priority action card

- [ ] **Step 1:** `NextAction` model + `nextActionsProvider` (Riverpod FutureProvider hitting `GET /api/v1/next-actions?surface=mobile`).
- [ ] **Step 2:** `NextActionCard` widget — title/body/CTA/dismiss; dismiss + complete calls; tap → navigate via `cta_url` route mapping.
- [ ] **Step 3:** `flutter analyze` clean → commit `"feat(mobile): next-action card + polling (§122)"`

---

## Self-review notes

- **Coverage:** spec sections ↔ tasks: JEOS (T1), tables (T2), queries (T3), types/validation (T4), predicates (T5), service+guest (T6), handlers (T7), wiring+flag+merge (T8), gate_hit (T9), sweep+notify (T10), bar+popup (T11), sidebar (T12), in-article (T13), control UI (T14), mobile (T15). Seed rules in T2. All settings/keys/tasks/RBAC registered in T1 and seeded in T2.
- **Type consistency:** `Suggestion` shape in T4 matches handler envelope (T7) and TS interface (T11); `Service` methods in T6 match handler calls (T7) and sweep (T10); `Notifier` iface T6 ↔ impl T10; predicate names in T4 validation ↔ T5 registry.
