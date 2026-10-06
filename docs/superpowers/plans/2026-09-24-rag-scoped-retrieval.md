# RAG Scoped Retrieval — Implementation Plan

**Spec:** `docs/superpowers/specs/2026-09-24-rag-scoped-retrieval-design.md`
**Branch:** `main`
**Preconditions already shipped:** verified-public post-filter on
`/public/search/semantic` (`d6448fe8`), `rag.search` removed from the
API-key allowlist (`c454eb42` + jeos `337efec`).

JEOS rules apply throughout: sqlc gen files never hand-edited (edit
`internal/db/queries/rag.sql`, run `make sqlc-gen`), `jc:` keys cataloged,
settings keys from the catalog, JEOS updated **before** code.

---

## Task 1 — JEOS (docs-first)

Update `.jeos/sections/69-rag-knowledge-layer.md`:

- New "Scoped retrieval" subsection: the three scope columns
  (`visibility`, `space_id`, `author_id`), the retrieval rule (public for
  all; own rows always; member-space rows for members except
  author-private content), `rag.scoped_retrieval.enabled` kill switch
  (default true; false = public-only fallback).
- Corpus completeness: indexers embed all entities including
  private/members; visibility is enforced at retrieval, not at index time.
- Transition-hook table: space update → reconcile space + its thread
  embeddings; content unpublish/visibility change → update or purge;
  thread delete → purge.

Update `.jeos/conventions/system-settings-catalog.md`: add
`rag.scoped_retrieval.enabled` (bool, default true).

`.jeos/sections/119-mcp-server.md` — already documents the `rag.search`
key-caller removal; add one line noting retrieval is now caller-scoped for
session callers (tool unchanged for them).

Commit in the `.jeos` submodule, then bump the parent repo pointer.

---

## Task 2 — Migration + sqlc queries

**Migration `000728_content_embeddings_scope.up.sql`** (verify still free
at implementation time — take the next number if not):

```sql
ALTER TABLE content_embeddings
    ADD COLUMN visibility text NOT NULL DEFAULT 'public',
    ADD COLUMN space_id   uuid NULL,
    ADD COLUMN author_id  uuid NULL;

ALTER TABLE content_embeddings
    ADD CONSTRAINT content_embeddings_visibility_chk
    CHECK (visibility IN ('public','members','private'));

UPDATE content_embeddings ce
SET visibility = s.visibility, space_id = s.id
FROM spaces s
WHERE ce.entity_type = 'space' AND ce.entity_id = s.id;

UPDATE content_embeddings ce
SET visibility = c.visibility, space_id = c.space_id, author_id = c.author_id
FROM contents c
WHERE ce.entity_type = 'content' AND ce.entity_id = c.id;

UPDATE content_embeddings ce
SET visibility = sp.visibility, space_id = ft.space_id,
    author_id = ft.author_user_id
FROM forum_threads ft
JOIN spaces sp ON sp.id = ft.space_id
WHERE ce.entity_type = 'forum_thread' AND ce.entity_id = ft.id;

UPDATE content_embeddings SET visibility = 'private'
WHERE entity_type = 'forum_thread' AND space_id IS NULL;

ALTER TABLE content_embeddings ALTER COLUMN visibility DROP DEFAULT;
```

Down migration drops the constraint and three columns.

**Queries** — edit `internal/db/queries/rag.sql`, regenerate:

- `ListMemberSpaceIDs(userID)` — `SELECT space_id FROM space_members WHERE
  user_id = $1 AND status = 'approved'` — the `status='approved'` filter is
  mandatory (`CheckSpaceMembership`, `spaces.sql.go:255`, requires it;
  pending members must not gain retrieval scope).
- `UpdateEmbeddingScope(entityType, entityID, visibility, spaceID,
  authorID)` — single-entity scope refresh.
- `UpdateThreadEmbeddingsBySpace(spaceID, visibility)` — bulk update for
  space visibility flips.
- `GetContentEmbeddingMeta(contentID)` — title, body, status, visibility,
  space_id, author_id (used by the worker embed path).
- Extend **`SearchContentEmbeddings` AND `SearchContentEmbeddingsGeoFiltered`**
  — `Retrieve` branches between them (`service.go:243`); both need scope.

Canonical semantics (`internal/content/visibility.VisibilityFilter` —
verified against `ListContents`, `contents.sql.go:2220`): `members` =
any authenticated viewer (a bool flag, NOT a space-membership join);
`private` = author or privileged; `public` = everyone. Space entities
additionally: `private` space = approved members only.

