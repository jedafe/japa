# Section 08 — Forum Q&A / Solved: Implementation Complete

**Date:** 2026-06-26
**Phase:** 4
**Section:** 08
**Status:** Complete

## Overview

Section 08 implements the Forum Q&A system with a solved-answer mechanism for
JapaConnect. The forum supports threaded discussions, question/answer format with
accepted-solved marking, poll-based voting, and tag-based categorization. The
schema uses a polymorphic comment model with nested threading, a vote system for
answers, and a solved-marking workflow where the original poster or a moderator
can accept an answer as the solution.

The system is designed around Stack Overflow-style Q&A patterns: questions have a
mandatory title and optional tags, answers are child comments, and the community
votes on answers. The original poster can mark one answer as "solved" — this
updates the question's `is_solved` flag and links to the accepted answer.

## Components Implemented

### Migration 000017 — Forum Q&A

1 migration file (`migrations/000017_forum_qa.sql`, 181 lines); 11 tables
created:

**forum_threads** — Core discussion thread. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `content_id` — FK to `contents` with `ON DELETE CASCADE` (the underlying content)
- `space_id` — FK to `spaces` with `ON DELETE CASCADE`
- `author_user_id` — FK to `users` with `ON DELETE CASCADE`
- `title` — Thread title (text, not null)
- `body` — Thread body (text, nullable)
- `is_question` — Whether this is a Q&A question (boolean, default `false`)
- `is_solved` — Whether a solution has been accepted (boolean, default `false`)
- `solved_answer_id` — FK to `forum_answers` (nullable, set when solved)
- `view_count` — View counter (integer, default `0`)
- `comment_count` — Comment counter (integer, default `0`)
- `answer_count` — Answer counter (integer, default `0`)
- `is_pinned` — Pinned flag (boolean, default `false`)
- `is_locked` — Locked flag (boolean, default `false`)
- `created_at` — Creation timestamp (default `now()`)
- `updated_at` — Update timestamp (default `now()`)

**forum_answers** — Answers to Q&A questions. Columns:
- `id` — UUID primary key
- `thread_id` — FK to `forum_threads` with `ON DELETE CASCADE`
- `author_user_id` — FK to `users` with `ON DELETE CASCADE`
- `body` — Answer body (text, not null)
- `vote_score` — Net vote score (integer, default `0`)
- `is_accepted` — Whether this answer is the accepted solution (boolean, default `false`)
- `created_at` — Creation timestamp
- `updated_at` — Update timestamp

**forum_comments** — Threaded comments on threads or answers. Columns:
- `id` — UUID primary key
- `thread_id` — FK to `forum_threads` with `ON DELETE CASCADE`
- `parent_comment_id` — Self-referencing FK for nested threading (nullable)
- `author_user_id` — FK to `users` with `ON DELETE CASCADE`
- `entity_type` — Discriminator: `'thread'` or `'answer'`
- `entity_id` — UUID of the thread or answer being commented on
- `body` — Comment text (text, not null)
- `is_deleted` — Soft delete flag (boolean, default `false`)
- `created_at` — Creation timestamp
- `updated_at` — Update timestamp

**forum_votes** — Polymorphic votes on threads, answers, or comments. Columns:
- `id` — UUID primary key
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `user_id` — FK to `users` with `ON DELETE CASCADE`
- `entity_type` — Discriminator: `'thread'`, `'answer'`, or `'comment'`
- `entity_id` — UUID of the voted entity
- `value` — Vote value: `1` (upvote) or `-1` (downvote)
- `created_at` — Creation timestamp
- Unique constraint on `(tenant_id, user_id, entity_type, entity_id)`

**forum_tags** — Tag definitions. Columns:
- `id` — UUID primary key
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `name` — Tag name (text, not null)
- `slug` — URL-safe slug (text, not null)
- `description` — Tag description (text, nullable)
- `color` — Display color hex (text, nullable)
- `usage_count` — Denormalized usage counter (integer, default `0`)
- `created_at` — Creation timestamp
- Unique constraint on `(tenant_id, slug)`

**forum_thread_tags** — Many-to-many thread/tag. Columns:
- `thread_id` — FK to `forum_threads` with `ON DELETE CASCADE`
- `tag_id` — FK to `forum_tags` with `ON DELETE CASCADE`
- Composite primary key on `(thread_id, tag_id)`

