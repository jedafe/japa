# Persona-Attributed Interactions & Persona Profile Editing — Design

Date: 2026-11-10 · Status: approved (user Q&A resolved)

## Model

Operators holding `content.assign_author` can already attribute a content
row to a bot persona (`POST /content` `author_id`, bot-only, §68/SEC-002).
This spec extends the same delegated-authorship pattern to **comments,
replies, votes, and reactions**, and lets the persona's **geo identity**
flow into the authored entity. It also adds **persona profile editing**
(avatar, cover, profession, etc.) to the admin panel now and the public
profile page as a follow-up.

No session/profile switching: the operator keeps their own identity; each
write carries an explicit delegate field (`author_id`/`actor_id`).

Decisions locked with the user:

- Eligibility: one permission gate (`content.assign_author`) → any active
  persona. No per-persona assignment table.
- Metrics: persona-attributed actions count as normal events (no
  `is_synthetic` tagging on human-driven writes).
- Disclosure: operators may drive both `disclosed` and `undisclosed`
  personas.
- Comment management: `comments.created_by_user_id` preserves operator
  edit/delete rights on persona-authored comments.
- Geo: auto-inherit the persona's geo — content defaults
  `geo_space_id` to the persona's `current_geo_space_id`; comment geo is
  byline-derived.
- Scope: comments + replies **and** reactions/votes in v1.
- Profile editing surface: admin panel modal first, main-app `/u/[username]`
  affordance as a follow-up phase.

## Part 1 — Interact as persona

### Backend

#### 1.1 Shared delegated-actor resolver

Extract `resolveBotAuthor` from `internal/content/handlers.go` into a
shared helper in `internal/personas` (persona-domain home):

```
ResolveDelegatedActor(ctx, rbacResolver, botChecker, operatorID, raw) (pgtype.UUID, error)
```

