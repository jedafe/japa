# Design: AI Content Campaigns — Multi-Persona Content Orchestration

**Status:** Draft — pending approval  
**Refs:** §75 (AI Content Studio), §68 (AI Persona System), §103 (AI Agent Executor), ADR-076, ADR-068, ADR-111, §03 (RBAC), §22 (HITL / content_reviews)

## 1. Problem

A user can currently create one AI-generated content draft at a time through the agent chat (`content.create`, `content.create_draft`) or configure a batch of Studio jobs manually in the Admin Studio. There is no way to turn a single natural-language brief such as:

> "Create 20 blog posts for Canadian users about investing in African countries"

into a campaign of 20 distinct pieces, each authored by a different AI persona, with each piece routed through the existing Studio generation and HITL review pipeline.

The desired behavior is **orchestration**: the system plans the set of content pieces, selects or assigns a unique persona per piece, and dispatches them through the proven `content:generate_draft` task flow.

## 2. Goal

Introduce a **Content Campaign** feature that:

- Accepts a high-level brief from either the AI chat or the Admin Studio.
- Plans N unique content pieces (title, topic prompt, content type, target geo/audience).
- Assigns **one distinct AI persona per piece** within the same campaign.
- Creates a Studio batch so each piece is generated, reviewed, and published through the existing §75 pipeline.
- Is fully auditable: the plan, the persona assignments, and the resulting Studio jobs are persisted.

## 3. Design

### 3.1. Persona uniqueness rule (hard constraint)

The orchestrator **must not** assign more than one campaign item to the same persona unless the caller explicitly opts in.

- A campaign with `requested_count = N` requires at least `N` distinct active personas.
- If the user provides a `persona_pool` with fewer than `N` personas, the campaign creation fails with a clear error: `persona_pool_too_small` **unless** `auto_provision_personas = true` and the caller has permission to create personas.
- If no `persona_pool` is provided, the orchestrator uses all active `ai_personas` for the tenant. If there are fewer active personas than `N`:
  - With `auto_provision_personas = true` and the right permission, the orchestrator generates and provisions the missing personas.
  - Otherwise, it fails with `persona_pool_too_small`.
- `auto_provision_personas` defaults to `true` in the `content.campaign.create` tool and in the Admin Studio form.
- The system prompt given to the planner explicitly instructs the model: "Assign a different persona to every item. Never reuse a persona within the same campaign."
- The service layer validates the returned plan and rejects any plan that reuses a persona.
- An optional `allow_persona_reuse` flag (default `false`) may be added in a future iteration; it is **out of scope** for the first version.

This means a prompt like:

> "Create 20 blog posts for Canadian users about investing in African countries"

is interpreted by the orchestrator as:

> "Create 20 unique blog posts, each authored by a different persona, for Canadian users about investing in African countries."

### 3.2. Data model

`content_campaigns` — one row per campaign.

```sql
CREATE TABLE content_campaigns (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  name            text NOT NULL,
  prompt          text NOT NULL,
  status          text NOT NULL DEFAULT 'planning'
                    CHECK (status IN ('planning','planned','generating','completed','failed','cancelled')),
  content_type    text,                                -- null = mixed types, planner decides
  requested_count int NOT NULL
                    CHECK (requested_count > 0 AND requested_count <= 100),
  persona_pool    uuid[],                              -- null = all active personas
  auto_provision_personas boolean NOT NULL DEFAULT true, -- true = create missing personas automatically
  auto_assign     boolean NOT NULL DEFAULT true,       -- true = AI selects from pool, false = round-robin
  target_space_id uuid REFERENCES spaces(id),
  target_tags     text[],
  plan_json       jsonb,
  created_by      uuid NOT NULL REFERENCES users(id),
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_content_campaigns_tenant_status ON content_campaigns (tenant_id, status, created_at DESC);
```

`content_campaign_items` — one row per planned/generated piece.

