# Post-as-Organization + Org Public Profile Backend — Design Spec

**Date:** 2026-07-25
**Status:** Approved (brainstorming complete)
**Author decisions:** locked via brainstorming Q1–Q5 (2026-07-25)
**Predecessor:** [[user-public-profile-backend]] Phase A COMPLETE. This is Phase B.
**Governing sections:** JEOS Section 07 (Organization Profiles & Claim Ownership); relates to Section 32 (feed/audience), Section 33 (professional profiles — separate system, untouched).

---

## Goal

Make the organization public profile (`/org/[slug]`) fully server-able by unauthenticated/guest viewers, and give organizations real authored content by introducing a `content → organization` attribution link ("post as organization").

## Problem statement (from recon)

- All `/organizations/*` routes are `RequireAuth`-gated; `GetOrganization` returns the raw `gen.Organization` row (PII/over-exposure: `owner_user_id`, `field_values`, internal counts).
- Section 07 line 103 defines `GET /organizations/:id/content` ("org's published content") and the frontend wants org content tabs — **but there is no `content → organizations` link in the schema.** `contents.author_id → users`; `contents.author_profile_id → professional_profiles` (Section 33, a separate parallel system). The only `REFERENCES organizations` are members/invitations/boosts. Orgs cannot author content, react, comment, or own spaces.
- The `author_profile_id` column (mig 000021) is a **stub**: one read query (`ListContentsByAuthorProfile`) exists, but `CreateContent` never sets it — no write path populates it. It is NOT a working pattern to copy.

## Locked decisions (Q1–Q5)

1. **Authorship model = dual attribution.** Add a new nullable `contents.organization_id`. `author_id` remains `NOT NULL` (the human who wrote it — for moderation/accountability). `organization_id` is the optional org attribution. The `author_profile_id` stub is left untouched (orgs are NOT merged with professional_profiles).
2. **Post-as-org permission = owner + admin + editor.** When `organization_id` is supplied on create/update, the caller's `organization_members.role` for that org must be `owner|admin|editor`; `member` and non-members get 403.
3. **Org profile tabs = Posts, Media, About, Members, Spaces.** Likes and Replies are dropped — they are inherently user actions (`reactions.user_id`, `comments.author_user_id`); an org does not react or comment.
4. **Org Spaces tab = spaces the org posts in** — distinct public+active spaces where the org has published content (`organization_id → contents.space_id`). "Where the org is active." No member-privacy exposure.
5. **Dual-surface with badge.** An org-attributed post appears on BOTH `/org/[slug]` AND the authoring user's `/u/[username]` (the personal card badged "on behalf of Acme"). Phase A user queries stay keyed by `author_id`; the user `PostCard` gains org-attribution fields so the badge renders.

## Global Constraints

- Go monolith `github.com/jedafe/japa`, layered Handler(Chi)→Service→Store(sqlc/pgx)→PostgreSQL 18. No business logic in handlers; sqlc-only SQL.
- Multi-tenancy (ADR-004): every query filters `tenant_id`. Every new query MUST be tenant-scoped.
- Response contract: `{"data": ...}` / `{"data": [...], "meta": {...}}`; errors `{"error":{"code","message"}}`; IDs as strings, timestamps RFC3339.
- Public profile surfaces reuse Phase A patterns: `OptionalAuth`, lean response DTOs (no internal columns), and a shared active-entity 404 gate.
- Integration tests against a real Postgres via `internal/testdb` (ADR-061), build tag `integration`.
- Work-on-main-in-place; commit by exact path (cron auto-committer bundles the tree). Catalogs (`api-routes-catalog.md`) updated in the same change (JEOS Gate 2).

---

## Architecture

Two sub-projects, sequenced. B1 is independent of the schema change; B2 is the enabling authorship half plus the content tabs that depend on it.

### Sub-project B1 — Org public read surface (no migration)

| Surface | Route | Change |
|---|---|---|
| Header | `GET /organizations/{slug}` | `RequireAuth`→`OptionalAuth`; new lean `PublicOrgResponse`; wrap with active-org gate |
| Members | `GET /organizations/{id}/members` | `RequireAuth`→`OptionalAuth`; gate; lean member DTO (public user shape + role; active members only; drop `invited_by`/email) |
| About | (part of header payload) | subset: name, tagline, description, org_type, website_url, country_code, verified, follower_count, member_count, logo/header media |
| Public list | `GET /organizations` | (optional) public path returns `verified` + `active` only; admin path returns all |

- **`PublicOrgResponse`** (lean): `id, name, slug, tagline, description, org_type, logo_media_id, header_media_id, website_url, country_code, verified, verification_type, follower_count, member_count, created_at`. Drops `tenant_id`, `owner_user_id`, `field_values`, `status`, `deleted_at`, `geo_space_id` (or resolve geo to a lean space ref if the profile needs it).
- **`requireActiveOrg`** middleware (server package, mirror of `requireActiveProfileUser`): resolves tenant from context, reads the org key (`{slug}` for header, `{id}` for tabs — parameterized `paramKey` + a by-slug vs by-id variant), 404s unless `status='active' AND deleted_at IS NULL`. Unverified-but-active orgs pass (Section 07: claimed-but-unverified is normal). Needs `ActiveOrgExistsBySlug` and `ActiveOrgExistsByID` queries.
- **Member DTO:** `{ user: {id, username, display_name, avatar_url}, role, joined_at }`. Only members whose user `status='active'`.

### Sub-project B2 — Post-as-org + content tabs (migration + write-path + read tabs)

