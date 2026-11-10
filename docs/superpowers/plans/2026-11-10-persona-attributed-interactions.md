# Persona-Attributed Interactions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let operators holding `content.assign_author` comment, reply in threads, vote, react, and follow as a bot persona — with the persona's geo flowing into authored content.

**Architecture:** A shared delegated-actor resolver in `internal/personas` validates `author_id`/`actor_id` overrides (bot-only, permission-gated). Comments and forum replies persist `created_by_user_id` so the operator keeps edit/delete rights without leaking their identity. Persona economy fields arrive via the profile-editing plan (sibling doc).

**Spec:** `docs/superpowers/specs/2026-11-10-persona-attributed-interactions-design.md`

**Tech Stack:** Go 1.x + chi + pgx + sqlc (`internal/db/queries/*.sql` → `make sqlc-gen`), postgres migrations (`migrations/NNNNNN_*.up/down.sql`), SvelteKit 2/Svelte 5 runes (apps/main), Bun.

## Global Constraints

- Never hand-edit `internal/db/gen/**` — edit `db/queries/*.sql`, run `make sqlc-gen`.
- Errors use `errpkg.WriteError(w, status, snake_code, msg)` — no sentence codes.
- `created_by_user_id` is NEVER serialized to clients — only the derived `viewer_can_manage` flag.
- Migration files are numbered `001112+` (next free at time of writing; take the real next number).
- Commit on `main` after every task. Commit style: `feat(scope):`, `fix(scope):` conventional messages, no attribution trailers.
- Run `go build ./...` and `go test ./internal/<pkg>/` per task.
- Bun only for frontend tooling (`bunx vite`, `bun run check`).

---

### Task 1: Migration + sqlc — `created_by_user_id` on comments & forum_replies

**Files:**
- Create: `migrations/001112_persona_created_by.up.sql`, `migrations/001112_persona_created_by.down.sql` (take the actual next number — `ls migrations | grep -v archive | tail -1`)
- Modify: `internal/db/queries/comments.sql`, `internal/db/queries/forum.sql`, `internal/db/queries/gdpr.sql`

**Interfaces:**
- Produces: `gen.CreateCommentParams.CreatedByUserID`, `gen.CreateForumReplyParams.CreatedByUserID`, `viewer_can_manage` field on comment/reply read rows, `gen.ClearCommentsCreatedBy`/`gen.ClearForumRepliesCreatedBy`, `gen.GetUserCurrentGeoSpace`.

- [ ] **Step 1: Write the migration**

`001112_persona_created_by.up.sql`:

```sql
ALTER TABLE comments
  ADD COLUMN created_by_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_comments_created_by ON comments(created_by_user_id)
  WHERE deleted_at IS NULL;

ALTER TABLE forum_replies
  ADD COLUMN created_by_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_forum_replies_created_by ON forum_replies(created_by_user_id)
  WHERE deleted_at IS NULL;
```

`001112_persona_created_by.down.sql`:

```sql
DROP INDEX IF EXISTS idx_forum_replies_created_by;
ALTER TABLE forum_replies DROP COLUMN IF EXISTS created_by_user_id;
DROP INDEX IF EXISTS idx_comments_created_by;
ALTER TABLE comments DROP COLUMN IF EXISTS created_by_user_id;
```

- [ ] **Step 2: Update `internal/db/queries/comments.sql`**

`CreateComment` — add `sqlc.narg(created_by_user_id)`:

```sql
-- name: CreateComment :one
INSERT INTO comments (content_id, author_user_id, parent_id, body, depth, is_anonymous, created_by_user_id) VALUES (@content_id, @author_user_id, sqlc.narg(parent_id), @body, @depth, @is_anonymous, sqlc.narg(created_by_user_id))
RETURNING *;
```

`GetComment` — expose `created_by_user_id` for the internal ownership check
(it is a column on the joined row; handlers must never serialize it):

```sql
SELECT c.id, c.content_id, c.author_user_id, c.created_by_user_id, c.parent_id, c.body, c.depth,
```

In `ListCommentsByContent`, `ListCommentsByContentNew`,
`ListCommentsByContentOldest`, `ListRepliesByParent` — add a nullable
actor override to the `user_vote` subquery and the manage flag (these
queries already take `@viewer_id`):

```sql
 COALESCE((
 SELECT cv.value
 FROM comment_votes cv
 WHERE cv.comment_id = c.id
 AND cv.user_id = COALESCE(sqlc.narg(vote_actor_id)::uuid, @viewer_id)
 LIMIT 1
 ), 0)::smallint AS user_vote,
 (c.author_user_id = @viewer_id OR c.created_by_user_id = @viewer_id) AS viewer_can_manage,
```

(PublicAPI variants `ListPublicCommentsByContent`/`ListPublicRepliesByParent`
get `viewer_can_manage` too — always `false` for guests — by adding the same
computed column with `@viewer_id` bound to the caller.)

