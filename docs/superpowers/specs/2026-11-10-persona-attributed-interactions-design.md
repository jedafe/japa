# Persona-Attributed Interactions & Persona Profile Editing — Design

Date: 2026-11-10 · Status: approved (user Q&A resolved)

## Model

Operators holding `content.assign_author` can already attribute a content
row to a bot persona (`POST /content` `author_id`, bot-only, §68/SEC-002).
This spec extends the same delegated-authorship pattern to **comments,
thread replies, votes, reactions, and follows**, lets the persona's **geo
identity** flow into the authored entity, and switches on the **persona
economy** (receive tips, accumulate points). It also adds **persona
profile editing** (avatar, cover, profession, etc.) to the admin panel
now and the public profile page as a follow-up.

No session/profile switching: the operator keeps their own identity; each
write carries an explicit delegate field (`author_id`/`actor_id`).

Decisions locked with the user:

- Eligibility: one permission gate (`content.assign_author`) → any active
  persona. No per-persona assignment table.
- Metrics: persona-attributed actions count as normal events (no
  `is_synthetic` tagging on human-driven writes).
- Disclosure: operators may drive both `disclosed` and `undisclosed`
  personas.
- Comment/reply management: `created_by_user_id` on `comments` and
  `forum_replies` preserves operator edit/delete rights.
- Geo: auto-inherit the persona's geo — content defaults
  `geo_space_id` to the persona's `current_geo_space_id`; comment geo is
  byline-derived.
- Scope: comments + thread replies + votes + reactions + follows in v1.
- Economy: personas can receive tips (settles to their own wallet) and
  accumulate points; bots stay off leaderboards.
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

Migration (up + down — covers both reply tables):

