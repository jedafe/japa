# List Content Type (BuzzFeed-Style Listicle)

**Date:** 2026-09-28
**Scope:** New built-in content type `list` — backend registration, permission gate, composer builder, public route family
**Refs:** .jeos/sections/14-content-type-system.md, .jeos/sections/03-rbac.md, .jeos/sections/45-post-create-page.md, .jeos/sections/106-video-content-and-import.md, .jeos/conventions/system-settings-catalog.md, .jeos/conventions/api-routes-catalog.md

## Background

JapaConnect needs a BuzzFeed-style listicle format ("Top 10 X" posts) as a
first-class content type titled **List**. Members of authorized user groups
compose numbered, countdown (ranked), or unnumbered lists whose items can carry
a heading, rich text, an uploaded image, an embed (internal JapaConnect content,
external URL, or platform video), and a source/credit line.

Creation is gated by a new RBAC node `list.create` (seeded, granted to
`super_admin` only — admins opt user groups in via the RBAC admin UI) and by the
public system setting `list.enabled`. There is no review queue: permitted
authors publish directly like `blog_post`/`guide` authors.

## Architecture

The type follows the §14 behavior-pack pattern: a `contents` row with
`content_type = 'list'` and structured item data in `field_values` JSONB.
`body` / `body_html` carry the fully serialized listicle so every generic
pipeline (content hash, read time, Meilisearch, feeds, excerpts, RSS, digests,
JSON-LD) works unchanged. `field_values` is the composer's editing source of
truth; `body_html` is the render source, byte-for-byte identical in kind to
other content types.

### Data model

`field_values` for `content_type = 'list'`:

```jsonc
{
  "list_mode": "numbered" | "countdown" | "unnumbered",
  "items": [
    {
      "heading": "optional string",
      "body_html": "optional sanitized rich text",
      "media_id": "uuid | null",          // uploaded image — XOR with embed
      "embed": {                          // null when media_id is set
        "kind": "content" | "url" | "video",
        "content_id": "uuid",              // kind=content: internal contents.id
        "content_slug": "string",          // kind=content: slug for hydration
        "snapshot": {                      // kind=content: static card fallback
          "title": "string", "excerpt": "string",
          "image_url": "string", "content_type": "string"
        },
        "url": "https://…",                // kind=url: external link
        "resolved": {                      // kind=url: /link-preview snapshot
          "title": "string", "description": "string",
          "image_url": "string", "site_name": "string",
          "embed_url": "string"            // oEmbed iframe src when supported
        },
        "video_source_id": "uuid"          // kind=video: video_sources.id
      },
      "source_url": "optional credit link",
      "credit": "optional credit text"
    }
  ]
}
```

- Item order in the array is canonical; `countdown` renders positions N→1.
- One visual slot per item: `media_id` **or** `embed` (mutually exclusive).
- `body` = intro text + plain-text serialization of all items (headings and
  item text) so `content_hash` (Rule 12), `read_time_minutes`, and search
  indexing cover the whole listicle.
- `body_html` = intro + rendered item markup using the platform's existing
  placeholder conventions (see below). Sanitized by `SanitizeHTML` like every
  other type.
- `content_revisions` snapshots `field_values` automatically — item history is
  free.

### `body_html` placeholder conventions

Item markup is plain sanitized HTML. Embeds use placeholder divs hydrated
client-side on view pages, mirroring §15/§106:

| Embed kind | Placeholder | Hydration |
| :--- | :--- | :--- |
| Uploaded image | `<img src="…">` (resolved URL) | none — static |
| External URL | `div[data-type="link-preview"][data-url]` | existing `hydrateLinkPreviews` |
| oEmbed platform (youtube/vimeo/codepen/ted) | `<iframe src="approved-embed-url">` | none — allowed by `isApprovedIframe` |
| Platform video | `div.video-embed[data-video-source-id]` | existing `hydrateVideoEmbeds` |
| Internal content | `div[data-type="content-embed"][data-content-slug]` | **new** `hydrateContentEmbeds` |

Internal-embed placeholders contain a static snapshot card (title + excerpt +
image link) as children so no-JS crawlers and feed surfaces still render a
useful card; hydration replaces it with a live `ContentEmbedCard` fetched via
`GET /content/{slug}` (rich card: title, excerpt, image, author, type badge;
resolves slug_history redirects).

Item numbering is baked into markup by the composer
(`<span class="li-num">3.</span>` inside the item heading block), so numbered
lists keep their numbers with JS disabled and for SEO.

One attribute is added to `sanitize.go`'s `div` whitelist: `data-content-slug`
(`data-type` is already allowed). Precedent: `data-coupon-id` was added the
same way.

