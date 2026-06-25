# Section 44 — Documentation System

**Status:** ✅ Locked

## Architecture Decision

Documentation lives at `docs.japaconnect.com` as a fifth SvelteKit app (SSR). Content is generated and maintained by AI agents (Section 20's `platform_agent`) with mandatory HITL review before publication — same pipeline as all AI-generated content. Access to each documentation section is RBAC-gated, consistent with all other permission decisions.

## Documentation Audiences & Access

| Audience | Path | RBAC Gate |
|---|---|---|
| Public / Frontend Users | `/docs/users` | `public.view` |
| Premium Members | `/docs/premium` | `membership.pro` |
| Organizations / Agents | `/docs/organizations` | `organizations.member` |
| Moderators | `/docs/moderators` | `moderation.queue.view` |
| Developers / API | `/docs/api` | `public.view` (API docs are public) |
| Admins | `/docs/admin` | `admin.panel.access` |
| Super Admin | `/docs/superadmin` | `super_admin` group |

The API reference at `/docs/api` is fully public — no auth required. It's the platform's OpenAPI/Swagger-style reference.

## Schema

```sql
CREATE TABLE doc_pages (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  title           text NOT NULL,
  slug            text NOT NULL,
  audience        text NOT NULL,   -- matches audience keys above
  content         jsonb NOT NULL DEFAULT '{}', -- TipTap JSON
  ai_generated    boolean NOT NULL DEFAULT false,
  review_status   text NOT NULL DEFAULT 'draft',
  -- 'draft'|'in_review'|'approved'|'published'
  last_reviewed_by uuid REFERENCES users(id),
  last_reviewed_at timestamptz,
  parent_id       uuid REFERENCES doc_pages(id),
  position        int NOT NULL DEFAULT 0,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE(tenant_id, audience, slug)
);
```

## AI Generation Workflow

```go
// platform_agent tool: docs.generate_page
// Input: audience, topic, related_features[]
// Output: TipTap JSON draft → HITL review queue → published on approval
// Same mandatory review pipeline as all other AI-generated content
func GenerateDocPage(ctx context.Context, req DocGenRequest) error {
  draft := aiGateway.Generate(ctx, "platform_agent", "docs.generate_page", req)
  return createDocPage(ctx, DocPage{
    Content:     draft,
    AIGenerated: true,
    ReviewStatus: "in_review",
  })
}
```

## API Playground

Interactive API explorer at `/docs/api/playground` — a SvelteKit component that lets developers execute real API calls against the platform with their API key. Uses the platform's own OpenAPI spec generated from Go route annotations.

## See Also

Section 20 (AI platform_agent, HITL pipeline), Section 42 (feature flags for docs sections). ADR-044.