`SearchContentEmbeddings` (all types):

```sql
AND (
    ce.visibility = 'public'
    OR ce.author_id = $viewer
    OR (ce.visibility = 'members' AND $viewer IS NOT NULL)
    OR (ce.visibility = 'private' AND $can_see_private)
    OR (ce.entity_type IN ('space','forum_thread')
        AND ce.visibility = 'private'
        AND ce.space_id = ANY($member_ids))
)
```

`SearchContentEmbeddingsGeoFiltered` (content-only — no space clause):

```sql
AND (
    ce.visibility = 'public'
    OR ce.author_id = $viewer
    OR (ce.visibility = 'members' AND $viewer IS NOT NULL)
    OR (ce.visibility = 'private' AND $can_see_private)
)
```

Keep the existing per-type EXISTS guards (published/non-deleted/active) —
they catch deletion races between index and query.

---

## Task 3 — `rag.Service` refactor + caller updates

`internal/rag/service.go`:

- `type EntityRef struct { Type string; ID, SpaceID, AuthorID pgtype.UUID; Visibility string }`
- `type RetrievalScope struct { ViewerID pgtype.UUID; CanSeePrivate bool; MemberSpaceIDs []pgtype.UUID }` — mirrors `visibility.VisibilityFilter` + membership.
- `IndexEntity(ctx, ref EntityRef, body, language string)` — replaces the
  `(entityType, entityID, body, language)` signature; writes scope columns.
  Interface decls at `spaces/handlers.go:62`, `forum/handlers.go:64`,
  `learning/handlers.go:43`, `wiki/handlers.go:79`, `worker/rag_indexer.go:28`
  all ripple.
- `Retrieve(ctx, query, language string, geoSpaceID *pgtype.UUID,
  viewerID pgtype.UUID)` — builds `RetrievalScope` internally:
  - `viewerID` invalid → empty member list (public-only).
  - `viewerID` valid → `ListMemberSpaceIDs` (approved only).
  - `rag.scoped_retrieval.enabled == false` → force empty scope.
  - Existing `rag.enabled` / embedder / empty-query early-outs unchanged.
- **Cache key (`ragQueryCacheKey`, `service.go:81`) gains a scope hash** —
  `sha256(viewerID || sort(memberSpaceIDs) || canSeePrivate)` folded into
  the `jc:rag:query:` key. Without this, scoped results leak across viewers
  through the 10-minute cache. Membership changes self-heal via the hash.
- `ReconcileSpaceScope(ctx, spaceID)` — refresh space entity row + bulk
  thread update (`UpdateThreadEmbeddingsBySpace`); used by the space
  update hook.

Caller updates (all existing `Retrieve` sites — signature change ripples):

| Site | viewerID source |
|---|---|
| `internal/ai/presearch.go:177` — `PreSearch` signature gains viewerID; callers `agent.go:294` + `agent_handlers.go:1090` pass session user | session user |
| `internal/ai/tools_deps.go:53` `RAGService` + `tools_knowledge.go:93` | tool-call `userID` |
| `internal/assessment/service.go:69,1329` + `internal/server/server.go:7979` adapter | `input.UserID` (anonymous → zero = public-only) |
| `internal/server/agent_adapters.go:86` `agentRAGAdapter` | tool-call userID |
| `internal/publicapi/phase2.go:63` iface + `:413` call | API-key owner's `UserID` — and `jc:api_semantic:` cache key gains the same scope hash |

`ai.RAGService` and `assessment` mirror interfaces gain the `viewerID`
param — adapters forward it.

**Adjacent fix (same exposure vector):** `presearch.go:201` `searchContent`
uses `visibility.InternalVisibilityFilter(zero)` — sees private content for
every user. Build the filter from the session viewer instead
(`VisibilityFilter{RequesterID: viewerID, CanSeeMembers: viewerID.Valid}`).

---

## Task 4 — Index-time metadata wiring

