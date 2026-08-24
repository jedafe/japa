# Content Campaigns Implementation Plan

> Generated from design spec: `docs/superpowers/specs/2026-08-24-content-campaigns-design.md`

## Goal

Implement AI Content Campaigns: a multi-persona content orchestration layer that turns one brief into N unique content pieces, each authored by a different AI persona, with automatic persona provisioning and HITL review.

## Global Constraints

- JEOS first: create `.jeos/sections/104-content-campaigns.md` and update catalogs before code.
- Reuse existing `content:generate_draft`, `studio:run_batch`, `content_reviews`, and `personas.Service.ProvisionPersonas`.
- Use `bun` for all JS/TS tooling; use `go` for backend.
- All new Valkey keys use `jc:` prefix and are catalogued.
- All new RBAC nodes are seeded completely.
- API errors use the standard `errpkg.WriteError` shape.

## Task 1: JEOS and Catalogs

**Files:**
- Create: `.jeos/sections/104-content-campaigns.md`
- Modify: `.jeos/JEOS.md`
- Modify: `.jeos/conventions/api-routes-catalog.md`
- Modify: `.jeos/conventions/valkey-keys-catalog.md`
- Modify: `.jeos/conventions/system-settings-catalog.md`
- Modify: `.jeos/conventions/permission-key-naming.md`
- Modify: `.jeos/sections/11-job-queue-asynq.md` if adding `campaign:plan` task
- Modify: `.jeos/sections/03-rbac.md` if adding `campaign.provision_personas`

- [ ] Step 1: Write `sections/104-content-campaigns.md` with schema, routes, tool schemas, and workflow.
- [ ] Step 2: Add §104 to `JEOS.md` index.
- [ ] Step 3: Add `campaign.*` permission keys, API routes, system settings, and Valkey keys to their catalogs.
- [ ] Step 4: Commit.

## Task 2: Migrations

**Files:**
- Create: `migrations/000XXX_content_campaigns.up.sql`
- Create: `migrations/000XXX_content_campaigns.down.sql`
- Create: `migrations/000XXX_seed_campaign_rbac.up.sql`
- Create: `migrations/000XXX_seed_campaign_settings.up.sql`
- Create: `migrations/000XXX_seed_campaign_ai_tools.up.sql`

- [ ] Step 1: Create `content_campaigns` and `content_campaign_items` tables with indexes.
- [ ] Step 2: Seed `campaign.create`, `campaign.view`, `campaign.manage`, `campaign.run`, `campaign.retry`, `campaign.cancel` RBAC nodes.
- [ ] Step 3: Seed `campaign.enabled` and `campaign.max_count` system settings.
- [ ] Step 4: Seed `personas.create` and `content.campaign.create` `ai_tools` rows.
- [ ] Step 5: Commit.

## Task 3: Backend — `internal/campaign`

**Files:**
- Create: `internal/campaign/types.go`
- Create: `internal/campaign/store.go`
- Create: `internal/campaign/planner.go`
- Create: `internal/campaign/service.go`
- Create: `internal/campaign/handlers.go`
- Create: `internal/campaign/service_test.go`
- Create: `internal/campaign/planner_test.go`
- Create: `db/queries/campaigns.sql`
- Modify: `internal/db/gen` (regenerate via `make sqlc-gen`)

- [ ] Step 1: Define `Campaign`, `CampaignItem`, `CreateCampaignInput`, `CampaignItemPlan`, `PersonaSpec` types.
- [ ] Step 2: Implement SQL queries for campaigns and items (insert, list, get, update status).
- [ ] Step 3: Run `make sqlc-gen` to generate `gen` package code.
- [ ] Step 4: Implement `Store` with transaction support.
- [ ] Step 5: Implement `Planner.PlanPersonas` and `Planner.PlanContent` with AI Gateway calls.
- [ ] Step 6: Implement `CampaignService.CreateCampaign` with pool validation and auto-provisioning.
- [ ] Step 7: Implement `CampaignService.RunCampaign`, `RetryItem`, `CancelCampaign`, `List`, `Get`.
- [ ] Step 8: Implement HTTP handlers for `POST /admin/campaigns`, `GET /admin/campaigns`, `GET /admin/campaigns/:id`, `POST /admin/campaigns/:id/run`, `POST /admin/campaigns/:id/retry`, `POST /admin/campaigns/:id/cancel`.
- [ ] Step 9: Add `CampaignService` wiring in `internal/server/server.go`.
- [ ] Step 10: Commit.

## Task 4: Backend — Agent Tools

**Files:**
- Modify: `internal/ai/tools_register.go`
- Create: `internal/ai/tools_personas.go`
- Create: `internal/ai/tools_campaign.go`
- Modify: `migrations/000XXX_seed_campaign_ai_tools.up.sql`

- [ ] Step 1: Implement `personas.create` tool handler.
- [ ] Step 2: Implement `content.campaign.create` tool handler.
- [ ] Step 3: Register handlers in the agent tool registry.
- [ ] Step 4: Commit.

## Task 5: Admin Studio UI

**Files:**
- Modify: `apps/admin/src/routes/(app)/studio/+page.svelte`
- Create: `apps/admin/src/routes/(app)/studio/campaigns/+page.svelte`
- Create: `apps/admin/src/lib/api/campaigns.ts`
- Modify: `apps/admin/src/lib/i18n/en.json`

- [ ] Step 1: Add “Campaigns” tab to the Studio page.
- [ ] Step 2: Create campaigns list page with server-side pagination, status, and counts.
- [ ] Step 3: Create campaign create form: prompt, content type, count, persona pool, auto-provision, auto-assign, target space, target tags, run immediately.
- [ ] Step 4: Create campaign detail view with item table and Run/Retry/Cancel actions.
- [ ] Step 5: Add API client for campaigns.
- [ ] Step 6: Add i18n keys.
- [ ] Step 7: Commit.

## Task 6: Integration and Testing

- [ ] Step 1: `go build ./...` and `go vet ./...`
- [ ] Step 2: `go test ./internal/campaign/...`
- [ ] Step 3: `bun run check` in `apps/admin`
- [ ] Step 4: Manual end-to-end test in Admin Studio and AI chat.
- [ ] Step 5: Verify every generated draft lands in `content_reviews`.
- [ ] Step 6: Commit.
