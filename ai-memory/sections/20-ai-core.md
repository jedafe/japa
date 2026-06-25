# Section 20 — AI Core

**Status:** ✅ Locked

## Decided
Multi-provider AI Gateway (Anthropic, OpenRouter, OpenAI, Google Gemini, Ollama), task-based routing, circuit-breaker fallback. Tool Registry, RBAC-gated per User Group. Honcho (Plastic Labs) self-hosted for AI user context/persistent memory — own database, same PostgreSQL 18.x instance as the main app (`honcho` database alongside the main app's, not a second Postgres server; Honcho manages its own internal schema/migrations independently of our sqlc/migrate pipeline). HITL pipeline (generate → review → compile → deploy) mandatory before publishing high-stakes (immigration/financial) AI-generated content. pgvector for embeddings, no separate vector DB.

## Amendments (post-audit)
- `ai.context.*` permission node family (Section 03, ADR-025) gates what feeds Honcho's context per User Group.
- Admin Panel (Section 22) and Control Panel (Section 21) gained Honcho visibility — per-user content panel and infra health respectively. See ADR-027.
- Erasure (Section 19, ADR-014) now calls Honcho's deletion API as part of the erasure job.

## V1 Tool Catalog (itemized during the final V1 audit — previously a known gap)
- Content agent: `content.generate_draft`, `content.editor_assist` (behind `ai.editor.assist`), `content.suggest_tags`
- Moderation agent: `moderation.flag_content`, `moderation.classify_severity`
- Platform agent: `platform.translate_string`
- Analytics agent: no tools registered in v1 — explicit v2/v3 placeholder, not an oversight; every v1 analytics dashboard is deterministic SQL, not AI-driven.

## See Also
ADR-010, ADR-002, ADR-025, ADR-027.
