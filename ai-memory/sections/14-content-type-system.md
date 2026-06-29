# Section 14 — Content Type System

**Status:** ✅ Locked (restored post-audit, see ADR-029 — the discriminator/JSONB/Page Builder line below survived reconstruction; JSON-LD, geo, revisions, scheduling, and engagement counts did not and have been restored from earlier session material)

## Decided

`contents` table with `content_type` discriminator, `field_values` JSONB, `content_media_fields` for referential integrity. Behavior-pack pattern. Built-in v1 types: `blog_post`, `discussion`, `page`, `comparison` (Section 30). V2 types: `event`, `job_listing`, `service_listing`. The `page` content type's connection to Page Builder is fully resolved: a page's `field_values` stores the Page Builder block array directly — same storage pattern, not two parallel systems.

## Contents Table Schema (audit pass 2 — was missing CREATE TABLE)

```sql
CREATE TABLE contents (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id           uuid NOT NULL REFERENCES tenants(id),
  content_type        text NOT NULL,   -- 'blog_post'|'discussion'|'page'|'comparison'
  title               text NOT NULL,
  slug                text NOT NULL,
  body                jsonb NOT NULL DEFAULT '{}',  -- TipTap JSON (Section 15)
  field_values        jsonb NOT NULL DEFAULT '{}',  -- content-type-specific fields
  author_user_id      uuid NOT NULL REFERENCES users(id),
  author_profile_id   uuid REFERENCES professional_profiles(id) ON DELETE SET NULL,  -- Section 33
  geo_space_id        uuid REFERENCES spaces(id) ON DELETE SET NULL,  -- primary space (backward compat)
  visibility          text NOT NULL DEFAULT 'public'
                        CHECK (visibility IN ('public','members','private')),
  status              text NOT NULL DEFAULT 'draft'
                        CHECK (status IN ('draft','in_review','scheduled','published','archived')),
  publish_at          timestamptz,
  published_at        timestamptz,
  is_anonymous        boolean NOT NULL DEFAULT false,  -- Section 08
  is_featured         boolean NOT NULL DEFAULT false,  -- Section 24
  featured_order      int,
  featured_until      timestamptz,
  view_count          int NOT NULL DEFAULT 0,
  comment_count       int NOT NULL DEFAULT 0,
  share_count         int NOT NULL DEFAULT 0,
  reshare_count       int NOT NULL DEFAULT 0,
  bookmark_count      int NOT NULL DEFAULT 0,
  read_time_minutes   int,
  content_hash        text,            -- Section 41 spam prevention
  canonical_url       text,            -- Section 26 SEO
  og_title            text,
  og_description      text,
  og_image_url        text,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  deleted_at          timestamptz,
  UNIQUE(tenant_id, content_type, slug)
);
CREATE INDEX idx_contents_author   ON contents (tenant_id, author_user_id, status, created_at DESC);
CREATE INDEX idx_contents_space    ON contents (geo_space_id, status, published_at DESC)
  WHERE geo_space_id IS NOT NULL;
CREATE INDEX idx_contents_type     ON contents (tenant_id, content_type, status, published_at DESC);
CREATE INDEX idx_contents_schedule ON contents (tenant_id, status, publish_at)
  WHERE status = 'scheduled';
CREATE INDEX idx_contents_featured ON contents (tenant_id, is_featured, featured_order)
  WHERE is_featured = true;
CREATE UNIQUE INDEX idx_contents_hash ON contents (tenant_id, content_hash)
  WHERE content_hash IS NOT NULL;

-- Revision history: every save appends here; live contents.body = latest revision
CREATE TABLE content_revisions (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  content_id   uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
  body         jsonb NOT NULL,
  field_values jsonb NOT NULL DEFAULT '{}',
  edited_by    uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_revisions_content ON content_revisions (content_id, created_at DESC);

-- Referential integrity for media referenced in TipTap body blocks (Section 15)
CREATE TABLE content_media_fields (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  content_id  uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
  media_id    uuid NOT NULL REFERENCES media(id) ON DELETE CASCADE,
  field_key   text NOT NULL,   -- 'featured_image' | 'gallery_0' | 'body_block_{n}'
  position    int NOT NULL DEFAULT 0,
  UNIQUE(content_id, field_key, position)
);
CREATE INDEX idx_cmf_content ON content_media_fields (content_id);

-- JSON-LD templates per content type — admin-maintained, no deploy needed (Section 26)
CREATE TABLE content_type_jsonld_templates (
  content_type text PRIMARY KEY,
  template     jsonb NOT NULL,   -- JSON-LD with {{field_key}} interpolation placeholders
  updated_by   uuid REFERENCES users(id) ON DELETE SET NULL,
  updated_at   timestamptz NOT NULL DEFAULT now()
);
```

## Reactions & Bookmarks (audit pass 2 — entity_id BIGINT bug fixed, tenant_id added)

```sql
-- Reaction types: admin-configurable per tenant (Section 22)
CREATE TABLE reaction_types (
  key         text NOT NULL,
  tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  emoji       text NOT NULL,
  label       text NOT NULL,
  enabled     boolean NOT NULL DEFAULT true,
  position    int NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, key)
);

-- Polymorphic reactions across content + comments (entity_id was BIGINT — bug; fixed to uuid)
CREATE TABLE reactions (
  tenant_id    uuid NOT NULL REFERENCES tenants(id),
  entity_type  text NOT NULL,   -- 'content'|'comment'
  entity_id    uuid NOT NULL,   -- uuid, NOT BIGINT
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  reaction_key text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, entity_type, entity_id, user_id, reaction_key)
);
CREATE INDEX idx_reactions_entity ON reactions (tenant_id, entity_type, entity_id);
```

