# Space System Refactor — Categories, Geo Linking & Global Feed

**Date:** 2026-08-19  
**Status:** Draft — pending user review  
**Source idea:** Major refactor of the JapaConnect space feature to make space creation simpler and to power geo-aware, globally-fallback feeds.  
**Scope:** Data model, migrations, backend, admin panel (`/space-types`), main frontend (`/spaces` create modal), and feed ranking.

---

## 1. Overview

JapaConnect is built around spaces. Today the system has a flat list of space types (`country`, `state`, `city`, `community`, `university`, `global`) and the create flow forces members to pick a geo hierarchy. This spec refactors spaces into a category + sub-type model, removes the manual geo selectors, auto-links new spaces to primary geo spaces by keyword, and turns `Global` into a virtual fallback feed.

### Decisions locked during brainstorming

| Decision | Choice |
|---|---|
| Category model | **Yes**. Built-in categories: `community`, `education`, `business`, `profession`, `interest`, `lifestyle`. Custom admin-created types are sub-types of `education`, `business`, `profession`, `interest`, or `lifestyle`. |
| Default selected type in create modal | `community` |
| Frontend dropdown style | **Flat list of all space types**; category is not shown to members. |
| Existing `university` type | Keep as a sub-type; set its `category = 'education'`; create a new `education` system category type. |
| `global` space / type | **Remove as a space type**. `Global` is a virtual feed concept, not a selectable type or browsable space. |
| Geo selection in create modal | **Remove country/state/city selectors**; backend infers geo parents by keyword-matching the space name. |
| Space-to-geo link | New spaces get `space_parent_spaces` entries to matched `country`/`state`/`city` primary spaces; `parent_id` points to the most specific match. |
| Content-to-geo link | When content is created in a member space, it is linked both to title-matched primary spaces (existing) and to the space’s own geo parents (new). |
| Default feed geo fallback | **Pull-based** for `/feed/for-you` and `/feed/explore`: followed spaces → city → state → country → region/continent → global popular. |
| `/feed` (Following) | **Keep push-based**; geo fallback is not required there. |
| Region/continent data | Add `region` to country-level spaces (migration + backfill) so the Nigeria → Africa → global cascade works. |

---

## 2. Architecture

```mermaid
flowchart TD
    subgraph ADMIN["Admin Panel /space-types"]
        A1["Create type: name, slug, category"]
    end

    subgraph MEMBER["Main app /spaces"]
        M1["Create space: name + type"]
        M2["No country/state/city picker"]
    end

    subgraph BACKEND["Backend"]
        S1["Space type CRUD"]
        S2["Space creation + keyword matcher"]
        S3["Content creation + geo linker"]
        S4["Feed service: geo-proximity ranking"]
    end

    subgraph DB[(Database)]
        T1["space_types<br/>+ category"]
        T2["spaces<br/>+ parent_id + space_parent_spaces"]
        T3["content_spaces"]
        T4["spaces.region<br/>(country level)"]
    end

    ADMIN --> S1 --> T1
    MEMBER --> S2
    S2 --> T1
    S2 --> T2
    S3 --> T3
    S4 --> T3
    S4 --> T2
    S4 --> T4
```

---

## 3. Data model & migrations

### 3.1 `space_types` table

Add `category` text (nullable). Deprecate `is_university` (to be removed after the refactor is complete).

| id | name | slug | category | is_system | note |
|---|---|---|---|---|---|
| *existing* | Country | country | `null` | true | geo |
| *existing* | State | state | `null` | true | geo |
| *existing* | City | city | `null` | true | geo |
| *new* | Community | community | `community` | true | default, no sub-types |
| *new* | Education | education | `education` | true | category parent |
| *new* | Business | business | `business` | true | category parent |
| *new* | Profession | profession | `profession` | true | category parent |
| *new* | Interest | interest | `interest` | true | category parent |
| *new* | Lifestyle | lifestyle | `lifestyle` | true | category parent |
| *existing* | University | university | `education` | true | sub-type under education |

- Admin-created types must have `category` in `{education, business, profession, interest, lifestyle}`.
- `community` and the geo types cannot be created by admins.
- The old `global` space type is deleted (or hidden and ignored) because `Global` is a virtual feed.

### 3.2 `spaces` table & `space_parent_spaces`

- `parent_id` continues to point to the most specific matched primary space (city preferred, then state, then country).
- `space_parent_spaces` stores all matched primary spaces for a member-created space.
- New spaces are keyword-matched against primary `country`/`state`/`city` spaces by name.

### 3.3 Country `region` data

Add `region` (or `continent`) text column to `spaces` for `level = 'country'` rows. Backfill from the country dataset already used in `000286_seed_all_countries_states.up.sql`. This allows the feed cascade: city → state → country → region → global.

---

## 4. Member space creation

### 4.1 Main app `/spaces` create modal

- Remove the country/state/city cascading selectors.
- Show a **flat dropdown of all selectable space types**, defaulting to `community`.
- Categories are not visible to members.
- Member enters a name and picks a type.

