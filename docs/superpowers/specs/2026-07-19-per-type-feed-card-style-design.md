# Design — Per-Content-Type Feed Card Style (Backend + Control)

**Date:** 2026-07-19
**Surfaces:** JapaConnect Go backend (`github.com/jedafe/japa`) + Control app (`apps/control`)
**Status:** APPROVED (2026-07-19) — awaiting final spec review
**Roadmap context:** Companion to the For-You feed frontend (`2026-07-18-for-you-feed-frontend-design.md`). That spec is **Layer 2** (frontend rendering); this spec is **Layers 1 & 3** (the setting contract + the admin UI) of one feature: letting a tenant assign a different feed card style per content type.

---

## 0. Purpose & decisions

Today `feed.card_style` is a **single scalar** per tenant (`"reddit"`), read in `internal/feed/handlers.go:GetCardStyle` and exposed by a single dropdown in `apps/control`. This spec upgrades it so an admin can map **each content type** to one of the four styles, with a fallback default.

| # | Decision | Choice |
|---|----------|--------|
| C1 | Storage shape | Extend the existing `feed.card_style` value from a scalar to a JSON object `{ default, by_type }`. No new key, no schema change (`system_settings.value` is already `jsonb`). |
| C2 | Backward compatibility | The reader tolerates the **legacy scalar** and normalizes it to `{ default: <scalar>, by_type: {} }`. Old data keeps working with or without the migration. |
| C3 | Endpoint contract | `GET /feed/card-style` returns `{ "data": { "default", "by_type" } }` (was `{ "card_style" }`). The only consumer is the not-yet-built feed frontend, so this is a safe break. |
| C4 | Type vocabulary | The admin UI enumerates types from the existing `GET /content-types` (dynamic, stays in sync); `default` covers any unmapped type. |
| C5 | Allowed styles per type | All four (`reddit`/`compact`/`grid`/`magazine`). `grid`'s mixed-feed layout is resolved entirely in Layer 2 (frontend), so no style needs to be forbidden here. |

**Rule 16:** the admin selector must demonstrably change what renders — that is the whole point of this feature; no dead controls.

---

## 1. Storage shape (Layer 1)

`system_settings` row: `key = 'feed.card_style'`, `value` (jsonb):

```json
{ "default": "reddit", "by_type": { "blog_post": "grid", "discussion": "reddit" } }
```

- `default` — a `CardStyle`, applied to any type without an override. Required; missing → `"reddit"`.
- `by_type` — `{ [content_type]: CardStyle }`. Absent/empty means "every type uses `default`."
- `CardStyle ∈ { reddit, compact, grid, magazine }`. Any unknown value is coerced to `default` (or `"reddit"` if that too is invalid).

No new column, index, or table — `system_settings.value` is `jsonb NOT NULL` (`migrations/000004`).

---

## 2. Backend reader + endpoint (Layer 1)

**Reader (in `internal/feed/handlers.go`, replacing the scalar read in `GetCardStyle`):** a small helper that resolves the setting into a normalized struct:

```go
type CardStyleConfig struct {
    Default string            `json:"default"`
    ByType  map[string]string `json:"by_type"`
}
```

Resolution logic (pure, unit-testable — keep it out of the handler):
1. Read raw `feed.card_style` jsonb. Absent → `{Default:"reddit", ByType:{}}`.
2. If the raw value is a **JSON string** (legacy) → `{Default: <string>, ByType:{}}`.
3. If a **JSON object** → decode into `CardStyleConfig`.
4. **Validate**: coerce any style (default or per-type) not in the allowed set to a safe value; drop `by_type` entries whose value is invalid; if `Default` is invalid → `"reddit"`.

**Endpoint:** `GET /api/v1/feed/card-style` (public, unchanged route/auth) now returns:

```json
{ "data": { "default": "reddit", "by_type": { "blog_post": "grid" } } }
```

The `card_style` scalar field is removed. (Grep confirms no built consumer of the old shape; the feed frontend is authored against the new shape.)

---

## 3. Migration (Layer 1)

Paired up/down, per `jc-backend-migrate` conventions (verify next migration number at build time). Reshapes the seeded scalar into the object form. Idempotent and guarded by `jsonb_typeof`.