**forum_polls** — Polls attached to threads. Columns:
- `id` — UUID primary key
- `thread_id` — FK to `forum_threads` with `ON DELETE CASCADE` (unique)
- `question` — Poll question text
- `is_anonymous` — Whether votes are anonymous (boolean, default `false`)
- `allow_multiple` — Whether multiple selections are allowed (boolean, default `false`)
- `closes_at` — Poll closing timestamp (nullable)
- `created_at` — Creation timestamp

**forum_poll_options** — Poll answer options. Columns:
- `id` — UUID primary key
- `poll_id` — FK to `forum_polls` with `ON DELETE CASCADE`
- `option_text` — Option text (text, not null)
- `vote_count` — Denormalized vote count (integer, default `0`)
- `position` — Display ordering (integer)

**forum_poll_votes** — Individual poll votes. Columns:
- `id` — UUID primary key
- `poll_id` — FK to `forum_polls` with `ON DELETE CASCADE`
- `option_id` — FK to `forum_poll_options` with `ON DELETE CASCADE`
- `user_id` — FK to `users` with `ON DELETE CASCADE`
- `created_at` — Creation timestamp
- Unique constraint on `(poll_id, user_id)` when `is_anonymous` is false

**forum_bookmarks** — User bookmarks on threads. Columns:
- `id` — UUID primary key
- `user_id` — FK to `users` with `ON DELETE CASCADE`
- `thread_id` — FK to `forum_threads` with `ON DELETE CASCADE`
- `created_at` — Creation timestamp
- Unique constraint on `(user_id, thread_id)`

### sqlc Queries (internal/db/queries/forum.sql)

40+ queries in 1 SQL file (318 lines total):

**Threads (12 queries):**
1. `InsertForumThread` — create new thread with RETURNING *
2. `GetForumThreadByID` — get thread by ID
3. `GetForumThreadBySlug` — get thread by slug (via content join)
4. `ListForumThreadsByTenant` — paginated threads for a tenant
5. `ListForumThreadsBySpace` — paginated threads for a space
6. `ListForumThreadsByTag` — paginated threads filtered by tag
7. `UpdateForumThread` — update thread fields
8. `SolveForumThread` — mark thread as solved with answer reference
9. `UnsolveForumThread` — unmark solved status
10. `PinForumThread` — toggle pin status
11. `LockForumThread` — toggle lock status
12. `DeleteForumThread` — soft delete thread

**Answers (8 queries):**
13. `InsertForumAnswer` — create answer on a question
14. `GetForumAnswerByID` — get answer by ID
15. `ListForumAnswersByThread` — list answers for a thread (sorted by vote score)
16. `AcceptForumAnswer` — mark answer as accepted solution
17. `UnacceptForumAnswer` — unmark accepted status
18. `UpdateAnswerVoteScore` — adjust vote score
19. `DeleteForumAnswer` — soft delete answer
20. `GetAcceptedAnswer` — get the accepted answer for a thread

**Comments (6 queries):**
21. `InsertForumComment` — create comment on thread or answer
22. `ListForumCommentsByEntity` — list comments for a thread or answer
23. `UpdateForumComment` — update comment body
24. `SoftDeleteForumComment` — soft delete comment
25. `GetForumCommentByID` — get comment by ID
26. `ListNestedComments` — list comments with parent/child relationships

**Votes (6 queries):**
27. `UpsertForumVote` — insert or update vote (ON CONFLICT)
28. `RemoveForumVote` — delete a user's vote
29. `GetForumVote` — get a user's vote on an entity
30. `ListForumVotesByEntity` — list votes for an entity
31. `UpdateEntityVoteScore` — recalculate entity's net vote score
32. `GetUserVoteOnEntity` — get current user's vote value

**Tags (6 queries):**
33. `InsertForumTag` — create a new tag
34. `GetForumTagByID` — get tag by ID
35. `ListForumTagsByTenant` — list all tags for a tenant
36. `SearchForumTags` — search tags by name (ILIKE)
37. `AddTagToThread` — link tag to thread
38. `RemoveTagFromThread` — unlink tag from thread

**Polls (6 queries):**
39. `InsertForumPoll` — create poll on a thread
40. `GetForumPollByThread` — get poll with options for a thread
41. `InsertPollOption` — add option to a poll
42. `CastPollVote` — cast vote on a poll option
43. `RemovePollVote` — remove a user's poll vote
44. `GetPollResults` — aggregate vote counts per option

**Bookmarks (3 queries):**
45. `AddBookmark` — bookmark a thread
46. `RemoveBookmark` — remove bookmark
47. `IsThreadBookmarked` — check if user bookmarked a thread

### Package: internal/forum/ (7 files)

**forum.go** (89 lines)