### 4.2 Backend creation flow

1. Validate name and selected `space_type_id`.
2. Keyword-match the space name against `country`/`state`/`city` primary spaces using the same n-gram logic used for content auto-assignment.
3. Insert `space_parent_spaces` rows for all matches.
4. Set `spaces.parent_id` to the most specific match (city > state > country).
5. If no match, `parent_id` stays `null`; the space is only reachable by category and the global feed.

---

## 5. Content-to-geo linking

When content is published:

1. Run the existing title-based primary-space matcher.
2. Also add the content to the primary geo spaces linked to its source space (from `space_parent_spaces`).
3. Deduplicate: a content item should not be linked to the same primary space twice.

This ensures a post in “Lagos Tech Community” surfaces in the Lagos and Nigeria feeds even if its title has no geo keywords.

---

## 6. Feed & Global fallback

`Global` is not a real space. It is the default, unfiltered public feed mode.

### 6.1 Feed targets

- `/feed/for-you` — logged-in default (pull, personalized, geo-aware).
- `/feed/explore` — guest default (pull, geo-aware).
- `/feed` — Following tab (push, unchanged; shows only followed/membership content).

### 6.2 Ranking

For pull feeds, compute a `geo_proximity_score` per candidate:

1. **Followed affinity** — author/space/tag the user follows (highest).
2. **Exact city match** — content is linked to the visitor’s current city.
3. **Same state** — content is linked to a city/state in the visitor’s state.
4. **Same country** — content is linked to any place in the visitor’s country.
5. **Same region/continent** — e.g., Nigeria → other African countries.
6. **Global popular / trending** — public content with high velocity/popularity.

The final order is a blend of `geo_proximity_score`, `velocity_score`, `vote_score`, and the followed-affinity flags. This satisfies the requirement that a Ukrainian user sees Ukraine-targeted content first, then neighboring/global content if local content is sparse.

### 6.3 Region support

The query joins `content_spaces` → `spaces` → country space (via `space_parent_spaces`) and compares `spaces.region` with the visitor’s country region. For performance, `region` is added to country-level `spaces` and indexed.

---

## 7. Admin `/space-types`

- Replace the “University type” checkbox with a **category selector**.
- Available categories for new types: `education`, `business`, `profession`, `interest`, `lifestyle`.
- `community` is not selectable for new types.
- The existing `university` type keeps its name and slug; its `category` becomes `education`.
- UI copy: “University type” → “Education type” or simply “Category”.

---

## 8. Backward compatibility & touch points

- Migrate `university` type → `category = 'education'`.
- Remove or hide the `global` space type.
- Update `SpaceCreationModal` (`apps/main/src/lib/components/spaces/SpaceCreationModal.svelte`):
  - remove geo cascade
  - default type `community`
  - flat type dropdown
  - keep permission preset selection
- Update `apps/admin/src/routes/space-types/+page.svelte`:
  - replace `is_university` checkbox with category select
  - rename labels
- Update backend handlers and service logic:
  - `internal/spaces/space_types_handlers.go`
  - `internal/spaces/space_crud_handlers.go`
  - `internal/spaces/store.go`
  - `internal/content/handlers.go` / `internal/content/store.go`
- Update `internal/db/queries/spaces.sql` and `internal/db/queries/contents.sql`:
  - `CreateSpaceType` uses `category` instead of `is_university`
  - space creation includes keyword geo matching
  - content creation links to space geo parents
- Update `internal/db/queries/feeds.sql` with geo-proximity ranking.
- Regenerate sqlc (`make sqlc-gen`).
- Update tests: `spaces_test.go`, `space_card_test.go`, content tests, feed tests.
- Update i18n strings in `apps/admin/src/lib/locales/en.json` / `fr.json`.

---

## 9. Implementation details (decisions for build)

1. **Keyword matcher for space names**  
   Reuse the existing `spaceMatcher` used for content auto-assignment, but lower the minimum match threshold for short names and prefer exact word matches over n-gram partials when the space name is short.

2. **`category` column type**  
   Use a `text` column with application-level validation. This avoids PostgreSQL enum migrations when categories change. A single `category` is allowed per type.

3. **`region` value**  
   Use the `region` (UN region / continent) value already present in the country dataset used by `000286_seed_all_countries_states.up.sql`. Add both `region` and `subregion` to `spaces` for country-level rows and backfill during migration.

4. **Member dropdown contents**  
   The flat dropdown includes **all** selectable space types: the generic category types (`education`, `business`, `profession`, `interest`, `lifestyle`, `community`) plus their sub-types (`university`, `research`, etc.). `community` is the default selection.

---

## 10. Success criteria

- A member can create a space without selecting country/state/city.
- The created space is linked to the correct primary geo spaces based on its name.
- Admin can create new space types under `education`, `business`, `profession`, `interest`, or `lifestyle`.
- The `university` type is now a sub-type under `education`.
- The guest and logged-in default feeds show geo-relevant content first and fall back to global popular content.
- No `/spaces/global` profile page exists because `Global` is virtual.
