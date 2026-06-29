# AGENTS.md — JapaConnect Architecture Context

This is the canonical, cross-tool context file — read natively by Cursor, Codex, Copilot, Gemini CLI, and most other coding agents. Claude Code looks for `CLAUDE.md` specifically; that file is a thin pointer back to this one, so there's a single source of truth, not two documents that can drift apart.

Read this in full before starting any task. For why a decision was made, see `ai-memory/decisions/`. For implementation patterns and code snippets, see `ai-memory/skills/`. For current build status, see `ai-memory/roadmap-status.md` and `ai-memory/sections/`.

> **Note on Sections 1–8:** reconstructed from carried-over architectural memory, not verbatim original session notes. Verify against original records if they exist.

## Project Overview

JapaConnect is a community platform for the Nigerian diaspora and Nigerians planning to relocate abroad. Repository: `github.com/jedafe/japa`.

## Sudo Credential
- sudo credentials → `.sudo`

## Tech Stack — Pinned Versions

Pin these exactly in `go.mod`, `package.json`, and `docker-compose.yml`. Do not silently drift to "latest" in CI — bump deliberately and update this file in the same PR.

| Component | Version | Notes |
|---|---|---|
| Go | **1.26.x** (1.26.4 at time of writing) | Chi router, sqlc, pgx/v5, Asynq |
| Bun | **1.3.x** (1.3.14 at time of writing) | Runtime for all three SvelteKit apps |
| SvelteKit | **2.57.x** | TypeScript 6.0 support, remote functions |
| PostgreSQL | **18.x** (18.4 at time of writing) | pgvector extension. PG19 is in beta as of mid-2026 — do not use for production until GA. |
| Redis-protocol cache/queue layer | **Valkey 9.x** | BSD-licensed, AWS/GCP/Oracle cloud-default fork of Redis — see decision note below |
| Meilisearch | **1.45.x** | |
| Nginx | latest stable from distro/official repo | |
| imgproxy | latest stable | |
| SeaweedFS | 4.x (weed mini for dev) — Apache 2.0 | |
| Tailwind CSS | v4 | CSS custom properties for theming tokens |
| Bits UI | latest stable | Headless, accessible Svelte component primitives (Dialog, Dropdown, Tooltip, Select, etc.) |
| Lucide Svelte | latest stable |
| AutoAnimate | @formkit/auto-animate latest |
| Docker Compose | v2.x (Compose Specification) | |

### Resolved: Valkey Over Redis 8