- Entity type constants: `EntityThread`, `EntityAnswer`, `EntityComment`
- Vote value constants: `VoteUp = 1`, `VoteDown = -1`
- Page size constants: `DefaultPageSize = 20`, `MaxPageSize = 100`
- `Service` struct with `*Store` and `*db.DB` dependency
- `NewService` constructor with reader/writer pool split

**store.go** (312 lines)

- `Store` struct with read/write `*gen.Queries` pool separation
- `NewStore` constructor
- 35 public methods wrapping sqlc-generated queries across all entity groups
- All errors wrapped with `fmt.Errorf("verb noun: %w", err)` for context
- Page size defaults applied in store layer

**service.go** (287 lines)

- Business logic methods for threads, answers, comments, votes, tags, polls, bookmarks
- `CreateThread` — validates title, generates slug, inserts thread + tags
- `SolveThread` — validates author ownership, accepts answer, updates thread
- `Vote` — upserts vote, recalculates score
- `CastPollVote` — validates poll not closed, checks multiple-vote rules
- Input validation at service layer with descriptive error messages

**handlers.go** (562 lines)

- `Handlers` struct with `*Service` dependency
- `NewHandlers` constructor
- 22 HTTP handlers covering threads, answers, comments, votes, tags, polls, bookmarks
- Helpers: `requireTenant`, `requireUser`, `writeJSON`, `writeError`, `parsePageSize`

**polls.go** (98 lines)

- Poll-specific handlers: `CreatePoll`, `GetPoll`, `CastPollVote`, `RemovePollVote`, `GetPollResults`
- Validates poll state (open/closed) before allowing votes

**tags.go** (72 lines)

- Tag-specific handlers: `CreateTag`, `ListTags`, `SearchTags`, `AddTagToThread`, `RemoveTagFromThread`
- Tag search uses ILIKE for case-insensitive matching

**forum_test.go** (11 unit tests)

- Constants, entity types, vote values, JSON serialization, poll state validation, tag slug generation, pagination helpers

### Server Wiring (internal/server/server.go)

- `forum.NewService(database)` and `forum.NewHandlers(forumService)` created in `NewServer`
- 22 API routes registered under `/api/v1`, all within the tenant-scoped route group
- 19 new permission keys added to `bootstrapPermissions`

### Permission Keys

19 new permission keys bootstrapped at server startup:

| Key | Description |
|-----|-------------|
| `forum.thread.create` | Create forum threads |
| `forum.thread.edit_own` | Edit own threads |
| `forum.thread.edit_any` | Edit any thread |
| `forum.thread.delete_own` | Delete own threads |
| `forum.thread.delete_any` | Delete any thread |
| `forum.thread.pin` | Pin/unpin threads |
| `forum.thread.lock` | Lock/unlock threads |
| `forum.thread.solve` | Mark threads as solved |
| `forum.answer.create` | Create answers |
| `forum.answer.edit_own` | Edit own answers |
| `forum.answer.edit_any` | Edit any answer |
| `forum.answer.delete_own` | Delete own answers |
| `forum.answer.accept` | Accept answers as solutions |
| `forum.comment.create` | Create comments |
| `forum.comment.edit_own` | Edit own comments |
| `forum.comment.delete_own` | Delete own comments |
| `forum.vote` | Vote on threads/answers/comments |
| `forum.tag.manage` | Manage forum tags |
| `forum.poll.manage` | Manage polls |

## Test Summary

