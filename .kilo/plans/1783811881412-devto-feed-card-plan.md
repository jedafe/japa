# Plan: Dev.to-Inspired Feed Card + Admin Toggle

## Context

The current feed card (`apps/main/src/routes/feed/+page.svelte`) uses a compact Reddit-style layout: author initial avatar + name + relative time, title, excerpt, and a bottom bar with reaction/comment counts and content type badge. The same card is reused across primary feed, space feed, and profile feed views.

The goal is to add a full dev.to-inspired card variant with: **cover image, author avatar + name + date, title, tags, reading time, reaction/comment/bookmark counts**, and an admin toggle to switch between the two card styles globally across all feed views.

## Scope

- **Frontend (main app):** Two card components (`RedditCard.svelte` and `DevtoCard.svelte`), extracted from the current inline card markup. The feed pages conditionally render the active style based on a `feed_card_style` setting.
- **Backend:** Extend the `FeedItem` struct and feed queries to return `author_name`, `author_avatar`, `tags[]`, `reading_time_minutes`, `cover_image_url`, `bookmark_count`, and `excerpt`. These fields already exist in the content/content_scores tables but are not currently returned by the feed service.
- **Admin panel:** Add a `feed_card_style` select setting (`"reddit" | "devto"`) under the existing `appearance` category in `internal/admin/settings.go`. Also update the system settings catalog.
- **No schema changes:** All required data (tags, cover_image, content body for reading time) already exists in the `contents` and `content_scores` tables.

## Decisions

1. **Card extraction:** Extract the current inline card in each feed page into a `FeedCard.svelte` component that renders either `RedditCard.svelte` or `DevtoCard.svelte` based on a prop. This avoids duplicating the infinite scroll / load-more logic across all three feed pages.
2. **Setting key:** `feed.card_style` in `system_settings`, with values `"reddit"` (default) or `"devto"`.
3. **Admin placement:** Under the existing `appearance` category in `defaultSettings`.
4. **Scope:** All three feed views (primary, space, profile) use the selected card style.
5. **Reading time:** Computed server-side from `contents.body` word count (~200 wpm). Returned as `reading_time_minutes` in the feed item.
6. **Cover image:** Use the `cover_image_id` from `contents` table, resolved to a URL via imgproxy (already used elsewhere in the codebase).
7. **Tags:** Joined from `content_tags` table, returned as `string[]`.
8. **Bookmark count:** Already in `content_scores.bookmark_count`, just needs to be added to the feed query and struct.

## Affected Files

### Backend (Go)

| File | Change |
|---|---|
| `internal/feed/service.go` | Extend `FeedItem` struct with `AuthorName`, `AuthorAvatarURL`, `Excerpt`, `Tags []string`, `ReadingTimeMinutes int`, `CoverImageURL`, `BookmarkCount` |
| `internal/feed/handlers.go` | No structural changes (already returns `FeedItem` list) |
| `internal/feed/feed.go` | No changes needed |
| `internal/db/queries/feeds.sql` | Extend feed queries to JOIN `contents` for excerpt/tags/cover, JOIN `content_scores` for bookmark_count, JOIN `users` for author_name/avatar, compute reading_time |
| `internal/db/gen/feeds.sql.go` | Regenerated via `make sqlc-gen` |
| `internal/admin/settings.go` | Add `feed_card_style` to `appearance` category settings |
| `internal/server/server.go` | Add GET `/feed/card-style` public endpoint (or serve from settings) |
| `.jeos/conventions/system-settings-catalog.md` | Add `feed.card_style` entry |

### Frontend — Main App

| File | Change |
|---|---|
| `apps/main/src/lib/components/feed/FeedCard.svelte` | **New.** Wrapper that renders RedditCard or DevtoCard based on `cardStyle` prop |
| `apps/main/src/lib/components/feed/RedditCard.svelte` | **New.** Extracted current card markup |
| `apps/main/src/lib/components/feed/DevtoCard.svelte` | **New.** Dev.to-style card with cover image, tags, reading time, bookmark count |
| `apps/main/src/routes/feed/+page.svelte` | Replace inline card with `<FeedCard />` component, fetch `feed_card_style` from API |
| `apps/main/src/routes/feed/space/[spaceId]/+page.svelte` | Replace inline card with `<FeedCard />` component |
| `apps/main/src/routes/feed/profile/[userId]/+page.svelte` | Replace inline card with `<FeedCard />` component |
| `apps/main/src/routes/feed/+page.server.ts` | Fetch `feed_card_style` setting and pass to page data |
| `apps/main/src/routes/feed/space/[spaceId]/+page.server.ts` | Same — fetch setting |
| `apps/main/src/routes/feed/profile/[userId]/+page.server.ts` | Same — fetch setting |
| `apps/main/src/lib/api.ts` | Extend `FeedItem` type, add `getCardStyle()` API method |
| `apps/main/src/lib/locales/en.json` | Add i18n keys for new card elements |
| `apps/main/src/lib/locales/fr.json` | Same |

### Frontend — Admin App