**JSON-LD per content type:** admin defines a JSON-LD template per content type (e.g. `Article`, `JobPosting`, `LocalBusiness`) in `content_type_jsonld_templates`, with field values from a given piece of content interpolated into the template at render time and injected into `<head>`. Fully dynamic — adding a new content type never requires a code change to get correct structured data, it's a template the admin fills in once. Feeds Section 26's SEO structured-data pipeline.

**Geo classification:** every content type can optionally attach a Geo Space (Section 05) via `geo_space_id`, the same field used for Country/State/City classification sitewide. This is what powers geo-filtered discovery ("Jobs in London," "Events in Toronto") without per-content-type custom filtering logic.

**Revisions:** `content_revisions` (content_id, field_values snapshot, edited_by, created_at) — every save creates a revision row rather than overwriting in place; the live row always reflects the latest revision, with full history available for rollback/audit.

**Scheduling:** a `publish_at` timestamp on any content row, with a background scheduler (`scheduled_publish` task, Section 11) polling for due rows and flipping status to `published`. Shared by Blog (Section 24), Ad Placement (Section 25), and any future content type needing future-dated publishing — not rebuilt per-feature.

**Polymorphic engagement counts:** `view_count`, `comment_count`, `share_count`, `reshare_count` live directly on `contents`, generalized rather than forum-specific. Reactions for all non-forum content use an admin-configurable emoji set (forum threads/replies use upvote/downvote instead — see Section 08):

```sql
reaction_types (
  key VARCHAR(50) PRIMARY KEY,  -- 'like', 'support', 'celebrate', 'insightful', 'grateful', 'funny'
  emoji TEXT NOT NULL, label TEXT NOT NULL, enabled BOOLEAN NOT NULL DEFAULT true
)
reactions (
  entity_type TEXT, entity_id BIGINT, user_id FK, reaction_key FK,
  created_at,
  PRIMARY KEY (entity_type, entity_id, user_id, reaction_key)
)
```

Admin can add/remove/customize the active reaction set from the Admin Panel (Section 22) without a code deploy. `bookmark_count` (Section 08 restoration) follows the same polymorphic `entity_type`/`entity_id` pattern, with a per-user bookmark folder structure for organization.

## See Also

ADR-029. Section 05 (Geo Spaces), Section 08 (forum-specific voting, bookmarks pattern origin), Section 11 (scheduled publish task), Section 16 (Page Builder block storage reuse), Section 22 (reaction-type admin config), Section 24 (scheduling reuse), Section 26 (JSON-LD consumer), Section 30 (Comparison content type registration).


## Additional Fields & Systems (post-audit additions)

**content_scores table** (drives Activity Feed sorting — Section 32):
```sql
CREATE TABLE content_scores (
  content_id          uuid PRIMARY KEY REFERENCES contents(id) ON DELETE CASCADE,
  vote_score          int NOT NULL DEFAULT 0,
  comment_count       int NOT NULL DEFAULT 0,
  view_count          int NOT NULL DEFAULT 0,
  bookmark_count      int NOT NULL DEFAULT 0,
  velocity_score      numeric NOT NULL DEFAULT 0,
  relevance_embedding vector(1536),
  last_scored_at      timestamptz NOT NULL DEFAULT now()
);
```

**read_time_minutes** — computed at save time from word count (`ceil(word_count / 200)`), stored as `int` on `contents`. Displayed in post cards and single post headers.

**content_hash** — SHA-256 of normalised body stored on `contents`. Unique index per tenant prevents duplicate posts (Section 41 spam prevention). Column: `content_hash text` on `contents`, with `CREATE UNIQUE INDEX idx_contents_hash ON contents (tenant_id, content_hash) WHERE content_hash IS NOT NULL;`. Set at create/update time via `hex(sha256(strip_markdown(body).toLowerCase().replaceAll(/\s+/g,' ').trim()))`.

**Slug history and auto-redirect** — critical for SEO equity: when a `contents` slug changes, the old slug is written to `slug_history` and a 301 is returned for the old URL:
```sql
CREATE TABLE slug_history (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  entity_type  text NOT NULL,   -- 'content'|'space'|'tag'|'user'
  entity_id    uuid NOT NULL,
  old_slug     text NOT NULL,
  new_slug     text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_slug_history_lookup ON slug_history (entity_type, old_slug);
```

**Canonical URL field** — `canonical_url text` on `contents`. When set, the page emits `<link rel="canonical" href="...">` pointing to the original source. Allows external bloggers to link to their original post when contributing content to the platform. Surfaced in the /create editor sidebar (Section 45) for blog_post and discussion types.

**Pagination style (platform + per-space override):** Admin sets the default pagination style in system_settings (`content.pagination.style`: `infinite_scroll`|`load_more`|`pagination`). Space moderators can override per space. Section 23 renders the appropriate control.

## content_spaces — Multi-Space Post Assignment (session 5 addition)

Every piece of content must have at least one primary geo space (country/state/city) — enforced at the API level for user-created content. Content can additionally belong to any number of custom spaces the author is a member of.

```sql
-- content_spaces: replaces single geo_space_id for non-system content
-- geo_space_id on contents is kept for backward-compat but content_spaces is authoritative
CREATE TABLE content_spaces (
  content_id  uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
  space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
  is_primary  boolean NOT NULL DEFAULT false,  -- exactly one row per content must be true
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (content_id, space_id)
);
CREATE INDEX idx_content_spaces_space ON content_spaces (space_id, created_at DESC);
CREATE INDEX idx_content_spaces_primary ON content_spaces (content_id) WHERE is_primary = true;

-- Constraint: at least one primary space per content (enforced at handler level, not DB level)
-- At publish: geo_space_id on contents is set to the primary space for query compatibility
```

The fan-out in Section 32 fires once per row in `content_spaces`, so a post assigned to three spaces fans out to all three.
