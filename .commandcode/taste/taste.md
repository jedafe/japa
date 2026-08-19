# Taste (Continuously Learned by [CommandCode][cmd])

[cmd]: https://commandcode.ai/

# git
- Stage and commit ALL modified files (including submodule pointer changes), not just task-specific ones, when running `git add`. Confidence: 0.75

# audit
- When fixing audit findings, verify the audit file for remaining ⚠️ and ❌ items before declaring all findings resolved. Confidence: 0.80
- Mark fixes as "**Already Fixed**" in the audit file, not in code comments. Confidence: 0.70

# ui
- Admin data tables should display human-readable entity names (organization names, user display names, etc.) rather than raw UUIDs or entity IDs — resolve names via backend enrichment, not frontend display tricks. Confidence: 0.85
- LayerChart is part of the admin panel tech stack and should be included in skill/architecture documentation for apps/admin/. Confidence: 0.65
- UI labels for the same data values (e.g., statuses) must be consistent across all elements on a page — dropdown options, filter tabs, badges, and table cells should all use the same names (e.g., "Published/Flagged/Removed", not "Approved/Pending/Rejected" in one place and different names elsewhere). Confidence: 0.85
- Admin moderation flows should distinguish soft delete (set status to 'removed') from hard delete (permanent DB deletion). "Hard Delete" action should only appear for items already in the 'removed' state; other states show "Delete" which soft-deletes. Confidence: 0.85

# testing
- Use Playwright for end-to-end testing in this project. Confidence: 0.65

# communication
- Present bug hunt results as a structured table with columns: #, Section (with code section reference), Bug description, Fix description. Include a summary header with counts (found/fixed/remaining), a verification results section, and a files-changed list. Confidence: 0.75
- When asked to arbitrate implementation decisions (e.g., contradictions in a plan), the user delegates the call to the assistant ("which yields the best results") rather than picking an option — make the judgment call, document the ruling, and proceed. Confidence: 0.70

# data
- Use Wikidata SPARQL for university ranking data instead of paid APIs. Confidence: 0.50

# audit
- When fixing audit findings, don't mark issues as P5/DEFERRED — actually implement the fixes. Confidence: 0.65

# i18n
- All user-facing strings must use `t()` with proper i18n keys — no hardcoded strings in code. Manage translations via the admin panel (not by editing JSON files directly). Confidence: 0.95
- New locales can be created by copying all keys from `en` via the admin panel's "Add Language" feature (batch-upserts to backend). Confidence: 0.80
- Missing i18n keys in catalog can be auto-detected by scanning for t('key') calls not in en.json — add key:value to en.json and sync fr.json. Confidence: 0.70
- Auto-wrap hardcoded markup text nodes safely by guarding against Svelte block expressions ({#if, {#each), JS operators (&&, ||), and script blocks. Confidence: 0.75

# workflow
- When encountering orphaned/disconnected code (unwired tasks, services, etc.), wire them up instead of deleting — don't remove anything that will be useful. Confidence: 0.75
- Confirm with user before removing any dead code — do not delete without explicit approval. Confidence: 0.85
- Use a todo list (todo_write/todo_list) to track multi-step tasks. The user says terse "continue" to keep progress flowing and expects the assistant to know what comes next from the list. Confidence: 0.80

# permissions
- Super admin groups should be granted the `*` wildcard permission rather than maintaining explicit permission key mappings. Confidence: 0.65

# backend
- When frontend has features the backend doesn't support yet (e.g., settings fields, wallet capabilities), wire up backend support rather than stripping the frontend. Confidence: 0.80

# svelte
See [svelte/taste.md](svelte/taste.md)
# api
- When backend API response field names don't match frontend TypeScript types, create a mapping/transform function rather than modifying the frontend type to match raw API fields. The frontend type should represent the ideal shape; the mapper normalizes the raw response. Confidence: 0.75
- The `apiFetch`/`api` client automatically unwraps the top-level `data` field from backend responses. When returning both collection data and metadata (e.g., pagination totals), nest them: `{"data": {"reviews": [...], "total": N}}` not `{"data": [...], "total": N}` — the latter loses `total`. Confidence: 0.90
- The `api()` helper already calls `JSON.stringify()` on the `body` — do NOT pass `JSON.stringify({...})` yourself or you get double-stringified (invalid JSON) errors. Pass a plain object. Confidence: 0.90
- API client `dispatch()` should read the response as text first (`res.text()`), then parse with `JSON.parse()` — never call `res.json()` directly. This gracefully handles empty bodies, server panics, and malformed responses without throwing. Confidence: 0.80

# workflow
See [workflow/taste.md](workflow/taste.md)
# bug-fixing
- When auditing and fixing bugs, fix ALL identified bugs — don't defer low-severity items without explicit user approval. The user expects comprehensive resolution, not prioritization. Confidence: 0.75
- Always reset loading/locking state variables (toggling, retrying, deleting, etc.) in `finally` blocks so they clear even on exceptions. Use try/catch/finally pattern for any async operation that sets state. Exception: Svelte 5 `$state` variables should NOT be set in `finally` — use explicit `try`/`catch` branches instead (see svelte section). Confidence: 0.70

# package-manager
- Use bun instead of pnpm for running/installing in the apps (SvelteKit frontends). Confidence: 0.65

# ui
- When DataTable column 'render' functions return HTML markup strings (e.g., `<span class="...">${v}</span>`), inject them as HTML using `{@html}` in the cell template rather than rendering them as text, otherwise the raw HTML tags will be displayed literally. Confidence: 0.70
- Make DataTable columns `sortable: true` by default when adding new columns (e.g., a Groups column on the users list) — users expect to sort columns like they can with other columns. Confidence: 0.70
- DataTables showing tab content (e.g., Activity, Sessions, AI Context, Warnings) should include pagination with configurable page size options (10/25/50/100), not use `showPagination={false}`. Confidence: 0.80
- Admin list pages (reviews, tickets, etc.) should include: (1) status filter tabs with active highlighting, (2) entity/type filter dropdown using Select component, (3) server-side Pagination component. Model new pages after the tickets page pattern. Confidence: 0.85
- Frontend filter option values (status, content_type, etc.) must match the actual database column values, not assumed or fabricated ones — verify against the backend schema/query before defining filter dropdown options. Confidence: 0.85

# mobile
- The monorepo includes a Flutter mobile app: `apps/mobile` (Flutter app) plus shared Dart package `packages/japaconnect_core`. Flutter SDK 3.47 stable / Dart 3.13 is installed at `~/flutter-sdk/flutter` and is NOT on PATH by default — prefix shell commands with `export PATH="$HOME/flutter-sdk/flutter/bin:$PATH"`. Confidence: 0.80

