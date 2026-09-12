# User hover card sweep — design

## Goal
Make the existing user hover card reusable across `apps/main` and apply it to all public content surfaces where another user (or source) is shown.

## Scope
Public/shared content surfaces only; skip the viewer's own profile UI, settings, auth forms, and onboarding.

## Components

- `lib/components/ui/EntityHoverChip.svelte` — moved from `lib/components/feed/`. Generic hover chip for `user | space | organization | source`.
- `lib/components/ui/UserHoverCard.svelte` — thin user-only wrapper (`type='user'` default, optional `trigger` snippet, safe fallback when no `id`/`slug`).

## Data gaps to close
- `ForumList.toFeedItem` must pass `author_username`.
- `AMAThread`, `Achievement`, `MutualAidPost` need `author_username` exposed/mapped where missing; if missing the hover card still renders but does not open.

## Surfaces to update (public content)

### Feed cards
- `FeedCard` — all styles (reddit, compact, magazine, grid)
- `DailyFeedCard` — update import path
- `ContentGridCard` — update import path

### Article bylines
- `ContentArticlePage`
- `blog/[slug]/+page.svelte`
- `story/[slug]/+page.svelte`
- `comparisons/[slug]/+page.svelte`
- `guide/[slug]/+page.svelte` (via `ContentArticlePage`)
- `content/[slug]/+page.svelte` (via `ContentArticlePage`)

### Comments & forum
- `CommentItem` — update import path
- `ReplyItem`
- `forum/[slug]/+page.svelte` thread header

### Search & discovery
- `search/+page.svelte`
- `trending/+page.svelte`
- `leaderboard/+page.svelte`
- `discover/+page.svelte`
- `users/+page.svelte`
- `lib/components/ui/DirectoryCard.svelte` (or call sites)
- `lib/components/shell/SearchAutocomplete.svelte`

### Spaces / orgs / tags / universities
- `tags/[slug]/+page.svelte`
- `spaces/[slug]/+page.svelte`
- `org/[slug]/+page.svelte`
- `universities/[slug]/+page.svelte`
- `lib/components/spaces/SpaceSidebar.svelte`
- `lib/components/spaces/SpaceFollowListModal.svelte`
- `lib/components/profile/OrgProfileSidebar.svelte`

### Other content
- `ama/+page.svelte` and `ama/[id]/+page.svelte`
- `mentors/+page.svelte` and `mentors/[username]/+page.svelte` (public profile)
- `achievements/+page.svelte` and `achievements/[slug]/+page.svelte`
- `digest/DigestView.svelte`
- `mutual-aid/+page.svelte` and `mutual-aid/[id]/+page.svelte`
- `certificates/[id]/+page.svelte`
- `services/[slug]/+page.svelte` and `services/+page.svelte`
- `universities/[slug]/+page.svelte` and `universities/+page.svelte`
- `jobs/[slug]/+page.svelte` (if it exposes a poster) and any other public listing that shows a user
- `japa-stage/+page.svelte`

### Dashboard / social
- `dashboard/organizations/[id]/+page.svelte`
- `messages/+page.svelte`
- `messages/requests/+page.svelte`
- `lib/components/hub/dashboard/views/RelationshipsView.svelte` (users only)
- `lib/components/profile/FollowListModal.svelte` (users only)
- `lib/components/shell/SidebarWidgetRenderer.svelte` (leaderboard / user-card widgets)

## Source support
`EntityHoverChip.type` gains `source`, with `href`/API routing appropriate for `/sources/{slug}`. Source bylines in `FeedCard` and `/sources` pages will be covered.

## Fallback behavior
When `slug` (or `id` for source) is missing, `UserHoverCard` renders the supplied `trigger` or a plain chip/row without opening a hover card, so no profile link is broken.

## Verification
- `bun run check` and `bun run build` in `apps/main`.
- Spot-check `/blog`, `/stories`, `/guides`, `/explore`, `/search`, `/u/{username}`, and `/sources` in the dev server.