Redis 8.x moved off BSD licensing to a tri-license (RSALv2 / SSPLv1 / AGPLv3). **Valkey** (the Linux Foundation's BSD-licensed fork, drop-in protocol-compatible) is the locked choice — see ADR-021. This was a clean call by the time it was actually decided: AWS, Google Cloud, and Oracle have all standardized their managed offerings on Valkey, it's surpassed 100M+ Docker pulls, and Valkey 9.x has caught up to or exceeded Redis 8 on the features this platform actually uses (cache, pub/sub, Asynq queue backing). BSD licensing removes the question entirely rather than requiring a legal judgment call on whether SSPL/AGPL's terms bind an internal-use deployment.

## Core Architectural Principles

1. **Dynamic over hardcoded.** RBAC, navigation, feature flags, language packs, and ad slots are admin-configurable registries, not hardcoded in code.
2. **Multi-tenancy from day one.** Every scoped table carries `tenant_id`.
3. **Path-based routing within a product surface; subdomains only for genuinely distinct surfaces.**
4. **Render-time over save-time processing** for affiliate links, mentions, and interlinks — resolve from stored entity IDs at render time, never bake in resolved text.
5. **HITL mandatory for high-stakes AI output** (immigration/financial content).
6. **AI code invisibility** — never label AI-assisted code as such in PRs/commits/comments.
7. **Provider abstraction everywhere external services are swappable** (payments, email, AI) — see the canonical interface shape below.
8. **Decisions get written down at the time they're made** (ADRs), not reconstructed later from memory.

## Canonical Code Pattern: Provider Abstraction

This exact shape repeats three times in this codebase (Payment Core, Email, AI Gateway). Every new provider implements this interface — never code business logic against a vendor SDK directly.

```go
package provider

import "context"

type Request struct {
    Category string // e.g. "transactional", "marketing", task-type for AI
    Payload  map[string]any
}

type Response struct {
    ProviderRef string
    Status      string
    Raw         map[string]any
}

type Provider interface {
    Name() string
    Send(ctx context.Context, req Request) (Response, error)
    VerifyWebhookSignature(payload []byte, sig string) error
}

// Dispatcher holds a priority-ordered list of providers per category
// and implements circuit-breaker fallback: on failure, try the next
// provider in order rather than failing the whole operation.
type Dispatcher struct {
    providersByCategory map[string][]Provider
}

func (d *Dispatcher) Dispatch(ctx context.Context, category string, req Request) (Response, error) {
    var lastErr error
    for _, p := range d.providersByCategory[category] {
        resp, err := p.Send(ctx, req)
        if err == nil {
            return resp, nil
        }
        lastErr = err
        // log failure for Control Panel's cost/health dashboards, then try next provider
    }
    return Response{}, lastErr
}
```

**Exception: Credit (Section 28) is not a `Provider`.** Payment Core's `credit` provider value has no external vendor, no `Send()` call, no webhook — it's a direct, synchronous ledger debit against the Wallet & Credits balance, in the same DB transaction as the `payments` row. Don't write a no-op `CreditProvider` to force it into this interface; see `ai-memory/skills/wallet-ledger-pattern.md`.

## Canonical Code Pattern: RBAC Permission Check

Never hardcode a role name. Always resolve through the dynamic permission system:

```go
// WRONG — hardcoded role check, breaks the dynamic RBAC model
if user.Role == "editor" {
    allowPublish()
}

// RIGHT — permission node resolved at runtime, admin-assignable to any group
if rbac.HasPermission(ctx, user, "blog.publish") {
    allowPublish()
}
```

Permission node naming: `{domain}.{action}` (e.g. `content.page_builder.create`, `ai.editor.assist`, `ai.context.points_balance`, `ai.context.admin_view`). Full convention in `ai-memory/conventions/permission-key-naming.md`.

## Canonical Code Pattern: Webhook Handler

Validate fast, process async — never run business logic synchronously inside the handler:

```go
func HandlePaymentWebhook(w http.ResponseWriter, r *http.Request) {
    event, err := provider.VerifyWebhookSignature(body, r.Header.Get("Signature"))
    if err != nil {
        w.WriteHeader(http.StatusBadRequest)
        return
    }
    if alreadyProcessed(ctx, event.ID) { // idempotency check
        w.WriteHeader(http.StatusOK)
        return
    }
    recordRawEvent(ctx, event)
    asynqClient.Enqueue(tasks.NewPaymentWebhookProcessTask(event.ID), asynq.Queue("critical"))
    w.WriteHeader(http.StatusOK) // return fast — providers retry aggressively on timeout
}
```

## Canonical Code Pattern: sqlc Query

```sql
-- name: GetContentFeedPage :many
SELECT id, title, content_type, field_values, created_at
FROM contents
WHERE tenant_id = @tenant_id
  AND status = 'published'
  AND (created_at, id) < (@cursor_created_at, @cursor_id)
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;
```

```go
// Generated param struct always uses pgtype.UUID, never uuid.UUID directly
type GetContentFeedPageParams struct {
    TenantID        pgtype.UUID
    CursorCreatedAt pgtype.Timestamptz
    CursorID        pgtype.UUID
    PageLimit       int32
}
```

## Canonical Code Pattern: TipTap Node Reference

Every mention/tag/interlink node stores an entity reference, never resolved text:

```json
{
  "type": "mention",
  "attrs": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "entity_type": "user",
    "label": "Chidinma O."
  }
}
```

`label` is a cached display value for editor UX only. Render-time always re-resolves from `id` + `entity_type` against current data, falling back to a "deleted content" placeholder if the entity no longer resolves — never a broken link, never an empty string.

## Canonical Code Pattern: Rate Limiting & File Upload Validation

Every new endpoint needs an assigned rate-limit tier at design time (tight/moderate/loose). Every file upload goes through MIME-by-content-inspection, a per-context size limit, and a ClamAV scan that blocks persistence until clear — never trust a client-reported file type or skip the scan for convenience.

```go
// Rate limiting — see ai-memory/skills/abuse-protection-patterns.md for the full pattern
ok, _ := RateLimit(ctx, rdb, "jc:ratelimit:ai_gateway:"+userID, dailyAIQuotaFor(userID), 24*time.Hour)

// Upload validation — see ai-memory/skills/file-upload-validation-patterns.md for the full pattern
detected := http.DetectContentType(fileHeaderBytes) // content inspection, never the Content-Type header
```

See ADR-022 (rate limiting) and ADR-023 (file upload validation) for the full rationale.

## Section Map (v1.0.0, all 45 sections locked)

Full detail per section lives in `ai-memory/sections/NN-slug.md`. See `ai-memory/roadmap-status.md` for the high-level status table. Sections 28–45 were added post-audit (28: Wallet & Credits from V2 planning; 29–45: Universal Ratings and 16 additional systems & Reviews and Generic Comparison System restored from cross-session audit — see ADR-028, ADR-029).

## Where To Go Next

- What this project is, full scope, foundation to production → `descriptions.md`
- Engineering judgment, build order, risk areas → `developer_review.md`
- Implementation patterns and code snippets → `ai-memory/skills/`
- Why a decision was made → `ai-memory/decisions/`
- Coding standards and hard rules → `RULES.md`
- Full section-by-section build detail → `ai-memory/sections/`
- Production-readiness audit findings → `REVIEW-v1.0.0.md`
- Release grouping with code examples → `roadmap/ROADMAP-v1.0.0.md` (single merged file — previously split across v1.1.0–v1.4.0, consolidated)
