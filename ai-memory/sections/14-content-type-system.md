# Section 14 — Content Type System

**Status:** ✅ Locked (restored post-audit, see ADR-029 — the discriminator/JSONB/Page Builder line below survived reconstruction; JSON-LD, geo, revisions, scheduling, and engagement counts did not and have been restored from earlier session material)

## Decided

`contents` table with `content_type` discriminator, `field_values` JSONB, `content_media_fields` for referential integrity. Behavior-pack pattern. Built-in v1 types: `blog_post`, `discussion`, `page`, `comparison` (Section 30). V2 types: `event`, `job_listing`, `service_listing`. The `page` content type's connection to Page Builder is fully resolved: a page's `field_values` stores the Page Builder block array directly — same storage pattern, not two parallel systems.

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

**content_hash** — SHA-256 of normalised body stored on `contents`. Unique index per tenant prevents duplicate posts (Section 41 spam prevention).

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