```sql
CREATE TABLE content_campaign_items (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  campaign_id     uuid NOT NULL REFERENCES content_campaigns(id) ON DELETE CASCADE,
  sort_order      int NOT NULL DEFAULT 0,
  persona_id      uuid NOT NULL REFERENCES ai_personas(id),
  content_type    text NOT NULL,
  topic_prompt    text NOT NULL,
  title           text,
  target_space_id uuid REFERENCES spaces(id),
  target_tags     text[],
  status          text NOT NULL DEFAULT 'planned'
                    CHECK (status IN ('planned','queued','generating','draft_ready','in_review','approved','published','rejected','failed')),
  studio_job_id   uuid REFERENCES studio_jobs(id),
  content_id      uuid REFERENCES contents(id),
  review_id       uuid REFERENCES content_reviews(id),
  error_message   text,
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_content_campaign_items_campaign ON content_campaign_items (campaign_id, status, sort_order);
CREATE UNIQUE INDEX idx_content_campaign_items_unique_persona
  ON content_campaign_items (campaign_id, persona_id)
  WHERE status NOT IN ('failed','cancelled');
```

The unique index enforces the persona-uniqueness rule at the database level.

### 3.3. Campaign service (`internal/campaign`)

New package `internal/campaign` with:

- `Service` struct
- `Store` / `gen.Queries` for persistence
- `Planner` for the AI Gateway call
- `Handlers` for the HTTP API

Public service methods:

```go
func (s *Service) CreateCampaign(ctx context.Context, tenantID, userID pgtype.UUID, in CreateCampaignInput) (Campaign, error)
func (s *Service) PlanCampaign(ctx context.Context, tenantID, userID pgtype.UUID, campaignID pgtype.UUID) (Campaign, []CampaignItem, error)
func (s *Service) RunCampaign(ctx context.Context, tenantID, userID pgtype.UUID, campaignID pgtype.UUID) (studio.Batch, []studio.Job, error)
func (s *Service) ListCampaigns(ctx context.Context, tenantID pgtype.UUID, f CampaignListFilter) ([]Campaign, int64, error)
func (s *Service) GetCampaign(ctx context.Context, tenantID, campaignID pgtype.UUID) (Campaign, []CampaignItem, error)
func (s *Service) RetryItem(ctx context.Context, tenantID, userID, itemID pgtype.UUID) error
func (s *Service) CancelCampaign(ctx context.Context, tenantID, userID, campaignID pgtype.UUID) error
```

Input / DTO types:

```go
type CreateCampaignInput struct {
    Prompt                string
    Name                  string            // optional; generated from prompt if empty
    ContentType           string            // optional
    Count                 int
    PersonaPool           []string          // optional; persona UUIDs
    AutoProvisionPersonas bool              // default true
    AutoAssign            bool              // default true
    TargetSpaceID         string            // optional
    TargetTags            []string
    RunImmediately        bool
}

type CampaignListFilter struct {
    Status  string
    Limit   int32
    Offset  int32
}

type Campaign struct {
    ID                    string
    TenantID              string
    Name                  string
    Prompt                string
    Status                string
    ContentType           string
    RequestedCount        int
    PersonaPool           []string
    AutoProvisionPersonas bool
    AutoAssign            bool
    TargetSpaceID         string
    TargetTags            []string
    CreatedBy             string
    CreatedAt             string
    UpdatedAt             string
}

type CampaignItem struct {
    ID            string
    CampaignID    string
    SortOrder     int
    PersonaID     string
    PersonaName   string
    ContentType   string
    TopicPrompt   string
    Title         string
    TargetSpaceID string
    TargetTags    []string
    Status        string
    StudioJobID   string
    ContentID     string
    ReviewID      string
    ErrorMessage  string
    CreatedAt     string
}

type CampaignItemPlan struct {
    PersonaID     string
    ContentType   string
    Title         string
    TopicPrompt   string
    TargetSpaceID string
    TargetTags    []string
}

type PersonaSpec struct {
    Name          string
    Slug          string
    Tagline       string
    VoicePrompt   string
    ActivityPolicy map[string]any // optional; defaults to §68 ActivityPolicy if empty
}

type CampaignPlanResult struct {
    NewPersonas []PersonaSpec
    Items       []CampaignItemPlan
}
```

`CreateCampaign` performs:

1. Validates `requested_count` against `campaign.max_count` (default 50, max 100).
2. Loads the available active persona pool:
   - If `persona_pool` is provided, validate all IDs belong to the tenant and are active.
   - If not, list all active `ai_personas` for the tenant.
