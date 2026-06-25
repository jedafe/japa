# RULES.md — JapaConnect Coding & Operational Rules

Hard rules for any agent or developer working on this codebase. Where a rule conflicts with convenience, the rule wins. Pinned tech stack versions live in `AGENTS.md` — don't duplicate version numbers here, reference that file so there's one place to update them.

## Take Note
- I'm a non-coder. I won't read source code to verify your work. After any change, give me a 3-step manual test I can run in the browser.

- Before touching files, list every file you plan to modify. If the list is >5, stop and propose splitting the task.

- If three consecutive fix attempts fail, STOP. Propose: (a) revert, (b) what we know vs don't know, (c) a different approach.

- This project uses only [stack list]. Do not introduce new libraries, frameworks, or services without asking me first.

- Before writing code for any non-trivial change, explain in plain language what you understand the goal is and your planned approach. Wait for my 'go.'

## Never Do

**Never hardcode a role name or permission check.** Every access decision routes through the dynamic RBAC permission system.
```go
// WRONG
if user.Role == "editor" { allowPublish() }
// RIGHT
if rbac.HasPermission(ctx, user, "blog.publish") { allowPublish() }
```

**Never store raw card data on our own servers.** All card collection happens through Stripe Elements or Paystack's hosted popup — payments code only ever handles tokens/references, never PANs.

**Never label AI-assisted code as AI-generated** in PRs, commits, or code comments.

**Never bypass HITL review** for AI-generated immigration or financial content before publish.

**Never bake resolved display text into stored content.** Mentions, interlinks, and tag references store the entity ID and resolve the label at render time.
```json
// WRONG — raw text baked in, breaks if the user renames
{ "type": "text", "text": "@oldusername" }
// RIGHT — entity reference, resolved at render time
{ "type": "mention", "attrs": { "id": "uuid-here", "entity_type": "user", "label": "cached display only" } }
```

**Never write business logic directly against a specific provider's SDK** (Stripe, Resend, a specific AI vendor) outside its abstraction layer implementation. Business logic calls the `Provider` interface in `AGENTS.md`, not the vendor.

**Never process a webhook's business effects synchronously inside the HTTP handler.** Validate signature, record the raw event, enqueue an Asynq task, return fast — see the canonical webhook handler pattern in `AGENTS.md`.

**Never trust a client-uploaded file's extension or self-reported MIME type.** Validate server-side using actual content inspection (e.g. `net/http.DetectContentType` as a first pass, not the `Content-Type` header alone).

**Never auto-seed city-level Geo Spaces.** Country and state/province seed automatically from the canonical dataset; cities are admin-created on demand.

**Never hard-delete a user row on account erasure.** Anonymize PII in place on the same row; only physically delete files (avatars) and revoke tokens.

**Never use `localStorage`/`sessionStorage`** in any artifact or frontend code path — not supported in this environment's rendering surfaces.

**Never expose an endpoint without an assigned rate-limit tier.** Every new endpoint gets an explicit tier at design time (tight/moderate/loose — see `ai-memory/skills/abuse-protection-patterns.md`), not the application default by accident. AI Gateway calls specifically are capped by a hard per-user quota, not just the admin-facing budget alert — a single account shouldn't be able to run up real provider costs unnoticed.

**Never accept a file upload without server-side validation.** MIME type by content inspection (not the client header or extension), per-context size limit, and a clean ClamAV scan via `media_scan` before the file is persisted to MinIO — see ADR-023.

**Never update a Points or Credits balance as a column mutation.** Every change is a new row in the immutable `wallet_ledger` — see `ai-memory/skills/wallet-ledger-pattern.md`. Current balance is the latest row's `resulting_balance`, not a counter maintained separately.

**Never force Credit spend through the `Provider` interface.** It has no external vendor, no `Send()`, no webhook — it's a direct, synchronous ledger debit in the same transaction as the `payments` row. See Section 28 and ADR-024.

**Never write Points/Credits directly from feature code.** Emit a typed activity event with an idempotency key (`reference`); let the Activity Point Rules consumer resolve the rule and write the ledger entry. See `ai-memory/skills/wallet-ledger-pattern.md`.

**Never treat the erasure job's Honcho deletion call as best-effort.** It's a required step alongside the same-row PII wipe, not an optional cleanup — if it fails, the job retries via Asynq's standard retry policy rather than reporting erasure complete with Honcho data left behind. See Section 19 and ADR-027.

**Never automate Group membership changes based on Points/Karma.** Crossing a Points threshold must never trigger an automatic Group promotion — Group assignment is always a manual admin action. See Section 28, Section 03, and ADR-028.

**Never include emojis in code**

**Never use n+1 query**

**NEVER bypass safety checks or hardcode credentials:** Agents should be forbidden from exposing sensitive data, bypassing security protocols, or hardcoding API keys and passwords in scripts.

**NEVER execute destructive database actions:** Agents must be barred from altering or dropping data schemas without explicit human approval.

**NEVER proceed without user authorization on major changes:** For non-trivial modifications (such as production deployments or architectural refactoring), the agent must stop and ask for explicit confirmation.

**NEVER use placeholder or test data in committed code:** Agents should not ship placeholder image URLs, incomplete strings, or untested stubs to production.

**NEVER introduce unapproved libraries:** An agent should be restricted to a pre-defined tech stack and must never introduce new, unvetted packages or frameworks without permission.

**NEVER hallucinate error handling:** When faced with edge cases (empty lists, missing permissions), the agent must not invent arbitrary logic; it must be instructed to fail safely or ask for clarification.

**NEVER fall into endless loops of the same mistake:** If the agent triggers consecutive errors or failures, it must stop, flag the issue, and wait for human intervention rather than iterating blindly.