Contract (identical to today's):

- `400 invalid_author_id` — not a UUID / user does not exist
- `403 author_assign_forbidden` — caller lacks `content.assign_author`
  (super_admin bypasses via `IsSuperAdmin`)
- `400 author_not_bot` — target exists but `is_bot = false`

`internal/content` keeps `resolveBotAuthor` as a thin wrapper so existing
callers and error codes don't change. `internal/comments` gains an
`IsBotUser`-style check wired the same way the comments service wires its
other adapters (`PermissionChecker` interface + `SetBotChecker`).

#### 1.2 Comments — `author_id` + `created_by`

`POST /api/v1/content/{id}/comments` body gains optional `author_id`
(applies to top-level comments and `parent_id` replies alike):

- `author_id` + `is_anonymous: true` in the same request →
  `400 mutually_exclusive` — an anonymous comment cannot also carry an
  explicit author.
- Resolved via 1.1; `author_user_id` = persona backing user;
  `created_by_user_id` = operator.

Migration (up + down):

```sql
ALTER TABLE comments
  ADD COLUMN created_by_user_id uuid NULL
    REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_comments_created_by ON comments(created_by_user_id)
  WHERE deleted_at IS NULL;
```

sqlc query changes (`db/queries/comments.sql` → `make sqlc-gen`):

- `CreateComment`/`CreateReply` params gain `created_by_user_id`.
- `ListCommentsByContent`, `ListRepliesByParent`, `GetComment` select a
  computed per-viewer flag:
  `(c.author_user_id = @viewer OR c.created_by_user_id = @viewer) AS viewer_can_manage`.
  The raw `created_by_user_id` is **never** returned in public payloads —
  it would dox the operator behind an otherwise anonymous-looking bot.
- `UpdateComment`/`DeleteComment` allow
  `author_user_id = caller OR created_by_user_id = caller OR comments.edit_any/delete_any`.
- GDPR erasure (`internal/gdpr`) adds
  `UPDATE comments SET created_by_user_id = NULL WHERE created_by_user_id = @user_id`
  to the erasure path.

Permission split on create:

- Space `commenting_perm`/`replying_perm` checks evaluate the **effective
  author** (persona backing user) — geo-space memberships live on the bot,
  which is what makes a London persona able to comment in London spaces.
- Spam pipeline (`SpamPipeline.Check` + `RecordContentCreation`) evaluates
  the **operator** — prevents persona-attributed spam from bypassing the
  operator's rate limit.
- Notifications (`DispatchCommentNotifications`) and fan-out use the
  comment author = persona automatically.

#### 1.3 Votes — `actor_id`

- `POST /api/v1/comments/{id}/vote` and `POST /api/v1/content/{id}/vote`
  bodies gain optional `actor_id`.
- `DELETE` on both gains `?actor_id=` query param (remove whose vote).
- Resolved via 1.1 with the same error contract (`actor_id` naming —
  nothing is "authored" by a vote).
- `GET /api/v1/content/{id}/comments` gains `?actor_id=` — when present,
  `user_vote` on returned rows is computed for that actor instead of the
  viewer (bot-validated the same way; invalid → same error codes). The
  viewer's own vote state is not mixed in — the UI shows exactly one
  identity's state at a time.

#### 1.4 Reactions — `actor_id`

- `POST /api/v1/content/{id}/reactions` (+ legacy `/react` alias) body
  gains optional `actor_id`; `DELETE .../reactions/{key}` and
  `DELETE /content/{id}/react` gain `?actor_id=`.
- `GET /api/v1/content/{id}/reactions/me?actor_id=` returns the persona's
  reaction set for UI state.
- Block check (`IsContentAuthorBlocked`): denied when **either** the
  operator or the effective actor is blocked by the content author.
- Visibility (`CanReactToContent`) and notification actor both evaluate
  the effective actor — the persona is the reactor of record.

#### 1.5 Geo default on content

When `author_id` resolves to a persona on `POST /content` or
`PATCH /content/{id}` and `geo_space_id` is not supplied in the same
request → default `contents.geo_space_id` to the persona's
`users.current_geo_space_id` (lookup in the resolve path, one indexed
read). An explicit `geo_space_id` always wins.

Comments carry no geo column — a persona comment's byline geo comes from
the backing user's `current_geo_space_id` automatically.

#### 1.6 `GET /api/v1/me/personas` (new, non-admin)

`content.assign_author`-gated; returns active personas only
(`ai_personas.status = 'active'`):

```json
{ "data": [{ "persona_id", "user_id", "name", "username", "avatar_url",
             "geo_space_id", "geo_space_name", "geo_space_level" }] }
```

Feeds every "interact as" picker on non-admin surfaces. The studio
composer may migrate off `GET /admin/personas` to this endpoint (optional,
not required for v1).

#### 1.7 Audit trail

Every persona-attributed mutation writes an `audit_logs` row (same helper
pattern as `impersonation.auditLog`):

- `actor_user_id` = operator, `impersonated_user_id` = bot user,
  `action_type` = `persona_attrib.{comment|vote|reaction|content}`,
  `metadata` = `{entity_type, entity_id, persona_id?}`.
- The comment path additionally persists `created_by_user_id` (needed for
  management rights; the audit row is the compliance trail for
  votes/reactions which have no `created_by` column).

#### 1.8 Error codes (standard envelope)

| Code | Status | When |
|---|---|---|
| `invalid_author_id` / `invalid_actor_id` | 400 | Bad UUID / user missing |
| `author_assign_forbidden` | 403 | No `content.assign_author` |
| `author_not_bot` | 400 | Target is a real user |
| `mutually_exclusive` | 400 | `author_id` + `is_anonymous` |

### Frontend (apps/main)

- **`persona-authors.svelte.ts` store** — lazy `GET /me/personas`, fetched
  once per session when a composer mounts and the caller's permissions
  (page data, same mechanism studio uses) include `content.assign_author`.
  Exposes picker items `{id: user_id, label: name, sublabel: '@username · geo_space_name'}`.
- **`actAsPersona` page-scoped store** — selected persona or `null` (self);
  reset on navigation.
- **"Interact as" chip** — small `Autocomplete`-style picker rendered by
  `ReplyThread` next to the top-level composer when personas exist:
  "Interact as: You ▾" → persona list with geo sublabels. Selection
  governs comments, comment votes, and content reactions/votes on that
  page.
- **Plumbing** — `commentsApi.postReply(parentId, body, {author_id})`,
  vote calls gain `{actor_id}` body / `?actor_id=` delete param,
  `fetchCommentPage`/`listAllComments` forward `?actor_id=` for persona
  `user_vote` state; `ForumReply` gains `can_manage` (from
  `viewer_can_manage`) and `ReplyItem` shows edit/delete when
  `canModerate || author==me || can_manage`.
- **Reactions** — the reaction bar passes `actor_id` when a persona is
  selected; `/reactions/me` fetched with `?actor_id=`.
- **Studio composer** — `SaveBar` persona picker sublabel gains the geo
  name (`@handle · London`); `GET /me/personas` may replace the
  `/admin/personas` fetch.
- i18n keys added to `en.json`/`fr.json` (+ other locale files per
  convention).

## Part 2 — Persona profile editing

### Backend — extend `PATCH /admin/personas/{id}`

`UpdatePersonaInput` + `HandleUpdate` gain profile fields that land on the
backing `users`/`user_profiles` rows (same write path `name`/`tagline`/
`username`/`geo_space` already use):

| JSON field | Destination |
|---|---|
| `avatar_url` / `avatar_media_id` | `user_profiles.avatar_url` / `avatar_media_id` (media id resolved to URL, `PATCH /users/me` semantics) |
| `cover_url` / `header_media_id` | `user_profiles.cover_url` / `header_media_id` |
| `profession` | `users.profession` |
| `location` | `user_profiles.location` |
| `website_url` | `user_profiles.website_url` |
| `education` | `users.education` |
| `interests` | `users.interests` |
| `disclosure_mode` | `ai_personas.disclosure_mode` (exists — "show as AI") |
| `geo_space` | geo chain + `current_geo_space_id` (exists) |
| `name` / `tagline` / `username` | (exists) |

All optional; empty string clears text fields, `null`/omitted leaves
unchanged. Audit row gains the new field names in `changed`.

`GetPersona`/`ListPersonas` responses gain `avatar_url`, `cover_url`,
`profession`, `location` (+ `website_url`, `education`, `interests` on
detail) so the forms seed current values.

### Phase A — admin panel (`apps/admin` personas page)

Extend the existing edit modal:

- **Avatar** + **cover** pickers via `MediaPickerModal` (same component
  the tags migration uses); preview thumbnails in the form.
- Text fields: `profession`, `location`, `website_url`, `education`,
  `interests` (chips input), `name`, `tagline`, `username` (existing).
- Disclosure toggle (existing select), geo cascade (existing).
- Save → `PATCH /admin/personas/{id}` with the union of the new fields.

### Phase B — main app (follow-up)

- "Edit persona" affordance on `/u/[username]` profile pages, visible when
  the viewer holds `persona.manage` **and** the profile belongs to an
  `ai_personas` backing user (profile payload / `/me/personas` can carry
  the flag).
- Inline editor sheet reusing `MediaLibraryModal` for avatar/cover;
  submits to the same `PATCH /admin/personas/{id}` (RBAC-gated, not
  surface-gated).

## Non-goals (v1)

- Persona session/profile switching (dropped per user direction).
- Forum replies (§08 `forum_replies`) as-persona — separate subsystem; the
  same resolver pattern can follow.
- Bookmarks, follows, DMs, org posting changes.
- AI orchestration holds (no human sessions exist to pause — dropped with
  the session model).
- New RBAC node — `content.assign_author` is reused.

## JEOS updates (before/with code — repo rules)

- `conventions/api-routes-catalog.md` — `GET /me/personas`; `author_id` /
  `actor_id` fields on comment/vote/reaction routes; PATCH additions.
- Migration + up/down pair; `comments.created_by_user_id` in §49 notes.
- §68 personas — delegated-authorship note + profile-field list.
- `persona_attrib.*` audit action types.
- No new permission node, no new Valkey keys, no new Asynq tasks.

## Edge cases

- Persona retired/paused after comments exist — comments stay authored by
  the backing user; the picker only lists `status = 'active'`.
- `author_id` re-pointed on `PATCH /comments/{id}`? — **not supported**;
  author is immutable post-create (same as content edit).
- Deleting a persona-authored comment works via `created_by` or
  `comments.delete_any`; the audit row records the operator either way.
- `/reactions/me?actor_id=` with a non-bot id → `author_not_bot` (same
  validation; no silent fallback).
- Reaction removal with `?actor_id=` for a persona that never reacted →
  404-style service result, same as removing your own missing reaction.