```sql
ALTER TABLE comments
  ADD COLUMN created_by_user_id uuid NULL
    REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_comments_created_by ON comments(created_by_user_id)
  WHERE deleted_at IS NULL;

ALTER TABLE forum_replies
  ADD COLUMN created_by_user_id uuid NULL
    REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_forum_replies_created_by ON forum_replies(created_by_user_id)
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

#### 1.3 Thread replies (§08 forum engine)

"Thread replies" and "forum replies" are the same subsystem —
`forum_replies` served under `/api/v1/thread/*` in `internal/forum`. Same
pattern as 1.2, applied to the forum engine's own tables:

- `POST /api/v1/thread` body gains optional `author_id` — thread creation
  attributed to the persona (parity with `POST /content`).
- `POST /api/v1/thread/{id}/replies` body gains optional `author_id` —
  `forum_replies.author_user_id` = persona backing user.
- `author_id` + `is_anonymous` → `400 mutually_exclusive` (F-002/C-003
  parity: anonymous replies never attribute an actor).
- Same migration adds `forum_replies.created_by_user_id uuid NULL
  REFERENCES users(id) ON DELETE SET NULL`; `CreateReply` sets it to the
  operator; `UpdateReply`/`DeleteReply` allow
  `author_user_id = caller OR created_by_user_id = caller OR forum.moderate`.
- `ListForumRepliesByThreadWithAuthor` + `GetReplyByIDWithAuthor` gain the
  `viewer_can_manage` computed flag; `created_by_user_id` never leaks.
- GDPR erasure clears `created_by_user_id` on `forum_replies` too (the
  table already has a `SET NULL author` precedent from migration 000467).
- Space `replying_perm` evaluates the effective author; rate-limiting
  (BL-012/`forum.max_replies_per_minute`-style settings) evaluates the
  operator. `dispatchMentionNotifications` names the effective author —
  for `author_id` replies the persona is the actor of record.

#### 1.4 Votes — `actor_id`

- `POST /api/v1/comments/{id}/vote` and `POST /api/v1/content/{id}/vote`
  bodies gain optional `actor_id`.
- `POST /api/v1/thread/{id}/vote` body gains `actor_id` — the single
  `VoteOnThread` handler already takes `entity_type: thread|reply`, so
  one field covers both thread and reply votes.
- `DELETE` on all three gains `?actor_id=` query param.
- Resolved via 1.1 with the same error contract (`actor_id` naming —
  nothing is "authored" by a vote).
- `GET /api/v1/content/{id}/comments`, `GET /api/v1/thread/{id}` (and its
  reply list) gain `?actor_id=` — when present, `user_vote` on returned
  rows is computed for that actor instead of the viewer (bot-validated
  the same way; invalid → same error codes). The viewer's own vote state
  is not mixed in — the UI shows exactly one identity's state at a time.

#### 1.5 Reactions — `actor_id`

- `POST /api/v1/content/{id}/reactions` (+ legacy `/react` alias) body
  gains optional `actor_id`; `DELETE .../reactions/{key}` and
  `DELETE /content/{id}/react` gain `?actor_id=`.
- `GET /api/v1/content/{id}/reactions/me?actor_id=` returns the persona's
  reaction set for UI state.
- Block check (`IsContentAuthorBlocked`): denied when **either** the
  operator or the effective actor is blocked by the content author.
- Visibility (`CanReactToContent`) and notification actor both evaluate
  the effective actor — the persona is the reactor of record.

#### 1.6 Follows — `actor_id`

A persona's backing user is a normal `users` row, so **following a
persona already works** — `POST /users/{persona_user_id}/follow` needs no
change. This section covers the operator following *as* a persona:

- `POST /api/v1/follows` body gains optional `actor_id`
  (`{entity_type, entity_id, actor_id}`).
- `POST /api/v1/users/{id}/follow` accepts `actor_id` in an optional JSON
  body (the existing path-param resolution stays — empty body still
  works); `DELETE` on both gains `?actor_id=`.
- `GET /api/v1/users/{id}/follow-status?actor_id=` reports the persona's
  follow state for the UI.
- `ErrUserBlocked` check applies to the effective actor; the
  follow-notification recipient sees the persona as follower.
- Applies to all followable `entity_type` values (user, space, tag) —
  the persona follows spaces/tags the same way.

#### 1.7 Persona economy — tips & points

A persona is a full `users` row, so both economies work on the backing
account — this section covers what's needed to switch them on:

- **Receiving tips** — `PATCH /admin/personas/{id}` gains `tips_enabled`
  (bool). When set, the personas service upserts `tip_settings` for the
  backing user **directly** (the `ensureWritable` self-write rule is
  bypassed — the gate is `persona.manage`, same as the rest of the PATCH).
  Minimum amount/methods take the platform defaults on first upsert.
  Requires `tips.receive` to resolve for the backing user (default member
  group — verify at implementation; no new node).
- **Settlement** — unchanged: `SettleFiatTip` → `walletOwnerFor` →
  `recipient_user_id` = backing user → the persona's own wallet credits.
  Persona wallets **accumulate**; bots have no payout/withdrawal path
  (out of scope). Refunds/clawbacks work unchanged.
- **Display** — `PublicUserTipConfig` and the `/u/[username]` tip button
  need no changes; once `tip_settings.tips_enabled` is set the persona's
  profile shows the tip CTA like any user.
- **Points** — no code change: human-driven persona actions already emit
  normal activity events → `EnqueuePointsAward` credits the persona's
  `wallet_ledger` (`ledger_type='points'`); the balance displays on the
  persona's profile. AI-orchestrated (`is_synthetic`) events keep their
  existing award behavior.
- **Leaderboards** — `LeaderboardAllTime`/`LeaderboardByPeriod` keep
  `u.is_bot = false`: personas accumulate points but do not rank.
  Intentional — a bot topping the human leaderboard undermines the board.
- `GetPersona` detail gains read-only `tips_enabled` and `points_balance`
  so admins can see the persona's economy state at a glance.

#### 1.8 Geo default on content

When `author_id` resolves to a persona on `POST /content` or
`PATCH /content/{id}` and `geo_space_id` is not supplied in the same
request → default `contents.geo_space_id` to the persona's
`users.current_geo_space_id` (lookup in the resolve path, one indexed
read). An explicit `geo_space_id` always wins.

Comments carry no geo column — a persona comment's byline geo comes from
the backing user's `current_geo_space_id` automatically.

#### 1.9 `GET /api/v1/me/personas` (new, non-admin)

`content.assign_author`-gated; returns active personas only
(`ai_personas.status = 'active'`):

```json
{ "data": [{ "persona_id", "user_id", "name", "username", "avatar_url",
             "geo_space_id", "geo_space_name", "geo_space_level" }] }
```

Feeds every "interact as" picker on non-admin surfaces. The studio
composer may migrate off `GET /admin/personas` to this endpoint (optional,
not required for v1).

#### 1.10 Audit trail

Every persona-attributed mutation writes an `audit_logs` row (same helper
pattern as `impersonation.auditLog`):

- `actor_user_id` = operator, `impersonated_user_id` = bot user,
  `action_type` = `persona_attrib.{comment|thread_reply|vote|reaction|
  follow|content}`, `metadata` = `{entity_type, entity_id, persona_id?}`.
- The comment/reply paths additionally persist `created_by_user_id`
  (needed for management rights; the audit row is the compliance trail
  for votes/reactions/follows which have no `created_by` column).

#### 1.11 Error codes (standard envelope)

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
- **"Interact as" chip** — small `Autocomplete`-style picker rendered
  next to the top-level composer when personas exist: "Interact as: You ▾"
  → persona list with geo sublabels. Rendered on `/thread/{slug}` (§08
  replies + thread/reply votes) and on comment surfaces (`ReplyThread`).
  Selection governs all persona-eligible actions on that page.
- **Plumbing** — `commentsApi.postReply`/`threadApi.postReply` gain
  `{author_id}`; vote calls gain `{actor_id}` body / `?actor_id=` delete
  param; comment/reply/thread fetches forward `?actor_id=` for persona
  `user_vote` state; reply rows gain `can_manage` (from
  `viewer_can_manage`) and `ReplyItem` shows edit/delete when
  `canModerate || author==me || can_manage`.
- **Follows** — `FollowButton` passes `actor_id` when a persona is
  selected; `follow-status` fetched with `?actor_id=`.
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
| `profession` | `user_profiles.profession` |
| `location` | `user_profiles.location` |
| `website_url` | `user_profiles.website_url` |
| `education` | `user_profiles.education` |
| `interests` | `user_profiles.interests` |
| `disclosure_mode` | `ai_personas.disclosure_mode` (exists — "show as AI") |
| `tips_enabled` | `tip_settings` upsert for backing user (new — §1.7) |
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
- Disclosure toggle (existing select), geo cascade (existing), and a
  "Can receive tips" toggle bound to `tips_enabled`.
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
- Bookmarks, DMs, poll votes, org posting changes.
- Tipping *as* a persona (spending the persona's wallet), persona
  payouts/withdrawals, leaderboard inclusion for bots.
- AI orchestration holds (no human sessions exist to pause — dropped with
  the session model).
- New RBAC node — `content.assign_author` is reused.

## JEOS updates (before/with code — repo rules)

- `conventions/api-routes-catalog.md` — `GET /me/personas`; `author_id` /
  `actor_id` fields on comment/thread/vote/reaction/follow routes; PATCH
  additions.
- Migration + up/down pair; `comments.created_by_user_id` in §49 notes,
  `forum_replies.created_by_user_id` in §08 notes.
- §68 personas — delegated-authorship note + profile-field list; §31
  follows — `actor_id` note; §125 tips — persona `tips_enabled` note.
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
