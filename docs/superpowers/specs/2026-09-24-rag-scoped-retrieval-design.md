# RAG Scoped Retrieval — Design

**Date:** 2026-09-24
**Status:** Draft (awaiting review)
**Refs:** japaconnect-engineering/sections/69-rag-knowledge-layer.md, ADR-069,
docs/superpowers/specs/2026-09-24-agent-data-api-phase2-design.md

## Context & Motivation

The `content_embeddings` index is **not visibility-scoped** and never has
been:

- `ListSpaceIDsForReindex` embeds every `status='active'` space — private and
  members-only included.
- `ListPublishedContentIDsForReindex` embeds every `status='published'`
  content row — `members` and `private` visibility included.
- `ListForumThreadIDsForReindex` embeds every non-deleted thread, including
  threads inside private spaces.
- `indexSpaceRAG` fires on every space create/update with no visibility
  check; `ragIndexAfterEmbed` embeds whatever content the task names.
- Purges exist for entity *deletion* (space, content, learning track) but not
  for visibility/status transitions (unpublish, public→members/private).

`SearchContentEmbeddings` guards entity *existence* (published/non-deleted/
active EXISTS clauses) but never *who may see it*. Until the Phase 2 review,
every `Retrieve` caller — the in-app AI assistant, the assessment path, and
the MCP `rag.search` tool — could surface non-public text to any caller. The
public `/public/search/semantic` endpoint shipped with a verified-public
post-filter (commit `d6448fe8`) and `rag.search` was removed from the
API-key allowlist (`c454eb42`) as interim mitigations.

**Chosen direction (user decision):** full scoped retrieval — store
visibility metadata on each embedding row and filter at retrieval time by
the caller's view, rather than collapsing the corpus to public-only.

## Goals

- `Retrieve` answers "what can THIS caller see" — anonymous/key callers get
  public only; authenticated callers get public + member-space content +
  their own private content.
- Index-time writes record `visibility`, `space_id`, `author_id` per
  embedding row (denormalized — no joins on the hot vector path).
- Visibility/status transitions update or purge embedding rows so the index
  never serves stale-visibility chunks.
- Backfill migration classifies all existing rows correctly.
- The `/public/search/semantic` post-filter is reworked to match: hits are
  dropped only when the entity is unresolvable (deleted), since retrieval
  itself is now scope-correct.

## Non-Goals

- Re-ranking, hybrid search, or retrieval-quality changes.
- New entity types in the corpus.
- ~~`rag.search` re-admission to the API-key allowlist (permanent removal —
  `search.semantic` is the key-facing semantic surface).~~ **Revisited
  2026-09-24:** restored for API keys at `api.read.search` + pro tier — the
  same gate as `search.semantic` — now that retrieval is viewer-scoped to
  the key owner. See §119.
- Embedding model changes.

## Schema (migration 000728 — verify next free at plan time)

```sql
ALTER TABLE content_embeddings
    ADD COLUMN visibility text NOT NULL DEFAULT 'public',
    ADD COLUMN space_id   uuid NULL,
    ADD COLUMN author_id  uuid NULL;

-- Backfill from source entities.
UPDATE content_embeddings ce
SET    visibility = s.visibility, space_id = s.id
FROM   spaces s
WHERE  ce.entity_type = 'space' AND ce.entity_id = s.id;

UPDATE content_embeddings ce
SET    visibility = c.visibility, space_id = c.space_id, author_id = c.author_id
FROM   contents c
WHERE  ce.entity_type = 'content' AND ce.entity_id = c.id;

UPDATE content_embeddings ce
SET    visibility = sp.visibility, space_id = ft.space_id, author_id = ft.author_user_id
FROM   forum_threads ft JOIN spaces sp ON sp.id = ft.space_id
WHERE  ce.entity_type = 'forum_thread' AND ce.entity_id = ft.id;

-- Threads whose parent space is gone: private by default (fail closed).
UPDATE content_embeddings SET visibility = 'private'
WHERE  entity_type = 'forum_thread' AND space_id IS NULL;

-- wiki_page / learning_track remain 'public' (public corpora).

ALTER TABLE content_embeddings ALTER COLUMN visibility DROP DEFAULT;
-- CHECK: visibility IN ('public','members','private')
```

## Retrieval Semantics

Caller scope = "what the viewer can see":

```go
// rag.RetrievalScope — assembled by rag.Service from the viewer ID.
// Zero viewer → public-only.
type RetrievalScope struct {
    ViewerID       pgtype.UUID
    MemberSpaceIDs []pgtype.UUID // space_members rows for the viewer
}
```

**Visibility semantics follow the canonical contract** in
`internal/content/visibility.VisibilityFilter` — verified against
`ListContents`/`ListContentByTag` and `spaces.go` constants:

- `public` → everyone.
- `members` → any authenticated viewer (NOT space members — the platform
  never joins `space_members` for content visibility).
- `private` → author only, or privileged viewers
  (`CanSeePrivate`: superadmin / `content.edit_any` / `admin.moderation`).
- `space`/`forum_thread` entity types additionally follow *space* semantics:
  `members` space = listed to authenticated users; `private` space = approved
  `space_members` only (`status='approved'` — `CheckSpaceMembership`
  requires it).

`RetrievalScope` mirrors `VisibilityFilter` plus membership:

```go
type RetrievalScope struct {
    ViewerID       pgtype.UUID   // zero = anonymous
    CanSeePrivate  bool          // privileged caller (RBAC-resolved)
    MemberSpaceIDs []pgtype.UUID // approved memberships only
}
```

Both retrieval queries gain the scope clause —
`SearchContentEmbeddings` (all entity types):

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

`SearchContentEmbeddingsGeoFiltered` (content-only — simpler):

```sql
AND (
    ce.visibility = 'public'
    OR ce.author_id = $viewer
    OR (ce.visibility = 'members' AND $viewer IS NOT NULL)
    OR (ce.visibility = 'private' AND $can_see_private)
)
```

Note: `content` rows never consult `space_id` for visibility (the platform
contract treats content visibility as self-contained) — the column is still
stored for future use and consistency.

`Retrieve` signature becomes:

```go
Retrieve(ctx context.Context, query, language string, geoSpaceID *pgtype.UUID, viewerID pgtype.UUID) ([]RetrievedChunk, error)
```

- `viewerID` invalid/zero → `MemberSpaceIDs` empty, `members` clause fails
  (`$viewer IS NOT NULL` false), author clause never matches → public-only.
- Membership loaded inside `rag.Service` via `ListMemberSpaceIDs` — must
  filter `status='approved'` (matching `CheckSpaceMembership`).
- `CanSeePrivate` defaults false for all user-facing paths; reserved for
  privileged/internal callers.
- Kill-switch setting `rag.scoped_retrieval.enabled` (default true): when
  false, retrieval falls back to public-only — lets ops degrade to the safe
  subset without a deploy if the scoping misbehaves.

### Cache keys must be viewer-scoped (leak vector)

`Retrieve` caches under `jc:rag:query:{hash}` keyed on
(query_embedding, language, top_k, geo_space_id). Once results are
viewer-scoped, that key would serve a member's private-corpus results to
the next viewer with the same query. The hash gains a scope component:

```go
scopeHash = sha256(viewerID || sort(memberSpaceIDs) || canSeePrivate)
cacheKey = jc:rag:query:{hash(query_embedding, lang, topK, geo, scopeHash)}
```

Folding membership into the hash also self-heals on membership changes —
a new approved space produces a different hash, no invalidation needed.
Same treatment for `jc:api_semantic:{q}:{lang}:{space}` in
`publicapi.SemanticSearch` — add the key-owner scope hash.

### Caller updates (all existing `Retrieve` call sites — verified)

| Caller | viewerID |
|---|---|
| `internal/ai/presearch.go:177` (`PreSearcher.PreSearch` — signature gains viewerID; callers `agent.go:294`, `agent_handlers.go:1090` pass session user) | session user |
| `internal/ai/tools_knowledge.go:93` (`rag.search` tool) via `RAGService` iface `tools_deps.go:53` | tool-call userID |
| `internal/assessment/service.go:1329` via adapter `server.go:7979` | `input.UserID` (anonymous submissions → zero = public-only) |
| `internal/server/agent_adapters.go:86` (`agentRAGAdapter`) | tool-call userID |
| `internal/publicapi/phase2.go:413` (`SemanticSearch`) | **API-key owner** — the key IS the caller's identity |

**Adjacent pre-existing exposure (in scope for this change):**
`PreSearcher.searchContent` uses `visibility.InternalVisibilityFilter(zero)`
— the content-search side of presearch sees private content for *every*
user. Tighten to the viewer's filter (`VisibilityFilter` built from the
session user) in the same change — same leak vector, same fix shape.

### Consequence for the Phase 2 post-filter

`semanticHit` currently drops any hit that isn't provably *public*. With
scoped retrieval, member-visible hits are legitimate — the filter becomes
"entity resolves and is not deleted":

- `space` → `GetSpaceByID` must succeed and `status='active'` (visibility no
  longer checked — retrieval already enforced it).
- `content`/`forum_thread` → resolve by ID without the public-only guard
  (check for a suitable gen query — `GetContentByID`-style, not
  `GetPublishedContentByID`) — attach title/href; unresolved → drop.