3. Computes the persona deficit: `deficit = requested_count - len(pool)`.
4. If `deficit > 0`:
   - If `auto_provision_personas = false`, return error `persona_pool_too_small`.
   - If `auto_provision_personas = true`, check caller has `persona.manage` permission.
   - Call `Planner.PlanPersonas(...)` to generate `deficit` new `PersonaSpec` rows.
   - Call `personas.Service.ProvisionPersonas` transactionally to create the new `ai_personas`, `users.is_bot`, and `professional_profiles` rows (§68).
   - Add the newly created persona IDs to the pool.
5. Insert `content_campaigns` row with `status = 'planning'`.
6. Call `Planner.PlanContent(...)` to generate `CampaignPlanResult`.
7. Validate that every `persona_id` is in the pool and has not been reused.
8. Insert `content_campaign_items` rows; enforce no duplicate `persona_id`.
9. Update campaign `status = 'planned'` and store the plan in `plan_json`.
10. If `run_immediately = true`, call `RunCampaign`.

`RunCampaign` performs:

1. Load planned items.
2. Call the existing `studio.Service.CreateBatch` with one `CreateJobInput` per item, preserving the item's `persona_id`, `content_type`, `topic_prompt`, `target_space_id`, and `target_tags`.
3. Update each item's `studio_job_id` and `status = 'queued'`.
4. Update campaign `status = 'generating'`.

The `Planner` has two responsibilities:

`Planner.PlanPersonas` performs:

1. Build a structured system prompt that includes:
   - The user brief,
   - The number of new personas needed,
   - The tenant's profile type defaults (`contributor`/`columnist` per §68),
   - The desired voice/role diversity (inferred from the brief).
2. Call the AI Gateway with a JSON-mode prompt.
3. Parse the result into `[]PersonaSpec`.
4. Validate each spec has `name`, `slug`, and `voice_prompt`.

`Planner.PlanContent` performs:

1. Build a structured system prompt that includes:
   - The user brief,
   - The list of allowed content types,
   - The **final** persona pool (existing + newly provisioned), with id, name, and voice prompt summary,
   - The persona-uniqueness rule,
   - Geo/audience hints from `target_space_id` and `target_tags`.
2. Call the AI Gateway with a JSON-mode prompt.
3. Parse the result into `[]CampaignItemPlan`.
4. Validate that every `persona_id` is in the pool and has not been reused.
5. If validation fails, return an error and mark campaign `failed`.

### 3.4. Agent tools

Register two new tools in `internal/ai/tools_register.go` and `ai_tools`:

#### `personas.create` (standalone persona provisioning)

```jsonc
{
  "function_name": "personas.create",
  "description": "Create one or more AI personas. Each persona is provisioned transactionally with a users.is_bot backing account, a professional profile byline, and an ai_personas config row.",
  "parameters": {
    "type": "object",
    "properties": {
      "personas": {
        "type": "array",
        "description": "List of persona definitions",
        "items": {
          "type": "object",
          "properties": {
            "name": { "type": "string" },
            "slug": { "type": "string" },
            "tagline": { "type": "string" },
            "voice_prompt": { "type": "string" },
            "disclosure_mode": { "type": "string", "enum": ["disclosed", "undisclosed"] },
            "autonomy_mode": { "type": "string", "enum": ["autonomous", "queued"] }
          },
          "required": ["name", "slug", "voice_prompt"]
        }
      }
    },
    "required": ["personas"]
  }
}
```

Required permission: `persona.manage`.

#### `content.campaign.create`

