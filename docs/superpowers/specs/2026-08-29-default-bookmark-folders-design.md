# Design — Default Bookmark Folders by Content Type

**Date:** 2026-08-29  
**Author:** Devin (via user request)  
**Status:** Approved for implementation  
**JEOS refs:** §08 (forum bookmarks), §14 (content type system), §50 (user dashboard / me/bookmarks)  

## Goal

When a user bookmarks any item on the platform, the bookmark is automatically filed into a default folder named for the item's content or entity type. The behavior is visible and usable on `http://localhost:3000/dashboard/bookmarks`.

## Decisions from clarification

- Forum threads and replies default to a folder named **"discussions"**.
- Default folders are **pre-seeded at user registration**.
- For existing users, the folder is **auto-created on the first matching bookmark**.
- Mapping is a **code-level lookup table**, not a system setting.
- The mapping covers **every valid content/entity type** that may be bookmarked, even those without a dedicated UI today.

## Default folder mapping

| Source type | Folder name | Notes |
|---|---|---|
| `content` with `content_type = blog_post` | `blog` | Detected by looking up the `contents` row. |
| `content` with `content_type = discussion` | `discussions` | |
| `content` with `content_type = guide` | `guides` | |
| `content` with `content_type = page` | `pages` | |
| `content` with `content_type = comparison` | `comparisons` | |
| `content` with `content_type = event` | `events` | |
| `content` with `content_type = job_listing` | `jobs` | |
| `content` with `content_type = story` | `stories` | |
| `content` with `content_type = achievement` | `achievements` | |
| `content` with `content_type = service_listing` | `services` | |
| `entity_type = academy_track` | `learn` | |
| `entity_type = doc_page` | `docs` | |
| `entity_type = forum_thread` (or legacy `thread`) | `discussions` | Stored as `forum_thread` in the unified table. |
| `entity_type = forum_reply` (or legacy `reply`) | `discussions` | Stored as `forum_reply` in the unified table. |
| `entity_type = space` | `spaces` | |
| `entity_type = tag` | `tags` | |
| `entity_type = comment` | `comments` | |
| `entity_type = user` / `organization` / `professional_profile` | — | No current bookmark UI; leave unfiled (`folder='default'`, `folder_id` NULL). |
| Any unrecognized type | — | Unfiled. |

The folder string and `folder_id` are set together when a default is resolved. Folders are created with `position` starting at `0` and incrementing in the order listed above.

## Architecture

A single `DefaultFolderResolver` lives in `internal/bookmarkfolders`. It is responsible for:

1. Resolving an entity/content type to a folder name.
2. Ensuring a `bookmark_folders` row exists for `(tenant_id, user_id, name)`.
3. Returning the folder name and UUID.

The resolver is called from the three bookmark creation entry points:

- `internal/content/store.go:CreateBookmark` — for `POST /content/{id}/bookmark` and persona `CreateBookmark`.
- `internal/users/service.go:CreateBookmark` — for `POST /me/bookmarks`.
- `internal/forum/service.go:AddBookmark` — for `POST /forum/bookmarks`.

### Content-type detection

- For `entity_type = content` or `blog_post`, the resolver must look up the `contents.content_type` column.
- For `forum` bookmarks, legacy `thread`/`reply` are normalized to `forum_thread`/`forum_reply` before mapping.
- For all other `entity_type` values, the map is used directly.

### Folder pre-seeding

`internal/bookmarkfolders` exposes `CreateDefaultFoldersForUser(ctx, ID, userID)` that inserts the full set of mapped folder names in a single call. This is invoked from:

- `internal/auth/handlers.go` after `CreateUser`.
- `internal/auth/oauth_handlers.go` after OAuth registration.
- `internal/admin/users.go` after admin user creation.

If a folder already exists (e.g. a previous partial call), it is skipped.

### Auto-create on first bookmark

When a bookmark is created with no explicit `folder`/`folder_id`, the resolver computes the default. If the folder does not exist, it is created lazily, so existing users also get the benefit without a backfill.

## Forum consolidation

The platform currently stores forum bookmarks in a separate `forum_bookmarks` table. To make the default-folder and dashboard-filter behavior uniform, forum bookmarks will be consolidated into the existing `bookmarks` table.

- Migration `000XXX_consolidate_forum_bookmarks.up.sql`:
  - Backfill existing `forum_bookmarks` rows into `bookmarks`:
    - `thread` -> `entity_type = 'forum_thread'`
    - `reply` -> `entity_type = 'forum_reply'`
  - Skip rows that would violate the `UNIQUE(user_id, entity_type, entity_id)` index.
- Update `internal/forum/store.go` to read from and write to `bookmarks` using the new entity types.
- Update `internal/forum/service.go` and `internal/forum/handlers.go` to use the consolidated store methods.
- Update the `ListUserBookmarks` query in `internal/db/queries/users.sql` to:
  - Remove the `forum_bookmarks` `UNION ALL`.
  - Add `forum_replies` joins for `forum_reply` title/slug resolution (reply links to parent thread).
- Update `apps/main/src/routes/(app)/dashboard/bookmarks/+page.svelte` to handle `forum_reply` URLs (`/forum/{threadSlug}#reply-{replyId}`).

## API behavior

- `POST /content/{id}/bookmark` continues to accept `folder` and `folder_id` overrides. When omitted, the default folder is resolved from `content_type`.
- `POST /me/bookmarks` continues to accept `entity_type`, `entity_id`, `folder`, and `folder_id`. When omitted, the default is resolved.
- `POST /forum/bookmarks` continues to accept `entity_type` (`thread`/`reply`) and `entity_id`. When omitted, the default is resolved to the `discussions` folder.
- `GET /me/bookmarks` continues to support `?folder_id=` filtering; after consolidation it returns forum bookmarks through the same path.
- `GET /me/bookmark-folders` returns the default folders along with any user-created folders.

## UI changes

- `dashboard/bookmarks/+page.svelte`: extend `entityUrl` and `entityLabel` for `forum_reply`.
- No changes to the folder sidebar or creation UI; it already consumes the existing `/me/bookmark-folders` list and `?folder_id=` filter.

## Migrations

1. `000XXX_consolidate_forum_bookmarks.up.sql` — backfill forum bookmarks into `bookmarks`.
2. (No new table columns required in `bookmarks`; `folder_id` already exists from 000172.)
3. Verify 000505 has dropped the old `bookmarks_entity_type_check` constraint.

## Catalog updates

- `api-routes-catalog.md` — no new routes; existing routes change internal behavior only.
- `valkey-keys-catalog.md` — no new Valkey keys.
- `asynq-tasks.md` — no new tasks.
- `system-settings-catalog.md` — no new settings.

## Testing plan

- Unit tests for `DefaultFolderResolver` covering each mapping and unknown fallback.
- Handler tests for `POST /content/{id}/bookmark`, `POST /me/bookmarks`, and `POST /forum/bookmarks` verifying the correct default folder is created and assigned.
- Migration tests for forum bookmark backfill (idempotency and conflict handling).
- Svelte build/typecheck for `dashboard/bookmarks`.
- `go build ./...` and `go vet ./...` clean.

## Open questions / none

All clarification questions resolved before this design was written.
