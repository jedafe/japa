# Design: daily.dev-style feed card, entity hover cards, and social post page

**Date:** 2026-09-10
**Scope:** Main site (`apps/main`) + backend (`internal/feed`, `internal/content`, `internal/db/queries`)
**Status:** Draft — pending final approval

## 1. Goals

Make all feed surfaces in JapaConnect (Explore, For You, Following, User, Organization, Space) render a consistent, daily.dev-inspired card. The new `daily` card:

- surfaces the publisher (author, space, and/or organization) as hoverable chips;
- uses reactions (heart icon) instead of upvote/downvote for posts;
- keeps a bottom metric bar with reactions, comments, views, and bookmarks;
- opens a new social `/post/[content_type]/[slug]` page with a content preview, reaction rail, and comment thread, plus a canonical link to the single-type page;
- becomes the default and only card for feed streams, while dedicated content-type list pages keep their existing styles.

## 2. Decisions already made

- **Post page URL:** `/post/[content_type]/[slug]` (e.g. `/post/blog_post/why-i-moved`). This disambiguates slug collisions across content types while staying readable.
- **Post page SEO:** The `/post/*` page is a social wrapper, not the canonical SEO page. It will emit `<link rel="canonical" href="/{content_type}/{slug}">` (or the correct single-type path) and `noindex`.
- **Route to all three phases in one pass:** card style, hover cards, and `/post` page together.

## 3. Phase 1 — `daily` card style for all feeds

### 3.1 Frontend card layout