All nine `IndexEntity` call sites (verified by grep — the create paths are
easy to miss, they're not just reindex):

- `internal/spaces/handlers.go:157` `indexSpaceRAG` — `EntityRef{Type:"space",
  ID:space.ID, Visibility:space.Visibility, SpaceID:space.ID}`.
- `internal/forum/handlers.go:240` — thread **creation** indexing: space
  visibility via the thread's space (fetch or join), `space_id = ft.space_id`,
  `author_id = ft.author_user_id`.
- `internal/worker/handlers.go:2468` content embed task — load
  `GetContentEmbeddingMeta`; skip embedding entirely when
  `status != 'published'` (self-heal on stale tasks); build `EntityRef`
  from visibility/space/author.
- `internal/academy/handlers.go:145` — `academy_track`, `Visibility:"public"`.
- `internal/wiki/handlers.go:626` — `wiki_page`, `Visibility:"public"`.
- `rag.reindexForumThreads` (:395) — extend `ListForumThreadIDsForReindex`
  to `JOIN spaces` for visibility; threads in a null/deleted space →
  `visibility='private'` (fail closed).
- `rag.reindexSpaces` (:450) — extend `ListSpaceIDsForReindex` to return
  visibility.
- `rag` content reindex (:505) — same meta as worker path.

---

## Task 5 — Transition hooks

- **Space update** (`spaces` update handler): after successful update where
  `visibility` or `status` changed → `ReconcileSpaceScope(spaceID)`. Space
  delete → existing `PurgeEntity` stays; also purge thread embeddings? —
  check: thread rows' `space_id` FK is `ON DELETE SET NULL`, threads
  survive space deletion → reconcile them to `private` via
  `UpdateThreadEmbeddingsBySpace(...,'private')` before/with the space
  purge.
- **Content update**: on `status` transitioning away from `'published'` or
  `visibility` change → `UpdateEmbeddingScope` (or `PurgeEntity` when
  unpublished — unpublished entities must not be retrievable; the EXISTS
  guard already blocks, but purging keeps the table honest). Verify where
  the existing unpublish path lives (`content/handlers.go` update/publish
  toggles) and hook next to the existing delete/archive purge sites.
- **Forum thread delete/move**: delete purge already exists
  (`forum/handlers.go:254`) — verify it fires on all delete paths. Thread
  space reassignment (`forum/handlers.go:~2156` update path) →
  `UpdateEmbeddingScope` with the new space's visibility.

---

## Task 6 — `semanticHit` post-filter rework + tests

`internal/publicapi/phase2.go`:

- `space` → `GetSpaceByID` must succeed and `status='active'` — visibility
  check removed (retrieval already scoped).
- `content`/`forum_thread` → resolve via `GetContentByID`
  (`contents.sql.go:729` — verified: filters `deleted_at IS NULL` only;
  `status='published'` is already enforced by the retrieval EXISTS guard).
  Attach title/href; unresolved → drop.
- `wiki_page`/`academy_track` — unchanged.
- Tests: member-space hit for key owner resolves with title/href;
  deleted entity drops; unresolvable ID drops; keep the existing
  private-drop test only if its stub setup now maps to "unresolvable"
  (update the test to reflect the new contract — a member-visible space
  should now *resolve*, not drop).

---

## Task 7 — Verification

- `make sqlc-gen` output committed (gen files change but are generated —
  commit them, never hand-edit).
- `go build ./...`, `go vet ./internal/rag/ ./internal/publicapi/
  ./internal/ai/ ./internal/assessment/ ./internal/mcp/`, `gofmt -l`.
- `go test ./internal/rag/ ./internal/publicapi/ ./internal/ai/
  ./internal/mcp/ ./internal/assessment/`.
- Apply `000728` against the dev DB (`make migrate` may still be blocked by
  the known 000721 drift — apply via psql if so) and verify backfill counts.
- Live check on `:8081` if still running: `search.semantic` as key owner
  returns only what the owner can see; MCP `rag.search` (JWT path can't be
  smoke-tested without a session — unit-level OK).
- JEOS §69/§119 diff reviewed; settings catalog entry present.

## Watch-outs

- **Clause ordering** — the `author_id` OR must come before (logically
  independent of) the member-space clause; author-private content in a
  member space stays author-only. The `NOT (content AND private)` guard in
  the member clause is what enforces this.
- **`Retrieve` signature change ripples** through four packages — do them
  in one commit so the tree never sits broken.
- **Backfill correctness** — run the UPDATEs against the dev DB before
  shipping the migration; `forum_thread` rows with null `space_id` must
  land on `private` (fail closed).
- **Unrelated worktree changes** — literal-resolver wiring in
  `server.go`, `.jeos` section edits, `migrations/000722` — do not stage.
  When `server.go` must be edited (adapter signatures), stage hunks only.
- **Migration number** — `000728` is free as of plan time; verify before
  writing.
