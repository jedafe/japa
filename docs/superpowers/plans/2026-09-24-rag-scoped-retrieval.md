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
  user_id = $1` (pattern exists in `trending.sql.go:141`).
- `UpdateEmbeddingScope(entityType, entityID, visibility, spaceID,
  authorID)` — single-entity scope refresh.
- `UpdateThreadEmbeddingsBySpace(spaceID, visibility)` — bulk update for
  space visibility flips.
- `GetContentEmbeddingMeta(contentID)` — title, body, status, visibility,
  space_id, author_id (used by the worker embed path).
- Extend `SearchContentEmbeddings` — add the scope clause:

```sql
AND (
    ce.visibility = 'public'
    OR ce.author_id = $N_viewer
    OR (ce.space_id = ANY($N_member_ids)
        AND NOT (ce.entity_type = 'content' AND ce.visibility = 'private'))
)
```

Keep the existing per-type EXISTS guards (published/non-deleted/active) —
they catch deletion races between index and query.

---

## Task 3 — `rag.Service` refactor + caller updates

`internal/rag/service.go`:

- `type EntityRef struct { Type string; ID, SpaceID, AuthorID pgtype.UUID; Visibility string }`
- `IndexEntity(ctx, ref EntityRef, body, language string)` — replaces the
  `(entityType, entityID, body, language)` signature; writes scope columns.
- `Retrieve(ctx, query, language string, geoSpaceID *pgtype.UUID,
  viewerID pgtype.UUID)` — builds `RetrievalScope` internally:
  - `viewerID` invalid → empty member list (public-only).
  - `viewerID` valid → `ListMemberSpaceIDs`.
  - `rag.scoped_retrieval.enabled == false` → force empty scope.
  - Existing `rag.enabled` / embedder / empty-query early-outs unchanged.
- `ReconcileSpaceScope(ctx, spaceID)` — refresh space entity row + bulk
  thread update (`UpdateThreadEmbeddingsBySpace`); used by the space
  update hook.

Caller updates (all existing `Retrieve` sites — signature change ripples):

| Site | viewerID source |
|---|---|
| `internal/ai/handlers.go:95` (iface) + `presearch.go:177` | caller userID passed down — check where the chat path knows the user |
| `internal/ai/tools_deps.go:53` `RAGService` + `tools_knowledge.go:93` | tool-call `userID` (already in args/context) |
| `internal/assessment/service.go:69,1329` + `internal/server/server.go:7979` adapter | assessed user ID |
| `internal/server/agent_adapters.go:86` `agentRAGAdapter` | tool-call userID |
| `internal/publicapi/phase2.go:63` iface + `:413` call | API-key owner's `UserID` |

`ai.RAGService` and `assessment` mirror interfaces gain the `viewerID`
param — adapters forward it.

---

## Task 4 — Index-time metadata wiring

- `internal/spaces` `indexSpaceRAG` — `EntityRef{Type:"space", ID:space.ID,
  Visibility:space.Visibility, SpaceID:space.ID}`.
- `internal/worker/embedder.go` `ragIndexAfterEmbed` — load
  `GetContentEmbeddingMeta`; skip embedding entirely when
  `status != 'published'` (self-heal on stale tasks); build `EntityRef`
  from visibility/space/author.
- `rag.reindexForumThreads` — query must return visibility via
  `JOIN spaces` (check existing `ListForumThreadIDsForReindex` — extend or
  new query); threads in a null/deleted space → `visibility='private'`.
- `reindexSpaces` — read each space's visibility (extend
  `ListSpaceIDsForReindex` to return visibility, or per-space fetch).
- Content reindex path — same meta as worker path.
- `learning` / wiki indexers — `Visibility:"public"`.

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
- **Forum thread delete/move**: `PurgeEntity("forum_thread", id)` — check
  `internal/forum` (or wherever thread delete lives); add if absent.

---

## Task 6 — `semanticHit` post-filter rework + tests

`internal/publicapi/phase2.go`:

- `space` → `GetSpaceByID` must succeed and `status='active'` — visibility
  check removed (retrieval already scoped).
- `content`/`forum_thread` → resolve via `GetContentByID` (exists,
  `contents.sql.go:729`) — check its filters at implementation time; if it
  returns unpublished/deleted rows, add a `deleted_at IS NULL` guard via a
  new query or reuse the EXISTS clause result implicitly (retrieval already
  filtered). Attach title/href; unresolved → drop.
- `wiki_page`/`learning_track` — unchanged.
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