- `wiki_page`/`learning_track` → unchanged (public corpora, no resolver).

## Index-Time Changes

The corpus stays **complete** — indexers continue embedding every entity
(private spaces, members-only content, everything); scoped retrieval is what
enforces visibility. What changes is that each embedding row must carry
correct scope metadata, so `IndexEntity` gains it via a struct parameter:

```go
type EntityRef struct {
    Type       string
    ID         pgtype.UUID
    Visibility string        // 'public' | 'members' | 'private'
    SpaceID    pgtype.UUID   // zero for unscoped entities
    AuthorID   pgtype.UUID   // zero when not applicable
}
IndexEntity(ctx context.Context, ref EntityRef, body, language string) error
```

Call sites (all nine, verified by grep):

- `spaces/handlers.go:157` `indexSpaceRAG` — space visibility; `space_id = space.ID`.
- `forum/handlers.go:240` — thread **creation** indexing: space visibility
  via join, `space_id = ft.space_id`, `author_id = ft.author_user_id`.
- `worker/handlers.go:2468` content embed task — needs
  `GetContentEmbeddingMeta` (title/body/status/visibility/space_id/author_id).
- `learning/handlers.go:145` — `learning_track`, `visibility='public'`.
- `wiki/handlers.go:626` — `wiki_page`, `visibility='public'`.
- `rag.reindexForumThreads` (:395) — join `spaces` for visibility.
- `rag.reindexSpaces` (:450) — space visibility.
- `rag` content reindex (:505 area) — content meta as above.
- All `IndexEntity` interface declarations in caller packages
  (`spaces/handlers.go:62`, `forum/handlers.go:64`, `learning/handlers.go:43`,
  `wiki/handlers.go:79`, `worker/rag_indexer.go:28`) — signature ripples.

## Transition Hooks (visibility/status changes update, not just purge)

- Space update: after `UpdateSpace`, call `ReconcileSpaceScope(spaceID)` —
  sets `content_embeddings.visibility/space_id` for the space entity AND all
  its forum_thread embeddings (threads inherit space visibility). Space
  delete → existing `PurgeEntity` stays.
- Content update: on `status` leaving `'published'` or `visibility` change →
  `UpdateEntityScope`/`PurgeEntity` as appropriate (delete/archive already
  purge — keep). Published→draft/archived → purge (unpublished entities must
  not be retrievable).
- Forum thread delete/move → `PurgeEntity` (check existing delete handler —
  add if absent).

New sqlc queries (edit `db/queries/`, run `make sqlc-gen` — never hand-edit
gen files):

- `ListMemberSpaceIDs(userID)` — `SELECT space_id FROM space_members WHERE user_id=$1` (pattern exists in trending.sql.go).
- `UpdateEmbeddingScope(entityType, entityID, visibility, spaceID, authorID)`.
- `UpdateThreadEmbeddingsBySpace(spaceID, visibility)` — bulk update for space flips.
- `GetContentEmbeddingMeta(contentID)` — title, body, status, visibility, space_id, author_id.
- Extend `SearchContentEmbeddings` — scope clause above.

## MCP / API-Key Surface

- ~~`rag.search` stays out of `apiKeyToolGates` permanently — documented in
  §119 (done in `c454eb42` + jeos `337efec`).~~ **Revisited 2026-09-24:**
  re-admitted at `{scope: "api.read.search", minTier: "pro"}` once scoped
  retrieval proved viewer-scoped for key owners (§119 updated).
- `search.semantic` (REST + MCP) returns the caller-scoped corpus — premium
  unchanged.
- JEOS §69 (RAG) gets a "scoped retrieval" amendment documenting the corpus
  rules; §119 already notes the rag.search removal.

## Consistency Window

Denormalized `visibility`/`space_id`/`author_id` can drift if a write path
misses its hook. Mitigations:

- `QueueReindex` re-embeds the whole corpus with fresh scope metadata — the
  periodic reindex is the sweeper.
- Each transition hook lives next to the existing purge call sites so future
  edits see both.

## Testing

- `rag` service tests: scope assembly (anonymous → no memberships), clause
  mapping, `rag.scoped_retrieval.enabled` fallback.
- `publicapi` semantic tests: member-space hit for key owner resolves with
  title/href; deleted-entity hit drops; unresolvable ID drops.
- Migration: SQL review + apply against the dev DB; verify backfill counts.
- `ai` tool tests: `search.semantic`/`rag.search` pass caller `userID`.

## Success Criteria

- No `Retrieve` path can return chunks for entities the viewer cannot see.
- Public API key → public-only results; session/member → member-visible too.
- `go build ./...`, `go vet`, and all touched package tests green.
- JEOS §69/§119 synchronized.