| File | Change |
|---|---|
| `apps/admin/src/routes/settings/+page.svelte` | No changes needed — the `appearance` category already renders select settings dynamically from the API |

## Implementation Steps

### Step 1: Backend — Extend FeedItem struct and queries

**`internal/feed/service.go`:** Add fields to `FeedItem`:
```go
type FeedItem struct {
    // ... existing fields ...
    AuthorName         string   `json:"author_name"`
    AuthorAvatarURL    string   `json:"author_avatar_url,omitempty"`
    Excerpt            string   `json:"excerpt,omitempty"`
    Tags               []string `json:"tags,omitempty"`
    ReadingTimeMinutes int      `json:"reading_time_minutes,omitempty"`
    CoverImageURL      string   `json:"cover_image_url,omitempty"`
    BookmarkCount      int32    `json:"bookmark_count,omitempty"`
}
```

**`internal/db/queries/feeds.sql`:** Extend all feed queries (GetExploreFeed, GetTopFeed, GetRisingFeed, GetPrimaryFeed, GetSpaceFeed, GetProfileFeed) to:
- JOIN `contents` for `body` (excerpt truncated to 200 chars), `cover_image_id`
- JOIN `content_scores` for `bookmark_count`
- LEFT JOIN `users` for `author_name`, `avatar_url`
- LEFT JOIN `content_tags` + `tags` for `tags[]`
- Compute `reading_time_minutes` from word count: `CEIL(LENGTH(contents.body) / 5.0 / 200.0)::int`
- Resolve `cover_image_id` to URL via imgproxy helper or return the ID for frontend resolution

**Run `make sqlc-gen`** to regenerate query code.

Update all `*RowsToItems()` converters to populate the new fields.

### Step 2: Backend — Add `feed.card_style` setting

**`internal/admin/settings.go`:** Add to the `appearance` category:
```go
{Key: "feed_card_style", Label: "Feed Card Style", Type: "select", Options: []string{"reddit", "devto"}, Value: "reddit"},
```

**`.jeos/conventions/system-settings-catalog.md`:** Add entry under a new "Feed Card" section:
```
| `feed.card_style` | `reddit` | string | `reddit` | `reddit` \| `devto` — Active feed card style |
```

### Step 3: Backend — Add public setting endpoint

**`internal/server/server.go`:** Add a public endpoint for the main app to read the card style:
```
GET /api/v1/feed/card-style → { "data": { "card_style": "reddit" } }
```

This avoids requiring the main app to call the admin settings endpoint (which requires admin auth). The feed handler or a small new handler reads `feed.card_style` from `system_settings` via the config cache.

### Step 4: Frontend — Create card components

**`apps/main/src/lib/components/feed/RedditCard.svelte`:**
- Extract the existing `<article>` markup from `feed/+page.svelte`
- Props: `item: FeedItem`, `index: number`

**`apps/main/src/lib/components/feed/DevtoCard.svelte`:**
- Cover image (if `cover_image_url` exists) as a top banner
- Author row: avatar (full image, not initial), name, relative date
- Title (linked to `/blog/{slug}`)
- Tags as clickable badges
- Bottom bar: reaction count, comment count, bookmark count, reading time badge

**`apps/main/src/lib/components/feed/FeedCard.svelte`:**
- Props: `item: FeedItem`, `cardStyle: 'reddit' | 'devto'`, `index: number`
- Conditionally renders `RedditCard` or `DevtoCard`

### Step 5: Frontend — Wire up feed pages

Update the `FeedItem` type in `apps/main/src/lib/api.ts` to include the new fields.

Add `getCardStyle()` to the `api` object:
```ts
getCardStyle() {
    return request<{ card_style: string }>('/feed/card-style');
}
```

Update `+page.server.ts` files to fetch the card style and pass it to the page data.

Update the three feed `+page.svelte` files to:
1. Replace inline card markup with `<FeedCard {item} {cardStyle} {i} />`
2. Accept `cardStyle` from page data

### Step 6: Frontend — i18n

Add keys to `en.json` and `fr.json`:
```json
"feed.reading_time": "min read",
"feed.bookmark": "Bookmark",
"feed.bookmarks": "Bookmarks"
```

## Edge Cases

- **No tags:** DevTo card hides the tags row entirely
- **No cover image:** DevTo card renders without the image banner (text-only mode)
- **Long excerpts:** Both cards use `line-clamp-3` for excerpts
- **Missing author info:** Graceful fallback to initial letter avatar (existing behavior)
- **Reading time of 0:** Show "1 min read" minimum
- **Card style mid-transition:** The setting takes effect on next page load (5min cache TTL). No real-time push needed.

## Validation

1. Run `make sqlc-gen` after SQL query changes
2. Run `go build ./...` to verify Go compilation
3. Run `go test ./internal/feed/...` to verify feed service tests pass
4. Run `go test ./internal/admin/...` to verify settings tests pass
5. Check that the admin Settings → Appearance tab shows the new "Feed Card Style" select
6. Verify all three feed pages render correctly with both card styles
7. Run frontend lint: check for any TypeScript/Svelte errors