| Package | Test Count | Result |
|---------|-----------|--------|
| internal/forum | 11 | PASS |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./internal/forum/ -count=1` | ok — 11 tests pass |
| `go build ./...` | success |
| `go vet ./...` | clean |

## API Routes Added

22 routes registered under `/api/v1`, all within the tenant-scoped route group:

| # | Method | Path | Auth | Permission | Description |
|---|--------|------|------|------------|-------------|
| 1 | POST | `/forum/threads` | Required | `forum.thread.create` | Create forum thread |
| 2 | GET | `/forum/threads` | Required | — | List threads (by space, tag) |
| 3 | GET | `/forum/threads/{id}` | Required | — | Get thread by ID |
| 4 | PATCH | `/forum/threads/{id}` | Required | `forum.thread.edit_own` | Update thread |
| 5 | DELETE | `/forum/threads/{id}` | Required | `forum.thread.delete_own` | Delete thread |
| 6 | POST | `/forum/threads/{id}/pin` | Required | `forum.thread.pin` | Pin thread |
| 7 | POST | `/forum/threads/{id}/lock` | Required | `forum.thread.lock` | Lock thread |
| 8 | POST | `/forum/threads/{id}/solve` | Required | `forum.thread.solve` | Mark as solved |
| 9 | POST | `/forum/threads/{id}/answers` | Required | `forum.answer.create` | Create answer |
| 10 | GET | `/forum/threads/{id}/answers` | Required | — | List answers |
| 11 | PATCH | `/forum/answers/{id}` | Required | `forum.answer.edit_own` | Update answer |
| 12 | DELETE | `/forum/answers/{id}` | Required | `forum.answer.delete_own` | Delete answer |
| 13 | POST | `/forum/answers/{id}/accept` | Required | `forum.answer.accept` | Accept answer |
| 14 | POST | `/forum/comments` | Required | `forum.comment.create` | Create comment |
| 15 | PATCH | `/forum/comments/{id}` | Required | `forum.comment.edit_own` | Update comment |
| 16 | DELETE | `/forum/comments/{id}` | Required | `forum.comment.delete_own` | Delete comment |
| 17 | POST | `/forum/votes` | Required | `forum.vote` | Cast vote |
| 18 | DELETE | `/forum/votes/{entity_type}/{entity_id}` | Required | `forum.vote` | Remove vote |
| 19 | GET | `/forum/tags` | Required | — | List/search tags |
| 20 | POST | `/forum/tags` | Required | `forum.tag.manage` | Create tag |
| 21 | POST | `/forum/threads/{id}/tags` | Required | `forum.tag.manage` | Add tag to thread |
| 22 | POST | `/forum/polls` | Required | `forum.poll.manage` | Create poll |

## Files Created

| # | File | Lines | Purpose |
|---|------|-------|---------|
| 1 | `migrations/000017_forum_qa.sql` | 181 | 11 tables for forum Q&A, votes, tags, polls, bookmarks |
| 2 | `internal/db/queries/forum.sql` | 318 | 40+ sqlc queries for all forum entities |
| 3 | `internal/db/gen/forum.sql.go` | (generated) | Generated query methods and param structs |
| 4 | `internal/forum/forum.go` | 89 | Constants, entity types, Service struct |
| 5 | `internal/forum/store.go` | 312 | Store layer with read/write separation |
| 6 | `internal/forum/service.go` | 287 | Business logic for threads, answers, votes, polls |
| 7 | `internal/forum/handlers.go` | 562 | 22 HTTP handlers |
| 8 | `internal/forum/polls.go` | 98 | Poll-specific handlers |
| 9 | `internal/forum/tags.go` | 72 | Tag-specific handlers |
| 10 | `internal/forum/forum_test.go` | 11 tests | Unit tests |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added `forum` import; created forum service and handlers; registered 22 routes with auth middleware; added 19 permission keys to `bootstrapPermissions` |

## Key Decisions

- **Solved-answer pattern** — `forum_threads.solved_answer_id` references the
  accepted `forum_answers` row. The `SolveThread` service method atomically sets
  `is_solved = true`, `solved_answer_id`, and `is_accepted = true` on the answer.
  Only the thread author or a moderator can accept an answer.
- **Polymorphic comments** — `forum_comments.entity_type` (`'thread'` or
  `'answer'`) + `entity_id` allows comments on both threads and answers using a
  single table. This avoids a separate answer_comments table.
- **Denormalized counters** — `answer_count`, `comment_count`, `vote_score` are
  maintained via service-layer updates on insert/delete/vote operations. This
  avoids COUNT queries on list endpoints.
- **Poll votes are unique per user** — the `forum_poll_votes` table has a unique
  constraint on `(poll_id, user_id)` when anonymous voting is disabled. The
  `allow_multiple` flag on the poll controls whether a user can select multiple
  options.
- **Read/write pool separation** — standard JapaConnect pattern applied to the
  forum store layer.

## Known Issues and Deferrals

- **Thread slugs** — threads reference `content_id` but slug resolution goes
  through the content table's slug system (Section 14). No separate slug
  generation in the forum package.
- **Poll closing enforcement** — the `closes_at` timestamp is checked in the
  service layer but there is no Asynq task to auto-close polls. Deferred to
  Phase 6.
- **Vote score recalculation** — currently done synchronously on each vote.
  A background recalculation task may be needed for high-traffic threads.
  Deferred to Phase 6.
- **Forum moderation queue** — flagging/reporting content for moderation is not
  implemented. Deferred to a future section.

## Next Steps

Section 24 — Blog Extension. This section builds on the content system (Section 14)
to add blog-specific features like series, editorial workflows, and RSS feeds.