```jsonc
{
  "function_name": "content.campaign.create",
  "description": "Create a content campaign that plans and optionally generates multiple unique content pieces, each authored by a different AI persona. If there are not enough personas, new ones are provisioned automatically by default.",
  "parameters": {
    "type": "object",
    "properties": {
      "prompt": { "type": "string", "description": "High-level brief describing the content campaign" },
      "content_type": { "type": "string", "description": "Content type (e.g. blog_post, guide, discussion). Omit to let the AI decide per item." },
      "count": { "type": "integer", "description": "Number of unique pieces to create" },
      "persona_ids": { "type": "array", "items": { "type": "string" }, "description": "Optional pool of persona UUIDs" },
      "auto_provision_personas": { "type": "boolean", "description": "True (default) = create missing personas automatically; false = fail if not enough exist" },
      "auto_assign": { "type": "boolean", "description": "True (default) = AI picks persona per item; false = round-robin" },
      "target_space_id": { "type": "string", "description": "Optional geo space UUID" },
      "target_tags": { "type": "array", "items": { "type": "string" } },
      "run_immediately": { "type": "boolean", "description": "True = create Studio batch immediately; false = plan only (default)" }
    },
    "required": ["prompt", "count"]
  }
}
```

The `content.campaign.create` tool handler:

- Validates `count` against `campaign.max_count`.
- If `auto_provision_personas = true`, verifies the caller has `persona.manage` before calling the campaign service.
- Calls `campaign.Service.CreateCampaign`.
- Returns the campaign ID, requested count, number of personas provisioned, planned items summary, and status.

Required permission: `campaign.create`. Additional required permission for auto-provisioning: `persona.manage`.

### 3.5. Admin Studio UI

New tab inside `/admin/studio`:

- **Campaigns list** — name, prompt, status, requested count, completed count, created at.
- **Create campaign form** — fields: prompt, content type, count, persona multi-select (optional), auto-provision personas checkbox (default on), auto-assign toggle, target space, target tags, run immediately checkbox.
- **Campaign detail** — items table with persona, content type, title, topic prompt, status, studio job id, content id, review id; actions: Run (if planned), Retry (if failed), Cancel.

The form calls:

- `POST /api/v1/admin/campaigns`
- `POST /api/v1/admin/campaigns/:id/run`
- `POST /api/v1/admin/campaigns/:id/retry`
- `POST /api/v1/admin/campaigns/:id/cancel`

### 3.6. API routes

```
POST   /api/v1/admin/campaigns              — create and plan a campaign
GET    /api/v1/admin/campaigns              — list campaigns
GET    /api/v1/admin/campaigns/:id          — campaign detail + items
POST   /api/v1/admin/campaigns/:id/run      — create Studio batch from planned items
POST   /api/v1/admin/campaigns/:id/retry    — retry failed items
POST   /api/v1/admin/campaigns/:id/cancel   — cancel pending items
```

### 3.7. End-to-end workflow

1. Caller submits brief (AI chat or Admin Studio).
2. `CampaignService.CreateCampaign` persists the campaign and validates the persona pool is large enough.
3. `Planner` calls the AI Gateway and returns N unique items.
4. `content_campaign_items` are inserted with `status = 'planned'`.
5. If `run_immediately`, `RunCampaign` creates a Studio batch and enqueues `studio:run_batch`.
6. Existing `content:generate_draft` tasks create drafts, using each item's assigned persona as the author.
7. Each draft is a `contents` row with `status = 'draft'` and a `content_reviews` row for HITL.
8. Admin approves in the HITL queue; the draft is published.
9. Studio review hook updates the corresponding `content_campaign_item` to `published`.

### 3.8. Error handling and limits

- **Persona pool too small:** return `400 persona_pool_too_small` with the required and available counts.
- **Plan parse failure:** campaign → `failed`, `error_message` set.
- **Plan validation failure (reused persona, unknown persona, invalid content type):** campaign → `failed`, retryable by the user.
- **Generation failure:** item → `failed`; the campaign remains `generating`; user can retry the individual item.
- **Concurrency:** reuses `studio.max_concurrent_jobs` through the existing batch pipeline.
- **Count cap:** `campaign.max_count` system setting, default 50, max 100.
- **Daily budget:** the planning call is counted against `agent.daily_budget_usd` when invoked through the agent.

## 4. JEOS / catalog updates

- **New section:** `sections/104-content-campaigns.md` (or extend §75).
- **New migrations:**
  - `000XXX_content_campaigns.up.sql` — create tables and indexes.
  - `000XXX_seed_campaign_rbac_nodes.up.sql` — add RBAC nodes.
  - `000XXX_seed_campaign_settings.up.sql` — add `campaign.max_count` and `campaign.enabled`.
  - `000XXX_seed_campaign_ai_tools.up.sql` — add `personas.create` and `content.campaign.create` to `ai_tools`.