## Backend changes

1. **Type registration** (`internal/content/content.go`):
   `TypeList = "list"` added to the constants, `validContentTypes`, and
   `ContentTypeLabels` (`"List"`). No DB schema change — `content_type` is
   free text with no CHECK constraint. `GET /content-types` advertises it
   automatically; the admin content page and moderation type pickers pick it
   up from the same catalog.

2. **Creation permission** (`internal/content/handlers.go`): `POST /content`
   checks `list.create` when `content_type === 'list'` — same pattern as the
   existing `story.create` check (super_admin wildcard or explicit node grant;
   403 `forbidden` otherwise). `PATCH /content/{id}` needs no new check: the
   update input carries no `content_type` field (type cannot change on edit),
   and editing follows the existing author/`content.edit_any` rules — matching
   how `story.create` applies to create only.

3. **`field_values` validation** (`internal/content/store.go` or handlers):
   for `list`, validate `list_mode` against the enum, enforce
   `items.length ≤ list.max_items`, sanitize each item's `body_html`, and
   verify `media_id` / `content_id` / `video_source_id` values are valid UUIDs.
   Invalid input → `400` with a `snake_case` code (Rule 5).

4. **System settings** (`internal/control/settings_catalog.go` +
   `.jeos/conventions/system-settings-catalog.md`):
   - `list.enabled` — bool, default `"true"`, public panel (killswitch, same
     role as `video.enabled`). Composer treats missing as enabled; explicit
     `false` hides the type.
   - `list.max_items` — int, default `"50"`, admin panel.

5. **RBAC** (`.jeos/sections/03-rbac.md` + seed migration):
   - `list.create` — create/edit list content via the composer and
     `POST /content`. Seeded and granted to `super_admin` only (wildcard covers
     it; explicit grant for completeness). **Not** granted to `member`.
   - `agent.content_type.list` — added to the §03 agent node family so the AI
     agent's per-type gating stays complete.

6. **JSON-LD** (seed migration into `content_type_jsonld_templates`): an
   `ItemList` template for `content_type = 'list'` interpolating title, items'
   positions/headings, author, and dates — per §14's admin-template design.

7. **No new API routes.** Reuses `POST /content`, `PATCH /content/{id}`,
   `GET /content`, `GET /content/{slug}`, `GET /link-preview`,
   `GET /media/{id}`, `GET /videos/{id}`, `GET /content-types`,
   `GET /me/permissions`. Internal-embed hydration uses the existing public
   `GET /content/{slug}`.

8. **No review clamp.** `SkipStoryModeration` machinery is untouched; `list`
   authors with the node publish/schedule directly.

## Frontend changes (apps/main)

1. **Type registry** (`contentTypeRegistry.ts`): add `'list'` to
   `CREATE_ALLOWED_TYPES`; `COMPOSER_OVERRIDES.list = { formKind: 'list',
   supportsSeries: false, featureFlag: 'list.enabled' }`; extend `FormKind`
   with `'list'`.

2. **Composer gate** (`/studio/create/+page.server.ts` + `createAvailableTypes`):
   the `list` type is offered only when `user.permissions` includes
   `list.create` AND `list.enabled !== false`. Guests/no-permission users never
   see it in the type selector (backend still enforces on write).

3. **`editors/ListBuilder.svelte`** (new; `formKind === 'list'` branch in
   `CreateShell`):
   - Intro editor — reuses the standard `PostEditor`.
   - Mode picker — numbered / countdown / unnumbered (segmented control).
   - Item cards — heading input; body text area (or compact TipTap); one
     visual-slot picker with tabs:
     - *Image* → `MediaLibraryModal` (existing) → `media_id`.
     - *JapaConnect content* → autocomplete over `GET /search`/`GET /content?q=`
       → stores `content_id` + `content_slug` + snapshot card data.
     - *External link* → `GET /link-preview?url=` (existing
       `link_preview.fetch` route) → stores `url` + `resolved` snapshot;
       `embed_url` kept when the resolver returns one.
     - *Video* → platform video picker (existing `VideoField`/`GET /videos`)
       → `video_source_id`.
   - `source_url` + `credit` inputs per item; add / remove / reorder
     (up-down buttons minimum).
   - Serialization on save: builds `body` (plain text incl. items), `body_html`
     (placeholder markup incl. baked numbers + snapshot card children), and
     `field_values` — one `POST /content` or `PATCH` payload.
   - Draft restore reads `field_values.list_mode` + `items`; intro from `body`.

