# Section 01 — Foundation & Tech Stack

**Status:** ✅ Locked (reconstructed from carried-over memory — verify against original session notes if available)

## Decided
Go 1.26+ monolith — Chi router, sqlc, pgx/v5, Asynq. SvelteKit 2.57.x on Bun 1.3.x runtime across five apps: main site (SSR), admin SPA, control panel SPA, learning platform (SSR), docs site (SSR). PostgreSQL 18.x with pgvector. Valkey (BSD-licensed Redis-protocol-compatible fork — see ADR-021; chosen over Redis 8.x specifically to avoid that version's move to a tri-license). Meilisearch 1.45.x. Nginx. imgproxy. SeaweedFS. Mailpit for local dev mail only. Honcho (self-hosted, Section 20) runs as its own service with its own logical database (`honcho`) on the same PostgreSQL 18.x instance — one Postgres server, two databases, not a second Postgres deployment; Honcho's own internal migrations are independent of our sqlc/migrate pipeline. Docker Compose orchestration. `JC_` env var prefix, `jc:` Valkey key namespace.

Frontend UI stack: Tailwind CSS v4 (CSS custom properties / dark-mode class strategy), **Bits UI** (headless, accessible Svelte component primitives — Dialog, Dropdown, Tooltip, Select, Combobox, etc.), **Lucide Svelte** (icon set). Bits UI is the primary source of interactive component primitives across all three SvelteKit apps; bespoke components (Entity Card, Feed Card, Comparison Matrix, Ad Slot, etc.) are built on top of it, not alongside a second headless library.

VPS provider is not locked to any vendor — Hetzner was the original cost reference point but the stack is fully portable to any Docker-capable Linux VPS.

## See Also
ADR-001 (Go monolith), ADR-002 (pgvector), ADR-008 (Nginx over Caddy), ADR-019 (Bun over Node), ADR-021 (Valkey over Redis 8).