`UpdateComment`/`UpdateCommentUnmarked` — extend the author predicate:

```sql
WHERE id = @id AND (author_user_id = @author_user_id OR created_by_user_id = @author_user_id) AND deleted_at IS NULL
```

`SoftDeleteCommentByAuthor`:

```sql
WHERE id = @id AND (author_user_id = @author_user_id OR created_by_user_id = @author_user_id)
 AND deleted_at IS NULL;
```

- [ ] **Step 3: Update `internal/db/queries/forum.sql`**

`CreateForumReply` — add `created_by_user_id` to columns + RETURNING list:

```sql
INSERT INTO forum_replies (thread_id, parent_reply_id, author_user_id, body, is_anonymous, created_by_user_id) VALUES (@thread_id, sqlc.narg(parent_reply_id), @author_user_id, @body, @is_anonymous, sqlc.narg(created_by_user_id))
RETURNING id, thread_id, parent_reply_id, author_user_id, created_by_user_id,
 body, vote_score, view_count, is_anonymous, is_edited,
 created_at, updated_at, deleted_at, ama_pending, ama_approved_at;
```

In `GetForumReplyByID`, `GetForumReplyByIDWithAuthor`,
`ListForumRepliesByThread`, `ListForumRepliesByThreadWithAuthor` —
add `fr.created_by_user_id` to the SELECT list. In the `WithAuthor`/list
variants add the computed flag (list queries gain a `@viewer_id` param):

```sql
 (fr.author_user_id = @viewer_id OR fr.created_by_user_id = @viewer_id) AS viewer_can_manage
```

`ListUserVotesForThread` — no change (callers pass the actor's user_id).

- [ ] **Step 4: Update `internal/db/queries/gdpr.sql`**

Append near `AnonymizeCommentsByAuthor`:

```sql
-- name: ClearCommentsCreatedBy :exec
-- GDPR: the operator behind persona-authored comments is an identity link —
-- cleared on erasure (author_user_id already has its own path).
UPDATE comments SET created_by_user_id = NULL WHERE created_by_user_id = @user_id;

-- name: ClearForumRepliesCreatedBy :exec
UPDATE forum_replies SET created_by_user_id = NULL WHERE created_by_user_id = @user_id;
```

- [ ] **Step 5: Regenerate sqlc + migrate**

```bash
make sqlc-gen
migrate -path migrations -database "$DATABASE_URL" up   # or repo's migrate target
go build ./internal/db/... ./internal/comments/... ./internal/forum/...
```

Expected: build clean; `gen.CreateCommentParams.CreatedByUserID` exists.

- [ ] **Step 6: Commit**

```bash
git add migrations internal/db
git commit -m "feat(personas): created_by_user_id on comments + forum_replies"
```

---

### Task 2: Shared delegated-actor resolver + audit helper (`internal/personas`)

**Files:**
- Create: `internal/personas/delegated.go`, `internal/personas/delegated_test.go`
- Modify: `internal/content/handlers.go:354-390` (resolveBotAuthor → thin wrapper)

**Interfaces:**
- Produces:
  - `personas.ErrDelegatedInvalid`, `personas.ErrDelegatedForbidden`, `personas.ErrDelegatedNotBot` (sentinels)
  - `personas.ResolveDelegatedActor(ctx, bots BotChecker, resolver *rbac.RBAC, operatorID pgtype.UUID, raw string) (pgtype.UUID, error)`
  - `personas.WriteDelegatedError(w http.ResponseWriter, err error, param string)` — `param` is `"author_id"` or `"actor_id"` (drives `invalid_author_id` vs `invalid_actor_id`)
  - `personas.AuditAttribution(ctx, pool *pgxpool.Pool, operatorID, actorID pgtype.UUID, actionType, entityType string, entityID pgtype.UUID)`
- Consumes: `gen.Queries.GetUserIsBot` (exists — `db/queries/gdpr.sql:623`), `rbac.FromContext` (`*rbac.RBAC` has `IsSuperAdmin`/`HasPermission`).

- [ ] **Step 1: Write the failing test**

`internal/personas/delegated_test.go`:

```go
package personas

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

type fakeBots struct{ bot bool; err error }
func (f fakeBots) IsBotUser(context.Context, pgtype.UUID) (bool, error) { return f.bot, f.err }

func TestResolveDelegatedActor_InvalidUUID(t *testing.T) {
	_, err := ResolveDelegatedActor(context.Background(), fakeBots{bot: true}, nil, pgtype.UUID{}, "not-a-uuid")
	if !errors.Is(err, ErrDelegatedInvalid) {
		t.Fatalf("err = %v, want ErrDelegatedInvalid", err)
	}
}

func TestResolveDelegatedActor_NotBot(t *testing.T) {
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	_, err := ResolveDelegatedActor(context.Background(), fakeBots{bot: false}, nil, pgtype.UUID{}, uid.String())
	if !errors.Is(err, ErrDelegatedNotBot) {
		t.Fatalf("err = %v, want ErrDelegatedNotBot", err)
	}
}

func TestWriteDelegatedError_Codes(t *testing.T) {
	for param, want := range map[string]string{"author_id": "invalid_author_id", "actor_id": "invalid_actor_id"} {
		rr := httptest.NewRecorder()
		WriteDelegatedError(rr, ErrDelegatedInvalid, param)
		if rr.Code != http.StatusBadRequest || !bytes.Contains(rr.Body.Bytes(), []byte(want)) {
			t.Fatalf("param %s: got %d %s", param, rr.Code, rr.Body.String())
		}
	}
}
```

- [ ] **Step 2: Run test — verify it fails**

Run: `go test ./internal/personas/ -run TestResolveDelegatedActor -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement `internal/personas/delegated.go`**

```go
// Package personas — delegated-actor resolution for author_id/actor_id
// overrides (design: docs/superpowers/specs/2026-11-10-persona-attributed-
// interactions-design.md §1.1). Callers keep their own identity; the delegate
// field picks the bot persona that records the action.
package personas

var (
	ErrDelegatedInvalid   = errors.New("delegate id is not a valid user UUID")
	ErrDelegatedForbidden = errors.New("delegate requires content.assign_author")
	ErrDelegatedNotBot    = errors.New("delegate must identify a bot persona account")
)

// BotChecker reports whether a user id is a bot backing account
// (users.is_bot). content.Service already satisfies this; other services
// gain a thin IsBotUser wrapper over gen.GetUserIsBot.
type BotChecker interface {
	IsBotUser(ctx context.Context, userID pgtype.UUID) (bool, error)
}

// ResolveDelegatedActor validates a delegate override. nil resolver means
// "no RBAC in context" → forbidden (mirrors canAssignAuthor).
func ResolveDelegatedActor(ctx context.Context, bots BotChecker, resolver *rbac.RBAC, operatorID pgtype.UUID, raw string) (pgtype.UUID, error) {
	var actorID pgtype.UUID
	if err := actorID.Scan(raw); err != nil || !actorID.Valid {
		return pgtype.UUID{}, ErrDelegatedInvalid
	}
	if resolver == nil || !(resolver.IsSuperAdmin(ctx, operatorID) ||
		resolver.HasPermission(ctx, operatorID, "content.assign_author")) {
		return pgtype.UUID{}, ErrDelegatedForbidden
	}
	isBot, err := bots.IsBotUser(ctx, actorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, ErrDelegatedInvalid
		}
		return pgtype.UUID{}, fmt.Errorf("personas: delegate bot check: %w", err)
	}
	if !isBot {
		return pgtype.UUID{}, ErrDelegatedNotBot
	}
	return actorID, nil
}