- **New RBAC nodes:**
  - `campaign.create`
  - `campaign.view`
  - `campaign.manage`
  - `campaign.run`
  - `campaign.retry`
  - `campaign.cancel`
- **New system_settings keys:**
  - `campaign.enabled` (default `true`)
  - `campaign.max_count` (default `50`)
- **New Valkey keys:**
  - `jc:campaign:lock:{campaign_id}` — prevents concurrent `RunCampaign` calls.
  - `jc:campaign:plan:{campaign_id}` — idempotency guard for the planning step.
- **Asynq tasks:** reuses `studio:run_batch` and `content:generate_draft`; no new task required for the first version.
- **Permission for auto-provisioning:** uses existing `persona.manage` in the first version. A future `campaign.provision_personas` node can be introduced to separate campaign persona creation from full persona management.
- **Service dependencies:** `internal/campaign.Service` depends on `internal/personas.Service` (for provisioning) and `internal/studio.Service` (for batch creation).
- **Catalogs:** update `api-routes-catalog.md`, `valkey-keys-catalog.md`, `system-settings-catalog.md`, `permission-key-naming.md`, `asynq-tasks.md`.

## 5. Phased rollout

| Phase | Scope |
|-------|-------|
| 1 | Migrations, `content_campaigns`/`content_campaign_items` schema, `internal/campaign/store` and `gen.Queries`. |
| 2 | `CampaignService`, `Planner` (`PlanPersonas` + `PlanContent`), persona-uniqueness validation, auto-provisioning via `personas.Service.ProvisionPersonas`, unit tests. |
| 3 | HTTP handlers, RBAC nodes, system settings, `personas.create` and `content.campaign.create` tool wiring. |
| 4 | Admin Studio UI: campaigns list, create form, detail view, run/retry/cancel actions. |
| 5 | Agent tools: `personas.create` and `content.campaign.create` handlers, `ai_tools` migration, executor pipeline, system prompt instructions. |
| 6 | Integration: Playwright check of Admin Studio, `go test ./internal/campaign/...`, agent chat end-to-end test. |

## 6. Testing

- Unit tests for `Planner` prompt construction, persona spec generation, and persona assignment.
- Unit tests for persona-uniqueness enforcement and auto-provisioning (success, reused persona, pool too small, permission denied).
- `httptest` handler tests for create, run, retry, cancel.
- Integration test with mocked AI Gateway returning valid/invalid plans.
- Verify every generated draft is created with `status = 'draft'` and a corresponding `content_reviews` row.
- `go build ./...`, `go vet ./...`, `go test ./internal/campaign/...`.

## 7. Open questions / risks

1. **Persona discovery for the agent:** The agent needs to know which personas are available. Should the planner load them directly from the `personas` store, or should we add a `personas.list` tool first? For the first version, the service loads them internally; a future `personas.list` tool can make this more transparent.
2. **Persona auto-provisioning permission:** Using `persona.manage` for campaign auto-provisioning may be too broad. A dedicated `campaign.provision_personas` node is preferred for a future release.
3. **Plan quality at high count:** A campaign of 50 unique topics + 50 unique personas is a large structured-output request. We may need to chunk planning into batches if the model struggles.
4. **Planning latency/timeouts:** `CreateCampaign` calls the planner synchronously. A large campaign may exceed the HTTP timeout; if this becomes an issue, the planning step can be moved to a background `campaign:plan` Asynq task.
5. **HITL queue volume:** 20–100 drafts at once could flood the content review queue. We may need a `campaign.throttle` setting or per-campaign max in the first release.
6. **Mixed content types:** If `content_type` is omitted, the planner must choose per item. We need clear validation of which types are eligible for campaigns.

## 8. Non-scope

- This design does **not** auto-publish campaign drafts. Every piece goes to HITL review, consistent with §75 and the user’s requirement.
- This design does **not** allow persona reuse within a campaign. `allow_persona_reuse` is deferred.
- This design does **not** replace the existing `studio:run_batch` or `content:generate_draft` tasks; it orchestrates them.