**Migration (pair):**
```sql
-- up
ALTER TABLE contents ADD COLUMN organization_id uuid REFERENCES organizations(id);
CREATE INDEX idx_contents_org ON contents (tenant_id, organization_id, status, published_at DESC)
  WHERE organization_id IS NOT NULL AND deleted_at IS NULL;
-- down
DROP INDEX IF EXISTS idx_contents_org;
ALTER TABLE contents DROP COLUMN IF EXISTS organization_id;
```

**Write path:**
- `CreateContent` / `UpdateContent` sqlc queries gain a nullable `organization_id` param (`sqlc.narg`).
- Content service: when `organization_id` is supplied, verify caller membership+role via `GetOrganizationMemberRole` (must be `owner|admin|editor`), else 403 (`org_post_forbidden`). `author_id` is always the authenticated user.
- **Edit rights:** updating an org post is allowed for the original `author_id` OR any `owner|admin|editor` of that org (collective ownership). Enforced in the update service.

**Read tabs (all `OptionalAuth` + `requireActiveOrg` by `{id}`, lean DTOs):**
- **Posts:** `GET /organizations/{id}/content` → new `ListPublishedContentByOrganization` (`organization_id=@org_id AND tenant_id AND status='published' AND visibility='public' AND deleted_at IS NULL AND auto_hidden=false`, offset-paginated). Lean `PostCard` (reuse Phase A shape).
- **Media:** `GET /organizations/{id}/media` → new `ListMediaFromOrgPublicPosts` (mirror Phase A media-from-posts: `EXISTS(contents c WHERE organization_id=org, published, public, featured/header image = m.id)`, `confirmed`, `user_group_id IS NULL`). Lean `MediaResponse`.
- **Spaces:** `GET /organizations/{id}/spaces` → new `ListSpacesByOrgPosts` (DISTINCT public+active spaces from the org's published-public posts). Lean `SpaceCard` (reuse Phase A).

**Cross-cutting consequences:**
- **User `PostCard` gains org-attribution** (`organization_id` + resolved `organization {id, name, slug, logo_media_id}`), so dual-surface renders the "on behalf of" badge on `/u/[username]`. The Phase A user Posts query stays keyed by `author_id`; add a LEFT JOIN to `organizations` for the badge fields. Same for the user Media tab if it surfaces org posts' media.
- **Feed:** feed item resolution includes org attribution when `organization_id` is set (byline shows the org). Update the feed content projection + response.
- **Search:** the content Meilisearch sync includes `organization_id` (and org name) so org posts are findable and the org profile is indexable. Enqueue on org-attributed writes (existing content sync task, extended payload).
- **Notifications:** replies/reactions on an org post notify the human `author_id` (existing behavior — `author_id` is always set). Org-admin fan-out is deferred (YAGNI).
- **RBAC:** no new global permission nodes — post-as-org is gated by org membership role, not a global node.

---

## ADR (to be written: ADR-062 — Organization Content Authorship)

- **Decision:** content attributes to an org via a new nullable `contents.organization_id`, alongside the always-present `author_id` (dual attribution). Orgs are not merged into `professional_profiles`; the stubbed `author_profile_id` path is left as-is.
- **Authority:** posting/editing as an org is gated by `organization_members.role ∈ {owner, admin, editor}`.
- **Visibility:** org posts are dual-surface (org profile + author profile with badge).
- **Scope:** org profile content = Posts, Media, Spaces (+ About, Members). Likes/Replies do not apply to org authorship.
- **Rationale:** dual attribution preserves moderation/accountability (human author always known) while giving orgs a first-class content identity; a dedicated column keeps the change incremental and avoids a risky refactor of the author model or a premature merge of two parallel profile systems.

---

## Testing strategy

Reuse `internal/testdb` (ADR-061). Add harness seeders: `SeedOrganization(OrgOpts{Slug, Status, Verified, OwnerUserID})`, `SeedOrgMembership(orgID, userID, role)`, and a `ContentOpts.OrganizationID` field so tests can seed org-attributed content.

Per-surface integration tests (real PG, negative cases):
- Header: active org → 200 lean (no `owner_user_id`/`field_values`); suspended/pending/deleted → 404; unverified-but-active → 200.
- Members: active members only; role present; `invited_by`/email absent.
- Write path: owner/admin/editor can set `organization_id`; member/non-member → 403; `author_id` always recorded.
- Posts/Media/Spaces tabs: only the org's published-public content; another org's posts and draft/private excluded; media limited to featured/header images of org public posts; spaces = distinct public spaces the org posted in.
- Dual-surface: an org post appears in both the org Posts query and the author's user Posts query (badge fields populated).

Mapper unit tests for every lean DTO (field-copy correctness + absence of internal keys), mirroring Phase A.

## Catalog / docs updates (same change)

- `.jeos/conventions/api-routes-catalog.md`: mark org profile routes `[public]` + gate note; add `/organizations/{id}/media` and `/organizations/{id}/spaces`; annotate `GET /organizations/{id}/content` as public.
- Section 07: reconcile the `:id/content` route with the new authorship model (note `organization_id`).

## Build order

1. **B1** (read surface) — independent, shippable first: header + members + about + active-org gate + public list filter.
2. **B2** (enabling + tabs) — migration → write-path/permission → Posts/Media/Spaces tabs → cross-cutting (user PostCard badge, feed, search) → ADR.

Each sub-project gets its own implementation plan (`writing-plans`) and subagent-driven execution.