**Up:**
```sql
UPDATE system_settings
SET value = jsonb_build_object('default', value, 'by_type', '{}'::jsonb),
    updated_at = now()
WHERE key = 'feed.card_style'
  AND jsonb_typeof(value) = 'string';
```

**Down:**
```sql
UPDATE system_settings
SET value = value->'default',
    updated_at = now()
WHERE key = 'feed.card_style'
  AND jsonb_typeof(value) = 'object'
  AND value ? 'default';
```

Because the reader (C2) is backward-compatible, this migration is hygiene rather than a hard dependency — the endpoint behaves correctly whether or not it has run. No `sqlc` regeneration needed (no query shape change).

---

## 4. Control panel admin UI (Layer 3)

Replace the single `feed.card_style` dropdown in `apps/control/src/routes/settings/+page.svelte` with a small mapping editor:

- **Default card style** — one select (all four styles). Writes `default`.
- **Per-type overrides** — one row per content type fetched from `GET /content-types` (dynamic; today `blog_post`, `discussion`, `comparison`, `page`). `page` is static and never appears in the feed, so mapping it is harmless/inert (kept for a uniform UI rather than special-casing). Each row: a select with options `[ "Use default", reddit, compact, grid, magazine ]`. "Use default" omits the type from `by_type`.
- On save, assemble `{ default, by_type }` (only non-"Use default" rows enter `by_type`) and persist via the existing settings update path (`UpdateSettings` → `system_settings`), keyed `feed.card_style`.

Existing i18n keys (`settings.card_style_reddit|compact|grid|magazine`) are reused; add keys for the section title, "Default", "Use default", and a one-line helper noting that in the mixed home feed a `grid` type renders as a full-width card (the Layer-2 behavior) so admins aren't surprised. Both `en`/`fr`.

---

## 5. Interaction with Layer 2 (frontend) — reference only

Not built here; recorded so the contract is unambiguous. The frontend (`2026-07-18-for-you-feed-frontend-design.md` §3A):
- resolves per item: `cardStyleFor(content_type) = by_type[content_type] ?? default`;
- renders the feed as a **2-col grid iff every item on the page resolves to `grid`**, else a single-column stream where a `grid` item is a full-width image-top card.

This is why Layer 1 need not forbid `grid` per type — mixing is handled by the frontend layout rule.

---

## 6. Testing & gates

- **Backend unit tests (Go, pure):** the resolver over — a legacy scalar; a full `{default, by_type}` object; a missing setting; an object with an invalid `default`; a `by_type` entry with an invalid style (dropped/coerced); an empty `by_type`.
- **Endpoint test:** `GET /feed/card-style` returns the normalized `{default, by_type}` for each of the above stored states; still public (no auth).
- **Migration:** up reshapes a seeded scalar to `{default, by_type:{}}`; down restores the scalar; both idempotent; run against seed data.
- **Control:** the mapping editor round-trips (load existing map → edit → save → reload shows the saved map); "Use default" removes the override.
- **Route catalog** updated if the endpoint's documented response shape is catalogued.
- **Rule 16 acceptance:** setting `blog_post → grid` in Control demonstrably makes blog posts render as grid/full-width cards in the feed (end-to-end with Layer 2).

---

## 7. Build sequence

1. **Layer 1** — resolver helper + `CardStyleConfig` + unit tests; update `GetCardStyle` endpoint to return `{default, by_type}`; migration (up/down). *(jc-backend-develop, jc-backend-migrate)*
2. **Layer 2** — frontend consumes the contract (its own spec) — can start once Layer 1's shape is merged, testable against a hand-set map.
3. **Layer 3** — Control mapping editor + i18n. *(jc-control-develop)* — last, since it only writes what Layer 1 reads.

---

## 8. Out of scope

- The feed rendering / layout rule itself (Layer 2 spec).
- V2 content types (`event`, `job_listing`, `service_listing`) — automatically supported once they appear in `GET /content-types`; no code change needed here.
- Per-space or per-user card style overrides (tenant-level only in v1).
- Cleaning up the stale `internal/admin/settings.go` `feed_card_style` (`["reddit","devto"]`) definition — noted as pre-existing drift; address separately.