**Always write against the standard Valkey/Redis-protocol command set, never a vendor-specific extension.** This was deliberately kept generic while the Redis-vs-Valkey decision was open (now resolved — see ADR-021, Valkey is the locked choice) and remains good practice regardless, since it's what keeps the abstraction honest.

## Always Do

- Always namespace Valkey keys with `jc:` and environment variables with `JC_`.
- Always version cache keys by content's `updated_at` (or an explicit version field) rather than explicitly invalidating on every edit — let stale keys age out via TTL.
- Always run DB migrations as a distinct deploy step before the new app version serves traffic. Breaking schema changes use the two-phase pattern (deploy code supporting both shapes → migrate → deploy code using only the new shape).
- Always validate uploaded files server-side (MIME sniffing, size limits) and route them through the malware scan task before persisting to MinIO.
- Always log sensitive admin actions (impersonation, manual payment approval, erasure, language pack publish, moderation actions) to `audit_logs`.
- Always treat payment webhook delivery as the source of truth for payment status — never the client-side success callback.
- Always check idempotency (`event_id` already processed) before acting on any inbound webhook.
- Always fall back to English for any translation key missing a value in an active non-English language — never render a blank string.
- Always batch high-frequency, low-consistency-need writes (ad impressions/clicks, engagement score increments) rather than writing synchronously on every event.
- Always use `pgtype.UUID`, never `uuid.UUID` directly, in sqlc params and generated structs.
- Always use cursor pagination (`WHERE (created_at, id) < (@cursor_created_at, @cursor_id)`), never offset/page-number pagination.
- Always apply exponential backoff lockout on repeated failed login attempts, keyed by account and IP — see `ai-memory/skills/abuse-protection-patterns.md`.
- Always apply Activity Point Rule value changes forward-only — never retroactively rescore historical `wallet_ledger` entries when an admin edits a rule's point value.
- Always check the `reference` idempotency key before writing a `wallet_ledger` entry for an activity event or a Credit purchase — the same discipline as inbound webhooks.
- Always ask for clarifications if you run into a state of dilemma: Never assumes and come to conclusion on your own.
- Always isolate execution: Never run code directly on the host system. Always use a secure, sandboxed container (like Docker) or virtual environment.
- Always block destructive commands: Never execute high-risk commands (e.g., rm -rf /, drop database) without explicit, multi-factor human confirmation.
- Always scrub secrets: Never commit or expose API keys, passwords, tokens, or hardcoded credentials. Automatically route them to environment variables. 
- Always run vulnerability scans: Check all generated or modified code for known security flaws (like SQL injection or XSS) before suggesting it. 
- Always fix root causes: Never patch symptoms with quick hacks. Fix the underlying architectural flaw or bug logic.  
- Always run testing suites: Execute relevant unit and integration tests after every modification to ensure existing features do not break. 
- Always follow style guides: Adhere strictly to the project's established linting, formatting, and language-specific conventions (e.g., PEP 8 for Python). 
- Always review side effects: Analyze how a code change impacts dependencies, database schemas, performance, and memory usage before implementing it.
- Always explain code changes: Provide a clear, high-level summary of exactly what the code does and why the change is necessary.
- Always document public interfaces: Write precise docstrings, type hints, and comments for all new functions, classes, and APIs.
- Always state missing context: Stop and ask the developer immediately if project requirements, architecture guidelines, or environment details are unclear.
- Always use parameterized queries: Never concatenate strings for database operations to block SQL injection in Golang.
- Always match migration schemas: Verify Postgres schema changes align perfectly with Meilisearch indexes to prevent search synchronization failures.
- Always set cache TTLs: Assign explicit expiration times for all Valkey keys to avoid out-of-memory crashes.
- Always run localized resets: Isolate database test cleaning to specific tables instead of dropping the entire database.
- Always handle Go errors: Never use blank identifiers (_) for error returns; log and handle every error explicitly.
- Always sign imgproxy URLs: Generate secure, cryptographically signed URLs in Golang to prevent denial-of-service image processing attacks.
- Always validate Nginx syntax: Run nginx -t inside a test container before applying configuration changes to reverse proxies.
- Always close network bodies: Defer resp.Body.Close() immediately after checking errors in Go HTTP requests to prevent memory leaks.
- Always isolate Svelte server-code: Use .server.ts modules to guarantee sensitive API keys never leak to the client browser.
- Always use Bun lockfiles: Keep bun.lockb synchronized across environments to ensure exact dependency version parity.
- Always compile Tailwind cleanly: Use official Tailwind directives and avoid arbitrary, un-scannable inline utility generation that breaks purging.
- Always write post implementation documentation of each section in the ai-memory/implementation/documentation folder after completing each Phase
- Always write implementation Skill for each section in ai-memory/implementation/documentation/skills folder after completing each Phase
- Always write documentation for each feature, bugs, and recommendations in their respective directories (i.e. ai-memory/implementation/bugs, etc) after completing each Phase


## Conventions Quick Reference

See `ai-memory/conventions/` for full detail on each:
- `error-handling-go.md`
- `api-response-format.md`
- `database-migration-naming.md`
- `permission-key-naming.md`
- `feature-flag-naming.md`
- `svelte-component-patterns.md`

## PR / Commit Discipline

- Reference the relevant `ai-memory/sections/NN-*.md` file in the PR description when a change touches a planned section's scope.
- If a PR introduces a pattern not yet captured in `ai-memory/skills/`, add the skill file in the same PR.
- If a PR fixes a non-obvious bug, add an entry to `ai-memory/bugs/` describing the root cause, not just the fix.
- If a PR makes an architectural decision not already covered by an ADR, add the ADR in the same PR.
- If a PR bumps a pinned dependency version, update the version table in `AGENTS.md` in the same PR — never let the two drift.