4. **Detail route** `/list/[slug]` (+page.server.ts + +page.svelte): loads via
   the existing `loadContentArticle` path, renders `ContentArticlePage` (same
   shell — CreatorCard, ReactionRail, comments, related) plus a new
   `use:hydrateContentEmbeds` action on the prose container. `/list/{slug}`
   becomes the canonical family: `contentHref` += `list: '/list'`;
   `/content/{slug}` 301-redirects `list` items to it like other families.

5. **`hydrateContentEmbeds.ts` + `ContentEmbedCard.svelte`** (new, in
   `$lib/tiptap` / `$lib/components/content`): scans for
   `div[data-type="content-embed"][data-content-slug]`, fetches
   `GET /content/{slug}`, mounts the card; on failure mounts the stored
   snapshot (kept as placeholder children) or an "unavailable" state.

6. **`/lists` index** (+page.server.ts + +page.svelte): lists published `list`
   content via `GET /content?type=list` (existing `ListContentsByType` —
   supports `?sort=popular|roadmap` and `?featured=1` for a Featured tab) —
   modeled on `/stories` (sort + pagination controls per
   `content.pagination.style`).

7. **i18n**: new keys in `en.json`/`fr.json` — `create.types.list`,
   list-builder labels, `/lists` chrome, embed-picker tab labels,
   "unavailable" states.

## JEOS + catalog updates

- `.jeos/sections/14-content-type-system.md` — add `list` to the built-in type
  list; new "List Content Type System" amendment (modes, item schema, embed
  contract, `body`/`body_html`/`field_values` serialization contract,
  permission gate).
- `.jeos/sections/03-rbac.md` — document `list.create` + `agent.content_type.list`.
- `.jeos/conventions/system-settings-catalog.md` — `list.enabled`, `list.max_items`.
- `.jeos/conventions/api-routes-catalog.md` — no new routes; note `list` flows
  through the universal content routes.
- Rule 17: no AI attribution anywhere.

## Testing

- Backend unit tests: type registry/labels; `list.create` gate on
  POST/PATCH (allowed / forbidden paths); `field_values` validation (bad mode,
  over-limit items, bad UUIDs); sanitizer keeps `data-content-slug`;
  `content_hash` covers item text.
- Frontend tests: `contentTypeRegistry` (type order, flag gate, permission
  gate); `ListBuilder` serialization (body/body_html/field_values contract,
  countdown numbering); `hydrateContentEmbeds` mount + fallback.
- Manual: create → publish → view `/list/{slug}` → verify embeds hydrate,
  numbers render, `/lists` lists it, `contentHref` links resolve, drafts
  round-trip.

## Files to change

Backend:
- `internal/content/content.go` (type registry)
- `internal/content/handlers.go` (`list.create` check, `field_values` validation call)
- `internal/content/store.go` (item validation, if not in handlers)
- `internal/content/sanitize.go` (div whitelist: `data-content-slug`)
- `internal/control/settings_catalog.go` (`list.enabled`, `list.max_items`)
- `internal/ai/tools_content_bulk.go` + `internal/ai/agent.go` (add
  `agent.content_type.list` to the enumerated node family)
- `migrations/000748_list_content_type.up.sql` + `.down.sql`
  (seed `list.create`, `agent.content_type.list`, grants, `ItemList` JSON-LD template)

Frontend (apps/main):
- `src/lib/components/create/contentTypeRegistry.ts`
- `src/lib/components/create/CreateShell.svelte` (`formKind === 'list'` branch)
- `src/lib/components/create/editors/ListBuilder.svelte` (new)
- `src/lib/tiptap/hydrateContentEmbeds.ts` (new)
- `src/lib/components/content/ContentEmbedCard.svelte` (new)
- `src/lib/utils/contentLink.ts` (`list: '/list'`)
- `src/lib/types/index.ts` (`'list'` in ContentType union + item/embed types)
- `src/routes/(app)/list/[slug]/+page.server.ts` + `+page.svelte` (new)
- `src/routes/(app)/lists/+page.server.ts` + `+page.svelte` (new)
- `src/routes/(app)/studio/create/+page.server.ts` (permission-aware type list)
- `src/lib/i18n/en.json`, `fr.json`

JEOS:
- `.jeos/sections/14-content-type-system.md`
- `.jeos/sections/03-rbac.md`
- `.jeos/conventions/system-settings-catalog.md`
- `.jeos/conventions/api-routes-catalog.md`

## Out of scope (v1)

- Per-item reactions / comments / share buttons (post-level engagement only).
- Per-item item anchors exist as `id="item-N"` attributes for free; no
  copy-link UI.
- Admin-panel list editor (lists are authored in `/studio/create` only).
- "N items" badge on feed cards (optional later enhancement).
- Re-resolving external embed snapshots server-side after publish (snapshot
  at compose time only).