// WriteDelegatedError maps a ResolveDelegatedActor error to the standard
// envelope. param selects the field name in codes ("author_id"/"actor_id").
// Non-sentinel errors are internal failures (e.g. the is_bot DB lookup),
// never a client error.
func WriteDelegatedError(w http.ResponseWriter, err error, param string) {
	switch {
	case errors.Is(err, ErrDelegatedInvalid):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_"+param,
			param+" does not identify a user")
	case errors.Is(err, ErrDelegatedForbidden):
		errpkg.WriteError(w, http.StatusForbidden, "author_assign_forbidden",
			"Setting "+param+" to another user requires the content.assign_author permission")
	case errors.Is(err, ErrDelegatedNotBot):
		errpkg.WriteError(w, http.StatusBadRequest, "author_not_bot",
			param+" must identify a bot persona account — real-user attribution requires impersonation")
	default:
		slog.Error("delegated actor resolution failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error",
			"Failed to validate "+param)
	}
}

// AuditAttribution writes a persona_attrib.* audit_logs row — fire-and-forget
// callers; a failure is logged by the caller, never fails the request
// (same posture as impersonation.auditLog, Rule 10). Callers reach the
// writer pool via their service's DB() accessor (see Task 3).
func AuditAttribution(ctx context.Context, pool *pgxpool.Pool, operatorID, actorID pgtype.UUID, actionType, entityType string, entityID pgtype.UUID) error {
	meta, _ := json.Marshal(map[string]any{"entity_type": entityType, "entity_id": entityID.String()})
	_, err := pool.Exec(ctx,
		`INSERT INTO audit_logs (actor_user_id, impersonated_user_id, action_type, target_type, target_id, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb)`,
		operatorID, actorID.String(), "persona_attrib."+actionType, entityType, entityID, meta)
	return err
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/personas/ -v`
Expected: PASS.

- [ ] **Step 5: Rewire `content.resolveBotAuthor` to the shared resolver**

In `internal/content/handlers.go`, replace the body of `resolveBotAuthor`
(lines 360-389) with a delegation so behavior is identical:

```go
func (h *Handlers) resolveBotAuthor(w http.ResponseWriter, r *http.Request, actorID pgtype.UUID, raw string) (pgtype.UUID, bool) {
	id, err := personas.ResolveDelegatedActor(r.Context(), h.Service, rbac.FromContext(r.Context()), actorID, raw)
	if err != nil {
		personas.WriteDelegatedError(w, err, "author_id")
		return pgtype.UUID{}, false
	}
	return id, true
}
```

(content.Service already has `IsBotUser` — `store.go:829`. `canAssignAuthor`
may remain for other callers or be removed if unused.)

- [ ] **Step 6: Run tests + commit**

Run: `go build ./... && go test ./internal/personas/ ./internal/content/`
Expected: build + PASS.
`git commit -m "feat(personas): shared delegated-actor resolver + audit helper"`

---

### Task 3: Comments — `author_id` on create + permission split

**Files:**
- Modify: `internal/comments/handlers.go:583-732` (CreateComment), `internal/comments/store.go:28-33,69-160` (CreateCommentInput + CreateComment), `internal/comments/handlers_test.go`

**Interfaces:**
- Consumes: `personas.ResolveDelegatedActor`, `personas.WriteDelegatedError`, `personas.AuditAttribution`.
- Produces: `comments.Service.IsBotUser(ctx, id)`, `CreateCommentInput{ContentID, ParentID, Body, IsAnonymous, CreatedByUserID pgtype.UUID}` — `CreateComment(ctx, authorID, input)` keeps signature; `authorID` is the *effective* author (persona or self).

- [ ] **Step 1: Failing tests**

Add to `comments_test.go` — handler-level: `author_id`+`is_anonymous` →
400 `mutually_exclusive`; missing perm → 403 `author_assign_forbidden`.
(Use the existing httptest harness in `handlers_test.go`; wire a stub
`rbac.RBAC` the same way existing perm tests do, or assert the input-shape
validation only where RBAC can't be stubbed.)

```go
func TestCreateComment_AuthorIDAndAnonymous_MutuallyExclusive(t *testing.T) {
	h := &Handlers{}
	req := authedRequest(t, "POST", "/api/v1/content/"+uuid+"//comments",
		`{"author_id":"`+botUUID+`","is_anonymous":true,"body":[1]}`)
	rr := httptest.NewRecorder()
	h.CreateComment(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "mutually_exclusive") {
		t.Fatalf("got %d %s", rr.Code, rr.Body.String())
	}
}
```

- [ ] **Step 2: Run — FAIL** (`go test ./internal/comments/ -run AuthorID -v`)

- [ ] **Step 3: Implement**

`internal/comments/store.go`:

```go
type CreateCommentInput struct {
	ContentID       pgtype.UUID
	ParentID        pgtype.UUID
	Body            []byte
	IsAnonymous     bool
	// CreatedByUserID is the operator when the comment is attributed to a
	// persona (author_id override). Zero for normal comments.
	CreatedByUserID pgtype.UUID
}
```

In `CreateComment`'s `CreateCommentParams` call add `CreatedByUserID: input.CreatedByUserID`. Add:

```go
// IsBotUser reports whether the user is a bot backing account — feeds the
// delegated-actor resolver (persona author_id overrides).
func (s *Service) IsBotUser(ctx context.Context, userID pgtype.UUID) (bool, error) {
	return s.read.GetUserIsBot(ctx, userID)
}
```

`internal/comments/handlers.go` `CreateComment` — input gains `AuthorID *string`; resolve before spam check:

```go
	effectiveID := user.UserID
	var createdBy pgtype.UUID
	if input.AuthorID != nil && *input.AuthorID != "" {
		if input.IsAnonymous {
			errpkg.WriteError(w, http.StatusBadRequest, "mutually_exclusive",
				"author_id and is_anonymous cannot be combined")
			return
		}
		aid, err := personas.ResolveDelegatedActor(r.Context(), h.Service, rbac.FromContext(r.Context()), user.UserID, *input.AuthorID)
		if err != nil {
			personas.WriteDelegatedError(w, err, "author_id")
			return
		}
		effectiveID, createdBy = aid, user.UserID
	}
```

- Spam pipeline + `RecordContentCreation` keep `user.UserID` (operator).
- Space `CanCommentInSpace`/`CanReplyInSpace` calls pass `effectiveID`.
- `createInput.CreatedByUserID = createdBy`; service call passes `effectiveID`.
- Audit: add an accessor on `comments.Service` (mirrors
  `personas.Store.DB()` precedent — no new wiring on Handlers):

```go
// DB exposes the writer pool for fire-and-forget audit writes.
func (s *Service) DB() *db.DB { return s.db }
```

  then after `CreateComment` succeeds, when `createdBy.Valid`:
  `go personas.AuditAttribution(context.Background(), h.Service.DB().Writer, user.UserID, effectiveID, "comment", "comment", comment.ID)`
  (same `DB()` accessor pattern is used by every service that audits —
  forum/follows/content add it the same way when their tasks need it).

- [ ] **Step 4: Run — PASS** (`go test ./internal/comments/ -v`)

- [ ] **Step 5: Commit** — `feat(comments): author_id override for persona-attributed comments`

---

### Task 4: Comments — `viewer_can_manage`, owner paths, GDPR

**Files:**
- Modify: `internal/comments/handlers.go` (UpdateComment ~line 794-814, DeleteComment ~line 863-910, list/get reply shapes), `internal/comments/replies.go` (list handlers that map rows to ForumReply payloads), `internal/gdpr/service.go`, `internal/comments/store.go` (GetCommentRow usage)

**Interfaces:**
- Consumes: `viewer_can_manage` on read rows (Task 1), `existing.CreatedByUserID` on `gen.GetCommentRow`.
- Produces: `can_manage` boolean in comment JSON payloads; GDPR clears `created_by_user_id`.

- [ ] **Step 1: Failing test** — a comment whose `author_user_id` is a bot but `created_by_user_id` is the caller → `viewer_can_manage=true`; author-deletion via `SoftDeleteCommentByAuthor` succeeds for the creator.

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement**

- Update/Delete handlers: after `existing, err := h.Service.GetComment(...)`, add
  `isCreator := existing.CreatedByUserID.Valid && existing.CreatedByUserID.Bytes == user.UserID.Bytes`
  and treat `isAuthor || isCreator` as the author path (perm check stays
  `comments.edit_own`/`delete_own` — the SQL `OR created_by_user_id` makes it atomic).
- Response mapping: wherever list rows serialize, add `can_manage` from
  `viewer_can_manage`; **never** include `created_by_user_id` in any payload
  (grep the response builders — strip if a raw row is returned).
- `replies.go` `ForumReply`/`Comment` payload structs gain `CanManage bool \`json:"can_manage"\``.
- GDPR (`internal/gdpr/service.go`): where `AnonymizeCommentsByAuthor` is
  invoked, also call `q.ClearCommentsCreatedBy` + `q.ClearForumRepliesCreatedBy`.

- [ ] **Step 4: Run — PASS** (`go test ./internal/comments/ ./internal/gdpr/`)

- [ ] **Step 5: Commit** — `feat(comments): creator manage rights on persona-attributed comments`

---

### Task 5: Votes — `actor_id` (comment + content votes, list state)

**Files:**
- Modify: `internal/comments/handlers.go` (VoteComment:915, RemoveVote:952, VoteContent:981, RemoveContentVote:1018, and the list handlers that pass `@viewer_id`)

**Interfaces:**
- Consumes: `sqlc.narg(vote_actor_id)` list param (Task 1), resolver.
- Produces: POST bodies `{value, actor_id?}`; DELETE `?actor_id=`; `GET /content/{id}/comments?actor_id=`.

- [ ] **Step 1: Failing test** — POST `{value:1, actor_id:<bot>}` with perm → vote row keyed to bot; `?actor_id=` swaps `user_vote` on list.

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement**

Add a shared helper in `internal/comments/handlers.go`:

```go
func (h *Handlers) resolveActor(w http.ResponseWriter, r *http.Request, operator pgtype.UUID, raw string) (pgtype.UUID, bool) {
	if raw == "" {
		return operator, true
	}
	id, err := personas.ResolveDelegatedActor(r.Context(), h.Service, rbac.FromContext(r.Context()), operator, raw)
	if err != nil {
		personas.WriteDelegatedError(w, err, "actor_id")
		return pgtype.UUID{}, false
	}
	return id, true
}
```

- `VoteComment`: input gains `ActorID *string`; `actor, ok := h.resolveActor(...)`; `VoteComment(ctx, commentID, actor, value)`; audit `persona_attrib.vote` when `actor != operator`.
- `RemoveVote`: `?actor_id=` → same resolution → `RemoveCommentVote(ctx, commentID, actor)`; audit.
- `VoteContent`/`RemoveContentVote`: identical.
- Comment-list plumbing: `ListComments` (`handlers.go:361`) and the replies
  branch (`:413`) read `?actor_id=` → resolve via `resolveActor` (empty →
  caller) → thread the actor through the service signatures, which gain a
  param:

```go
ListCommentsByContent(ctx, contentID, viewerID, actorID pgtype.UUID, sort string, limit, offset int32)
ListRepliesByParent(ctx, contentID, parentID, viewerID, actorID pgtype.UUID, limit, offset int32)
```

  The gen params then set `ViewerID: viewerID, VoteActorID: actorID` —
  `@viewer_id` stays the caller for the block filter AND `viewer_can_manage`;
  `vote_actor_id` only feeds the `user_vote` subquery (see Task 1 SQL).
  Update `coerceNewRows`/`coerceOldestRows` (`store.go:557,590`) to copy
  `ViewerCanManage` into the canonical row struct.

- [ ] **Step 4: Run — PASS** (`go test ./internal/comments/`)

- [ ] **Step 5: Commit** — `feat(comments): actor_id override on comment/content votes`

---

### Task 6: Reactions — `actor_id`

**Files:**
- Modify: `internal/content/handlers.go` (AddReaction:3080, RemoveReaction:3253, `GET /content/{id}/reactions/me` handler), content store/service reaction paths

**Interfaces:**
- Consumes: resolver; `h.Service` already has `IsBotUser` (content.Service).
- Produces: `actor_id` body/`?actor_id=` on reaction write/delete/read-state.

- [ ] **Step 1: Failing test** — reaction with `actor_id` records under bot; `reactions/me?actor_id=` returns bot's set; operator-blocked OR actor-blocked → denied.

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement** — same pattern as Task 5: parse `actor_id` → `personas.ResolveDelegatedActor(r.Context(), h.Service, rbac.FromContext(r.Context()), user.UserID, raw)` → on error `personas.WriteDelegatedError(w, err, "actor_id")`. Add `actor_id` to `AddReaction` input; resolve → pass actor to service; block check `IsContentAuthorBlocked` runs for operator AND actor; `RemoveReaction`/`RemoveMyReaction` + `reactions/me` take `?actor_id=`; audit `persona_attrib.reaction`.

- [ ] **Step 4: Run — PASS** (`go test ./internal/content/`)

- [ ] **Step 5: Commit** — `feat(content): actor_id override on reactions`

---

### Task 7: Forum — `author_id` on thread/reply + `created_by` manage rights

**Files:**
- Modify: `internal/forum/handlers.go` (CreateThread:542, CreateReply:1530, ListReplies:1603), `internal/forum/service.go` (CreateThread:73, CreateReplyInput:385 + CreateReply:396, UpdateReply:674-718, DeleteReply:724-753), `internal/forum/store.go`

**Interfaces:**
- Consumes: resolver, `viewer_can_manage` rows, `fr.created_by_user_id`.
- Produces: `CreateReplyInput{ParentReplyID, Body, IsAnonymous, CreatedByUserID}`; `Service.IsBotUser`; forum reply payloads expose `can_manage`.

- [ ] **Step 1: Failing test** — `POST /thread/{id}/replies {author_id: bot}` → reply authored by bot + `viewer_can_manage` true for operator; `author_id`+`is_anonymous` → 400.

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement**

- `Service.IsBotUser` via `s.read.GetUserIsBot` (or store wrapper — match existing store access pattern).
- `CreateReplyInput.CreatedByUserID pgtype.UUID`; `CreateForumReplyParams.CreatedByUserID` wired; service signature `CreateReply(ctx, threadID, authorUserID, input)` — authorUserID = effective author.
- `CreateReply` handler: input gains `AuthorID *string`; mutually-exclusive check with `is_anonymous`; resolve → pass bot as author + `CreatedByUserID: user.UserID`; `dispatchMentionNotifications` actor = bot; audit `persona_attrib.thread_reply`.
- `CreateThread` handler: input gains `AuthorID *string` → resolve → `CreateThread(ctx, botID, input)`; audit `persona_attrib.content` with entity_type `forum_thread`.
- `UpdateReply`/`DeleteReply` service: widen the owner check to
  `reply.AuthorUserID == userID || reply.CreatedByUserID == userID` (keep
  `canModerateReply` moderator path; the author-path F-009 access check
  applies to the creator too).
- Signature changes — `ListRepliesByThreadWithAuthor` gains a viewer param
  (`ListRepliesByThreadWithAuthor(ctx, threadID, viewerID)`); the
  `ListReplies` handler passes the caller. `GetReplyByIDWithAuthor` also
  gains `viewer_id` for the computed flag — **mind the other callers**: it
  is also used for the create-response row (`handlers.go:1592` → pass the
  operator) and JSON-LD/accepted-answer paths (public → pass zero UUID,
  `viewer_can_manage` evaluates false).
- Surface `can_manage` in the reply payload; never serialize
  `created_by_user_id`.
- GDPR hookup covered in Task 4 (`ClearForumRepliesCreatedBy`).

- [ ] **Step 4: Run — PASS** (`go test ./internal/forum/`)

- [ ] **Step 5: Commit** — `feat(forum): author_id override + creator rights on thread replies`

---

### Task 8: Forum votes — `actor_id`

**Files:**
- Modify: `internal/forum/handlers.go` (VoteOnThread:1814, RemoveThreadVote:1871, thread detail `ListUserVotesForThread` call site ~line 1174)

**Interfaces:**
- Produces: `{entity_type, value, actor_id?}` on POST; `?actor_id=` on DELETE and on `GET /thread/{id}` user_vote hydration.

- [ ] **Step 1: Failing test** — vote as persona on thread + reply entity types; `?actor_id=` returns persona's `user_vote`.

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement** — local `resolveActor` helper (same shape as comments); `VoteOnThread` input gains `ActorID *string` → `s.Service.Vote(ctx, actor, input)` (self-vote checks already compare actor to entity author); `RemoveThreadVote` `?actor_id=`; thread detail + replies list vote-state hydration calls `ListUserVotesForThread(actor)` when `?actor_id=` present; audit `persona_attrib.vote`.

- [ ] **Step 4: Run — PASS** (`go test ./internal/forum/`)

- [ ] **Step 5: Commit** — `feat(forum): actor_id override on thread/reply votes`

---

### Task 9: Follows — `actor_id`

**Files:**
- Modify: `internal/follows/handlers.go` (ToggleFollow:94-187, GetFollowStatus:373), `internal/follows/service.go` / `store.go` (`IsBotUser`)

**Interfaces:**
- Produces: `POST /follows {entity_type, entity_id, actor_id?}`; `POST /users/{id}/follow` optional body `{actor_id}`; `DELETE ?actor_id=`; `follow-status?actor_id=`.

- [ ] **Step 1: Failing test** — follow as persona records `follower_id = bot`; `follow-status?actor_id=` reflects it; `ErrUserBlocked` path still applies to the actor.

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement** — `actor_id` parsed in both body branches of `ToggleFollow` (path route: optional JSON body `{actor_id}` — decode leniently, empty body still works); `EnsureFollow`/`EnsureUnfollow` called with the actor; `GetFollowStatus` honors `?actor_id=`; audit `persona_attrib.follow` with entity_type = the follow's entity_type.

- [ ] **Step 4: Run — PASS** (`go test ./internal/follows/`)

- [ ] **Step 5: Commit** — `feat(follows): actor_id override for follow-as-persona`

---

### Task 10: Geo default on content create/update

**Files:**
- Modify: `internal/content/handlers.go` (author_id call sites ~1210, ~2345), `internal/content/store.go` (geo lookup), `internal/db/queries/users.sql` or `contents.sql`

**Interfaces:**
- Produces: `gen.GetUserCurrentGeoSpace :one SELECT current_geo_space_id FROM users WHERE id = @id` (add to `users.sql` if absent — check first: `grep -rn "current_geo_space_id" internal/db/queries/`).

- [ ] **Step 1: Failing test** — `POST /content {author_id: bot}` with no `geo_space_id` → content row gets bot's `current_geo_space_id`; explicit `geo_space_id` wins.

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement** — after `resolveBotAuthor` succeeds, when `input.GeoSpaceID` is unset: `s.read.GetUserCurrentGeoSpace(ctx, authorID)` → set `geo_space_id` when valid. Same at the update call site (only when neither input nor existing row carries a value? — per spec: default only when not supplied in the same request; leave existing value otherwise).

- [ ] **Step 4: Run — PASS** (`go test ./internal/content/`)

- [ ] **Step 5: Commit** — `feat(content): persona geo default on author_id override`

---

### Task 11: `GET /api/v1/me/personas`

**Files:**
- Modify: `internal/db/queries/personas.sql`, `internal/personas/handlers.go` (new handler `ListMyPersonas`), `internal/personas/service.go`, `internal/server/server.go` (~line 4613 `/me/following` block)

**Interfaces:**
- Produces: `GET /me/personas → {data:[{persona_id,user_id,name,username,avatar_url,geo_space_id,geo_space_name,geo_space_level}]}` gated by `content.assign_author`.

- [ ] **Step 1: Failing test** — handler returns active personas only; non-permitted → 403 (route-level gate).

- [ ] **Step 2: Run — FAIL**

- [ ] **Step 3: Implement**

`personas.sql`:

```sql
-- name: ListAssignablePersonas :many
-- Active personas offered to "interact as" pickers (content.assign_author
-- gated at the route). Slim join: byline + geo.
SELECT ap.id AS persona_id, u.id AS user_id,
 COALESCE(up.display_name, u.username) AS name,
 u.username, COALESCE(up.avatar_url, '') AS avatar_url,
 gs.id AS geo_space_id, gs.name AS geo_space_name, gs.level AS geo_space_level
FROM ai_personas ap
JOIN users u ON u.id = ap.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
LEFT JOIN spaces gs ON gs.id = u.current_geo_space_id
WHERE ap.status = 'active' AND u.deleted_at IS NULL
ORDER BY up.display_name NULLS LAST, u.username;
```

`service.go` `ListAssignable(ctx)` → store → row → slim struct; `handlers.go` `ListMyPersonas` (RequireAuth'd route; permission enforced by route middleware); mount:

```go
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "content.assign_author")).Get("/me/personas", personasHandlers.ListMyPersonas)
```

- [ ] **Step 4: Run — PASS** (`go test ./internal/personas/`)

- [ ] **Step 5: Commit** — `feat(personas): GET /me/personas assignable list`

---

### Task 12: Frontend — persona store, "Interact as" picker, comment/thread plumbing

**Files:**
- Create: `apps/main/src/lib/api/personas.ts`, `apps/main/src/lib/stores/act-as-persona.svelte.ts`, `apps/main/src/lib/components/personas/InteractAsPicker.svelte`
- Modify: `apps/main/src/lib/components/forum/ReplyThread.svelte` (+ `ReplyItem.svelte` `can_manage` affordance), thread page component that posts replies + votes (`apps/main/src/lib/api/` thread + comments api files), `apps/main/src/lib/api/comments.ts`

**Interfaces:**
- Consumes: `GET /me/personas` (Task 11); `author_id`/`actor_id` backend fields.
- Produces: `personaAuthors` store `{load(), items}`; `actAsPersona` page store `{selected: Persona|null}`; `InteractAsPicker` component `<InteractAsPicker bind:value={actAsPersona.selected} />`.

- [ ] **Step 1: `personas.ts` API client**

(`api.get` unwraps the envelope's `data` field — see
`apps/main/src/lib/api/reactions.ts` for the pattern):

```ts
import { api } from '#lib/api/client.js';
export interface AssignablePersona {
	persona_id: string; user_id: string; name: string; username: string;
	avatar_url: string; geo_space_id: string | null;
	geo_space_name: string | null; geo_space_level: string | null;
}
export async function listAssignablePersonas(): Promise<AssignablePersona[]> {
	const res = await api.get<AssignablePersona[]>('/me/personas');
	return res.data ?? [];
}
```

- [ ] **Step 2: `act-as-persona.svelte.ts`** — runes store (`$state`), `load()` lazy-fetches when caller has `content.assign_author` (read from page permissions the same way `SaveBar` gates `botAuthors`); `selected` resets on navigation (`afterNavigate`).

- [ ] **Step 3: `InteractAsPicker.svelte`** — compact select chip ("Interact as: You ▾") listing personas with `@username · geo_space_name` sublabels (reuse the `SaveBar` botAuthors Autocomplete pattern — `apps/main/src/lib/components/create/SaveBar.svelte` is the reference).

- [ ] **Step 4: Wire ReplyThread + thread page**

- Mount the picker next to the top-level composer (visible only when `personaAuthors.items.length > 0`).
- `postReply`/`postComment` calls pass `author_id: selected?.user_id` when set.
- Vote calls (`commentsApi.vote`, content vote, `threadApi` vote) pass `actor_id` in POST bodies and `?actor_id=` on DELETEs; list fetches forward `?actor_id=` so `user_vote` reflects the selected persona.
- `ReplyItem` edit/delete visibility: `canModerate || author==me || reply.can_manage`.

- [ ] **Step 5: Verify** — `cd apps/main && bunx svelte-check` (0 new errors), manual: select London persona → comment posts under persona byline, VoteRail state follows selection, edit/delete appears on the persona comment.

- [ ] **Step 6: Commit** — `feat(main): interact-as-persona picker for comments, threads, votes`

---

### Task 13: Frontend — follows, reactions, studio sublabel, i18n

**Files:**
- Modify: `FollowButton` + `follows` api client (actor_id on POST/DELETE + `follow-status?actor_id=`), reaction bar + reactions api (`/reactions`, `/reactions/me?actor_id=`), `apps/main/src/lib/components/create/SaveBar.svelte` (geo sublabel), `apps/main/src/lib/i18n/en.json`, `fr.json` (+ other locale files per convention)

- [ ] **Step 1:** reaction + follow api signatures gain an optional `actorId` param; components read `actAsPersona.selected`.
- [ ] **Step 2:** `SaveBar` persona picker `sublabel` → `` `@${username}${geo_space_name ? ' · ' + geo_space_name : ''}` ``.
- [ ] **Step 3:** i18n keys: `personas.interact_as`, `personas.you`, etc. (follow existing key conventions in `en.json`).
- [ ] **Step 4: Verify** — `bunx svelte-check`; commit — `feat(main): persona actor on follows/reactions, geo sublabels`

---

### Task 14: JEOS bookkeeping

**Files:**
- Modify: `.jeos/conventions/api-routes-catalog.md`, `.jeos/sections/49-*.md` (comments), `.jeos/sections/08-*.md` (forum), `.jeos/sections/31-*.md` (follows), `.jeos/sections/68-ai-personas.md`

- [ ] **Step 1:** add `GET /me/personas` + `author_id`/`actor_id`/`vote_actor_id` fields to the catalog; `created_by_user_id` notes in §49/§08; delegated-authorship note + `persona_attrib.*` action types in §68; `actor_id` note in §31.
- [ ] **Step 2:** Commit — `docs(jeos): persona delegated-authorship + created_by + /me/personas`

---

## Post-implementation verification

- `go build ./... && go test ./internal/comments/ ./internal/forum/ ./internal/follows/ ./internal/content/ ./internal/personas/ ./internal/gdpr/`
- `cd apps/main && bunx svelte-check`
- Manual: persona picker on a thread → reply as London persona → `can_manage` edit/delete visible; operator vote state vs persona vote state distinct via `?actor_id=`; audit rows in `audit_logs` (`persona_attrib.*`).
