# Thread Rename & Unified Discussions — Design

Date: 2026-11-08 · Status: approved (user Q&A resolved)

## Model

`/thread/{slug}` is the single place where discussion happens.

- A thread is either a **forum_thread** (§08 engine — votes, replies, polls,
  solve; rendered exactly as the old `/forum/{slug}` page) or an **entity
  thread**: an embed of another entity (any contents row, a wiki page, or a
  tool) plus the §49 comment engine below it.
- Entity pages (`/{content_type}/{slug}`, `/wiki/{space}/{page}`,
  `/tools/{tool}`) do not host the full comment engine. Their bottom block
  shows **5 parent comments** (children render inline; replies still work),
  sorted by **Popular | Latest** tabs, plus a CTA: "Join this Conversation"
  when comments exist, "Start a conversation" when they don't. The CTA opens
  `/thread/{slug}`.
- "For You" and "Following" feed cards open `/thread/{slug}` on whole-card
  click. Other surfaces keep canonical `contentHref` targets.

## Route map (frontend)

| Old | New |
|---|---|
| `/forum` | `/thread` (same list page: Latest \| Top \| Featured) |
| `/forum/{slug}` | `/thread/{slug}` (dispatches forum vs entity thread) |
| `/forum/{category}/new` | `/thread/{category}/new` |
| `/forum/feed.xml`, `/forum/rss.xml` | `/thread/feed.xml`, `/thread/rss.xml` |

Legacy `/forum*` URLs 301 → `/thread*` (SvelteKit `+server.ts` catch-all).

## Backend

### API rename

- All `/api/v1/forum/*` routes renamed to `/api/v1/thread/*`
  (`/thread`, `/thread/{slug}`, `/thread/{id}/replies`, `/thread/{id}/solve`,
  `/thread/{id}/poll`, `/thread/polls/{id}`, `/thread/tags`,
  `/thread/bookmarks`, `/thread/ama/*`, `/thread/feed.xml`).
- A path-rewrite middleware registered in `NewServer` before routing maps
  `/api/v1/forum[/...]` → `/api/v1/thread[/...]` so existing API consumers
  (mobile app, external clients) keep working. RBAC nodes stay `forum.*`
  (route paths, not permission keys, are renamed).
- User-facing URLs emitted by the backend (RSS item links, sitemap) switch
  to `/thread/...`.

### Thread anchors (wiki pages + tools)

Comments key on `contents.id`; wiki pages and tools have no contents row.
New `thread_anchors` table binds a lazily-created `discussion` contents row
to a non-content entity:

```sql
CREATE TABLE thread_anchors (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id uuid NOT NULL UNIQUE REFERENCES contents(id) ON DELETE CASCADE,
    entity_type text NOT NULL,          -- 'tool' | 'wiki_page'
    entity_key text NOT NULL,           -- tool slug | wiki_pages.id
    entity_url text NOT NULL,           -- /tools/{key} | /wiki/{space}/{page}
    entity_title text NOT NULL,
    entity_summary text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (entity_type, entity_key)
);
```

Endpoints (mounted under plural `/threads` to avoid shadowing `/thread/{slug}`):

- `GET /api/v1/threads/anchor/{entityType}/{entityKey}` — public resolve →
  `{content_id, slug, entity_*}` or 404.
- `POST /api/v1/threads/anchor` `{entity_type, entity_key}` — auth; get-or-
  create. Validates the entity (tool slug against the canonical tool list
  mirroring `seo.mainAppToolPaths`; wiki page by UUID → resolves space slug,
  page slug, title, URL server-side). Creates a `discussion` contents row
  (`"Discussing: {entity_title}"`, same hash-uniqueness trick as
  StartConversation) via the shared content creator, then the anchor row.

### Content detail enrichment

`GET /content/{slug}` gains two optional envelope fields so `/thread/{slug}`
can render the embed without extra round-trips:

- `thread_anchor` — present on anchor rows (tool/wiki entity descriptors).
- `thread_parent` — present on `discussion` rows that are the link target of
  a `content_thread_links` row (i.e. created by §49 StartConversation):
  `{slug,title,content_type,excerpt,cover_image_url}` of the parent content.

## Frontend

### `/thread/{slug}` dispatch

Load tries `GET /thread/{slug}` (forum API) first → `kind: 'forum'` payload
identical to today. On 404 → `GET /content/{slug}` → `kind: 'embed'`:

- **anchor row** → `ThreadEmbedCard` from `thread_anchor` (title, summary,
  CTA → `entity_url`).
- **§49 discussion row** → `ThreadEmbedCard` from `thread_parent` (CTA →
  `contentHref(parent)`).
- **any other contents row** → `ThreadEmbedCard` of itself (title, cover,
  excerpt; CTA → canonical `contentHref`).

Below the card: the existing `CommentThread` engine (`contentId` = the row's
id — comments post directly to the entity/anchor row).

`ThreadEmbedCard` = title + embed/media + excerpt + "View …" CTA per the
user's description; links to the canonical page.

### `EntityDiscussions` bottom block (new component)

Props: `contentId?`, `slug?`, `entityType?`, `entityKey?`, `authed`.

- Contents-backed: `GET /content/{id}/comments?sort=top|new&page_size=5`.
- Anchor-backed: `GET /threads/anchor/{type}/{key}` → comments via the
  anchor's `content_id`; no anchor → only the "Start a conversation" CTA.
- Tabs Popular → `sort=top`; Latest → `sort=new` (client refetch).
- ≤5 parent comments via `CommentItem` (children + inline reply unchanged);
  no top-level composer — new top-level discussion lives on `/thread/{slug}`.
- CTA: `Join this Conversation` → `/thread/{slug}`; `Start a conversation` →
  creates anchor if needed (`POST /threads/anchor`) then navigates. Guests →
  `/login?redirect=…`.

Mounted on: `ContentArticlePage`, blog/story/comparisons/achievements/
roadmap singles (replacing the inline `CommentThread`/`DiscussionPanel`),
`/wiki/{space}/{page}` (`entityType=wiki_page`, `entityKey=page.id`), and
every `/tools/{tool}` page (`entityType=tool`, `entityKey={tool slug}`).

### For You / Following

`AuthedHomeView` → `FeedList` → `FeedCard`/`DailyFeedCard` get a `threadLinks`
prop; when set, card href = `/thread/{slug}` regardless of content_type.

### Links and labels

- `contentLink.ts`: `forum_thread` and `discussion` → `/thread`.
- Left nav + page titles/i18n values: "Forum" → "Threads" (keys unchanged).
- All `api.*('/forum/…')` call sites → `/thread/…`.

## Out of scope

- Permission-key renames (`forum.*` RBAC nodes unchanged).
- Mobile app route renames (legacy API alias covers runtime).