Update `apps/main/src/lib/components/feed/FeedCard.svelte` with a new `daily` branch. Structure per card:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ [Avatar] Author Name         Space/Org • 6m                Read post · ⋮    │
├──────────────────────────────────────────────┬──────────────────────────────┤
│                                              │                              │
│  Title of the post                           │        Cover image           │
│                                              │                              │
│  #tag #tag +1                                │                              │
│                                              │                              │
├──────────────────────────────────────────────┴──────────────────────────────┤
│  💬 8    ♥ 24    🔖    ↗                                                    │
└─────────────────────────────────────────────────────────────────────────────┘
```

- **Top-left:** Author chip. The user is the primary entity for feed cards; the secondary line under the author shows the publishing organization/space (when present), the read time, or the date.
- **Content-type identity:** Because the same `daily` card is used for blog, guide, video, story, discussion, etc., each card must clearly identify its content type. The type badge is rendered as a pill with a Lucide icon and a label, using a distinct accent color per type. Unknown or custom types fall back to a generic icon. The icon and color make each type scannable at a glance without forking the card layout.

  Proposed default icon map (overridable by admin CSS variables / settings later):
  | content type | icon | accent role |
  |---|---|---|
  | `blog_post` | `FileText` | `--color-content-blog` |
  | `guide` | `Compass` | `--color-content-guide` |
  | `story` | `BookOpen` | `--color-content-story` |
  | `discussion` | `MessagesSquare` | `--color-content-discussion` |
  | `video` | `Play` | `--color-content-video` |
  | `comparison` | `Scale` | `--color-content-comparison` |
  | `event` | `Calendar` | `--color-content-event` |
  | `job_listing` | `Briefcase` | `--color-content-job` |
  | `service_listing` | `Wrench` | `--color-content-service` |
  | `achievement` | `Trophy` | `--color-content-achievement` |
  | other / custom | `FileText` | `--color-primary` |
- **Top-right:** relative time, read time, a context-aware "Read {type}" or "View {type}" arrow (e.g. "Read guide", "View comparison", "Read blog", "View video", "Read story", "View discussion"), and a `⋮` action menu (share, bookmark, report). Labels are i18n keys so they are localizable.
- **Middle:** content type badge, title, and excerpt on the left; cover image on the right (video play overlay preserved).
- **Bottom metric bar:** comment count, `reaction_count` (heart icon), bookmark, and share. Post-level upvote/downvote controls are not present; the heart is a visual metric only. Each bottom icon opens `/post/[content_type]/[slug]` on click. Hovering a chip opens a compact entity card.

The entire card body (outside the chips, menu, and explicit links) is clickable and navigates to `/post/[content_type]/[slug]`.

### 3.2 Card style config

- Add `daily` to `CardStyle` in `apps/main/src/lib/feed/cardStyle.ts`.
- Add `daily` to `apps/admin/src/lib/feed/cardStyleValue.ts` and `CARD_STYLE_OPTIONS` so admins can set it as the default.
- The six feed pages (home, explore, user, space, org, and any future feed surface) will force `cardStyleConfig = { default: 'daily', byType: {} }` at the page level.
- Dedicated content-type list pages (blog, forum, guides, etc.) will continue to use the global `cardStyleConfig` or their own override, so they are not forced to `daily`.

### 3.3 Feed data fields

`FeedItem` (frontend) and the backend `feed.FeedItem` struct need:

- `author_username` — for user hover cards and links.
- `organization` — `{ id, name, slug, logo_url } | null`.
- `reaction_count` — total content reactions.
- `space_name` / `space_slug` / `space_avatar_url` (if not already present).
- `cover_image_url` (already present but must be populated for org/user feeds).
- `tags` (already present but must be populated for org/user feeds).
- `bookmark_count` (already present but must be populated for org/user feeds).

### 3.4 Backend query changes

Update these sqlc queries in `internal/db/queries/feeds.sql`:

- `GetExploreFeed`
- `GetPrimaryFeed`
- `GetSpaceFeed`
- `GetProfileFeed`
- `GetTopFeed`
- `GetRisingFeed`

Each query will:

1. Join `users u` and `user_profiles up` and also select `u.username`.
2. Join `organizations o` on `c.organization_id` and select `o.id`, `o.name`, `o.slug`, and `o.logo_media_id` (resolve to `logo_url` in Go/frontend).
3. Left join a `reactions` subquery to compute `reaction_count`:
   ```sql
   COALESCE(r.reaction_count, 0) AS reaction_count
   LEFT JOIN (
       SELECT entity_id, count(*) AS reaction_count
       FROM reactions
       WHERE entity_type = 'content'
       GROUP BY entity_id
   ) r ON r.entity_id = c.id
   ```
4. Select `content_scores.bookmark_count` (already present in many queries).
5. Select `m.storage_bucket` / `storage_path` for the cover image and resolve it via `media.MediaURL` in the service layer.
6. Add `reaction_count` to the `content_scores` table, update `UpdateContentScore` and `UpdateVelocityScores`, and use `reaction_count` in the `top` and `rising` sort ordering so reactions drive feed ranking.

Update `internal/db/queries/contents.sql`:

- `ListPublishedContentByAuthor` and `ListPublishedContentByOrganization` to include the same fields (author name/avatar/username, space name/slug/avatar, org details, tags, cover image, reaction count, bookmark count, excerpt, journey metadata).

Run `make sqlc-gen` to regenerate `internal/db/gen/*`.

Update mapping functions in `internal/feed/service.go` to populate the new `FeedItem` fields.
Update `internal/content/profile_posts.go` (`PostCard` and `toPostCards` / `toOrgPostCards`) to populate the new fields and produce feed-compatible cards.

### 3.5 Org feed migration

- `apps/main/src/routes/(app)/org/[slug]/+page.svelte` currently renders `posts` as a hand-rolled list of links.
- Change it to map the new `ProfilePost` (now feed-compatible) to `FeedItem` using a `toFeedItem` helper and render `FeedCard` components inside a `FeedList` (or a minimal bordered list if no infinite scroll is needed).
- Update `apps/main/src/routes/(app)/org/[slug]/+page.server.ts` to use the new feed-compatible `ProfilePost` mapping.
- Remove the custom `/content/{post.slug}` links and let `FeedCard` handle the link (to the new `/post/[content_type]/[slug]` page or to the canonical page depending on the click target).

### 3.6 User feed alignment

- `apps/main/src/routes/(app)/u/[username]/+page.svelte` already maps `ProfilePost` to `FeedItem`. Update the `toFeedItem` helper to use the new backend fields (`author_username`, `reaction_count`, `bookmark_count`, `cover_image_url`, `tags`, `space_name`, `space_slug`, `organization`) and remove fallbacks that come from `data.user`.

### 3.7 Reactions in the card

- The bottom metric bar replaces the `ArrowUp`/`ArrowDown` pair with a `Heart` icon and `reaction_count`.
- **Hover behavior:** hovering/focusing the heart button opens a compact reaction picker (the same reaction types configured by the admin). The picker fetches the current user's reactions for this content on open.
- **Click behavior:** clicking the heart button navigates to `/post/[content_type]/[slug]` and toggles the default (`like`) reaction on arrival. Clicking a specific reaction in the hover picker also navigates to the post page and toggles that reaction. This keeps per-card state simple while still allowing quick reactions.
- The `ReactionRail.svelte` component already implements reaction fetching, toggling, and bookmark/share. The `/post` page will reuse it in horizontal mode.

## 4. Phase 2 — Entity hover cards

### 4.1 Components

Create three hover-card components built on `Bits.UI` `HoverCard`:

- `apps/main/src/lib/components/feed/UserHoverCard.svelte`
- `apps/main/src/lib/components/feed/SpaceHoverCard.svelte`
- `apps/main/src/lib/components/feed/OrgHoverCard.svelte`

Wrap them in a small `EntityHoverCard.svelte` helper that picks the right card based on the entity.

### 4.2 Content

Each hover card fetches the relevant public detail endpoint on hover and shows:

- Avatar / logo.
- Name, handle/slug, and a verification/organization-type badge.
- Short description / tagline.
- Follower / member count.
- A `FollowButton` when the viewer is authenticated and not the owner.

Endpoints used:

- `GET /users/{username}`
- `GET /spaces/{slug}`
- `GET /organizations/{slug}` (accepts slug for public profile per Section 07)

### 4.3 UX

- Delayed open/close to avoid accidental popovers.
- Positioned with `HoverCard.Content` so it does not clip inside the feed container.
- Degrades to a normal link on touch devices.
- Keyboard focusable via the trigger and close button.
- Respects `prefers-reduced-motion`.

## 5. Phase 3 — `/post/[content_type]/[slug]` social page

### 5.1 Frontend route

- `apps/main/src/routes/(app)/post/[content_type]/[slug]/+page.server.ts`
- `apps/main/src/routes/(app)/post/[content_type]/[slug]/+page.svelte`

### 5.2 Backend endpoint

Add a new route:

```
GET /api/v1/post/{content_type}/{slug}
```

It returns the same public detail shape as `GET /content/{slug}` but disambiguated by `content_type`. This protects against slug collisions and lets the post page load the correct content when the same slug exists for two content types.

The handler reuses the existing content-detail query logic, adding a `content_type` filter. It returns a payload with:

- `id`, `title`, `slug`, `content_type`, `body_html`, `excerpt`
- `author_id`, `author_name`, `author_username`, `author_avatar_url`
- `organization` block
- `space` block
- `cover_image_url`, `featured_image_url`, `header_image_url`
- `canonical_url` (or computed canonical from `contentHref`)
- `view_count`, `comment_count`, `reaction_count`, `bookmark_count`
- `published_at`, `created_at`, `reading_time_minutes`
- `tags`, `journey_*` metadata, `guide_category_label` if guide
- `video_source_id`, `duration_secs` for video

### 5.3 Page layout

The page renders inside the unified shell (left nav, center column, right sidebar).

**Center column:**

1. Back-to-feed link.
2. Post preview card:
   - Author and source chips (with hover cards).
   - Content type badge.
   - Title.
   - Cover image / video preview.
   - Body preview rendered from the full `body_html`, collapsed by default to a max height (e.g. `24rem` / `384px`). A "Read more" link expands the preview. A context-aware "Read full {type}" or "View full {type}" button links to the canonical single-type page (`/blog/[slug]`, `/guide/[slug]`, etc.) for the authoritative reading layout.
3. Horizontal `ReactionRail` with the same `contentId`, seeded with `reaction_count`, `comment_count`, and initial bookmark status.
4. `CommentThread` with the content's comments, paginated.

**Right sidebar:**

- `CreatorCard` for the author.
- Related content (`GET /content/{id}/related`).
- Standard sidebar widgets from the layout.

### 5.4 SEO

- `<link rel="canonical" href="{canonical single-type URL}">`
- `<meta name="robots" content="noindex, follow">` or `X-Robots-Tag: noindex, follow`
- The canonical single-type page keeps its existing JSON-LD and meta tags.

### 5.5 Canonical single-type pages

No changes to `/blog/[slug]`, `/guide/[slug]`, `/content/[slug]`, etc. They remain the authoritative SEO/read page. The `/post` page links to them.

## 6. Backend route catalog updates

Add to `conventions/api-routes-catalog.md`:

```
GET    /post/{content_type}/{slug}       [public, tenant-scoped] social preview of content; canonical link points to single-type page
```

## 7. JEOS section updates

Before implementation, update the following section files to reflect the new behavior:

- `sections/23-main-site-frontend.md` — describe the `daily` card, the unified feed surfaces, and the `/post` social page inside the shell.
- `sections/14-content-type-system.md` — add `reaction_count` aggregation in feed queries and the new `/post/{content_type}/{slug}` route.
- `sections/32-activity-feed-timeline.md` — document the `daily` card as the default feed card and the `reaction_count` field.
- `sections/49-comments-replies.md` — confirm the post page reuses the unified comment engine.
- `sections/07-organization-profiles-claim-ownership.md` — confirm the org Posts tab uses the same `FeedCard`.

## 8. Testing and verification

- **TypeScript:** `bun run check` in `apps/main`.
- **Svelte tests:** Add tests for the new `daily` `FeedCard` variant and hover cards.
- **Go tests:** Add tests for the updated `PostCard` mappers and `FeedItem` mapping functions.
- **SQLC:** `make sqlc-gen` and ensure no regressions.
- **Manual checks:**
  - Home, Explore, Following, User, Space, and Org feeds all render `daily` cards.
  - Hover cards work for user, space, and org.
  - `/post/[content_type]/[slug]` loads, shows preview, canonical link, reaction rail, and comments.
  - Upvote/downvote arrows no longer appear on post cards.
  - Dedicated list pages (blog, forum, guides) retain their existing card styles.

## 9. Risks and mitigation

| Risk | Mitigation |
|---|---|
| Slug collisions on `/post/[content_type]/[slug]` | The route includes `content_type`; the backend filters by both. |
| Duplicate SEO URLs | `/post/*` uses canonical + noindex. |
| N+1 reaction counts in feed | Reaction count is computed via a single `LEFT JOIN` subquery in each feed SQL query. |
| Org feed missing fields | Extend `ListPublishedContentByOrganization` to select the same columns as feed queries. |
| Breaking existing card styles | `daily` is an opt-in style; feed pages force it per page, list pages keep their config. |

## 10. Files likely to change

### Frontend

- `apps/main/src/lib/feed/cardStyle.ts`
- `apps/main/src/lib/components/feed/FeedCard.svelte`
- `apps/main/src/lib/components/feed/FeedList.svelte`
- `apps/main/src/lib/components/feed/FeedSortBar.svelte` (if spacing changes)
- `apps/main/src/lib/types/index.ts` (`FeedItem`, `ProfilePost`)
- `apps/main/src/routes/(app)/+page.svelte`
- `apps/main/src/routes/(app)/+page.server.ts`
- `apps/main/src/routes/(app)/explore/+page.svelte`
- `apps/main/src/routes/(app)/u/[username]/+page.svelte`
- `apps/main/src/routes/(app)/u/[username]/+page.server.ts`
- `apps/main/src/routes/(app)/spaces/[slug]/+page.svelte`
- `apps/main/src/routes/(app)/org/[slug]/+page.svelte`
- `apps/main/src/routes/(app)/org/[slug]/+page.server.ts`
- `apps/main/src/routes/(app)/post/[content_type]/[slug]/+page.server.ts` (new)
- `apps/main/src/routes/(app)/post/[content_type]/[slug]/+page.svelte` (new)
- `apps/admin/src/lib/feed/cardStyleValue.ts`

### Backend

- `internal/db/queries/feeds.sql`
- `internal/db/queries/contents.sql`
- `internal/db/gen/*` (regenerated via `make sqlc-gen`)
- `internal/feed/service.go` (`FeedItem` struct and mappers)
- `internal/content/profile_posts.go`
- `internal/content/store.go` (content detail by slug + content_type)
- `internal/content/handlers.go` (new `GetPost` handler)
- `internal/server/server.go` (new route)
- `conventions/api-routes-catalog.md`

## 11. Decisions recorded

1. **Heart interaction:** Hovering/focusing the heart opens a compact reaction picker; clicking the heart (or any reaction in the picker) navigates to `/post/[content_type]/[slug]` and toggles the selected reaction on arrival.
2. **Post preview:** Render the full `body_html` but collapse it to a max height by default, with a "Read more" expand affordance and a persistent "Read full post" link to the canonical single-type page.
3. **Scoring:** `reaction_count` is added to `content_scores` and used in the `velocity_score` formula and `top` / `rising` sort ordering.

---

**Next step:** Approve this spec. Once approved, I will update the relevant JEOS section files, then begin implementation starting with the backend query changes and the new `daily` `FeedCard`.
