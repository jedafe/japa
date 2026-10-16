---
name: japa-docs-author
description: Use when writing or updating engineering documentation in the repo `docs/` directory (NOT the apps/docs SvelteKit site — that's jc-docs-*). Grounds every claim in `.jeos/` spec files (sections/, conventions/, adr/, schemas/, .ai/) and in shipped code — routes, migrations, handlers — so documentation is accurate rather than remembered. Triggers: "write docs for X", "document the X feature", "create documentation in /docs", "add a doc explaining X", "update docs/".
---

# JapaConnect Docs Author — Repo `docs/` Writer

Writes engineering documentation into the repo `docs/` directory. Ground truth is **`.jeos/` (the spec) verified against shipped code** — never write from memory. JapaConnect has ~158 tables, ~819 routes, 118 section files; your training data does not contain them. The files do.

## ⛔ LOAD-ORDER GATE

Before writing a single line of a doc:

1. **`.jeos/JEOS.md`** — the master index. Find which sections/conventions/ADRs own the thing you're documenting.
2. **`.jeos/.ai/*.yaml`** — orientation layer: `project.yaml`, `architecture.yaml`, `stack.yaml`, `domains.yaml`, `glossary.yaml`, `conventions.yaml`. These are the machine-readable "how the site is built" summary.
3. **`.jeos/.ai/context/implementation-phase.md`** — current project STATE (what version, what shipped). Transient — never cite it as spec.
4. **The specific `.jeos/sections/NN-*.md` files** for the feature — the HOW.
5. **Relevant `.jeos/conventions/*.md`** — `api-routes-catalog.md`, `system-settings-catalog.md`, `valkey-keys-catalog.md`, `permission-key-naming.md`, `asynq-tasks.md` for any route/key/task/setting you name.
6. **Verify against code** — spec can drift from shipped code (JEOS.md says so itself). Spot-check: does the route exist in `internal/server`? Does the table exist in `migrations/`? Does the permission node exist in seed data? If spec and code disagree, **document the code** and flag the drift explicitly in the doc ("JEOS §NN says X; shipped code does Y — drift flagged").

Writing docs from memory or from the spec alone, unverified, is a violation — same spirit as Rule 1/Rule 16.

## Where docs go

| Content | Location | Naming |
|---|---|---|
| Foundational principles, constitution, architecture philosophy | `docs/00-foundation/` | `UPPER_SNAKE.md` |
| Feature/design docs, guides, audits | `docs/` root or a topic dir | `kebab-case.md` |
| Implementation plans | `docs/superpowers/plans/` | `YYYY-MM-DD-<slug>.md` |
| Feature specs/design docs | `docs/superpowers/specs/` | `YYYY-MM-DD-<slug>-design.md` |

Match the existing neighbors' structure. `docs/superpowers/` files are date-prefixed — always use today's date.

## Doc conventions

- **Cite sources.** Reference the owning spec in-line: `Ref: .jeos/sections/44-documentation-system.md`, `ADR-069`, `migrations/000735_baseline_schema.up.sql`. A doc a reviewer can't trace back to a spec/code is a guess.
- **Real names only.** Route paths must match `conventions/api-routes-catalog.md` (or actual handler registrations). Table/column names must match `migrations/`. Valkey keys must match `valkey-keys-catalog.md` (`jc:` prefix). Asynq task names must match `sections/11-job-queue-asynq.md`. Permission nodes must match `sections/03-rbac.md` / seed data.
- **Current truth markers.** Note the JEOS version (`JEOS.md` header) and date at the top of substantial docs; mark anything version-gated (v1 vs v2, roadmap status).
- **Rule 17 — no AI attribution.** Never state or imply AI authorship anywhere in the doc.
- **Rule 18 — bun** for any JS/TS tooling commands you document (`bun`, `bunx`, `bun run` — never npm/npx/pnpm/yarn).
- **Honest status.** If a feature is spec-complete but unshipped, say so. Never document aspirational behavior as live behavior.

## Structure for feature docs

1. What it is (one paragraph — user-facing purpose)
2. Where it lives (surfaces, routes, `internal/` package, key tables)
3. How it works (data flow; the JEOS reference trail)
4. API surface (routes table — verified against catalog/code)
5. Configuration (feature flags via `features.Registry` → Control → Extensions; `system_settings` keys; env vars)
6. Permissions (RBAC nodes that gate it)
7. Background work (Asynq tasks, Valkey keys)
8. Known limits / drift notes / open questions

## What NOT to do

- Do NOT document a route, table, setting key, Valkey key, or task name you haven't verified exists in the catalog or the code.
- Do NOT copy spec prose wholesale — a doc explains the *shipped* system; the spec already exists in `.jeos`.
- Do NOT put public/user-facing guides here — user docs live in the docs *system* (`apps/docs` + `docs.*` agent tools), not the repo.
- Do NOT create README/index churn — add files where the existing tree puts them; update `docs/README.md` only if it indexes siblings.
- Do NOT leave `// QUESTION:`-style gaps silently — if a spec is ambiguous, document what's verifiable and flag the gap.

## Deliverables

1. The doc file(s) in the correct location per the table above.
2. Every factual claim traceable to a `.jeos` file or shipped code (Ref comments/links).
3. Drift flags where spec ≠ code, stated plainly in the doc.
4. No AI attribution, bun-only tooling commands, catalog-exact identifiers.
