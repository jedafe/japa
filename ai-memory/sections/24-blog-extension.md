# Section 24 — Blog Extension

**Status:** ✅ Locked

## Decided
Tags only, no categories. Granular permission nodes (`blog.create`, `blog.edit_own`, `blog.edit_any`, `blog.publish`, `blog.feature`) through dynamic RBAC rather than fixed roles. Workflow: draft → in_review → scheduled → published, with reviewer feedback captured in a `content_reviews` table distinct from public comments. Scheduled publish reuses the `scheduled_publish` task. Featured posts manually curated (`is_featured`, `featured_order`, optional `featured_until` auto-expiry) rather than algorithmic. RSS feed (static XML, regenerated via the new `rss_regenerate` task on every publish).

**Blog Series:**

```sql
blog_series (
  id, tenant_id FK tenants, title, slug, description, cover_image_id FK NULL,
  author_user_id FK, status TEXT NOT NULL DEFAULT 'active',
  created_at, updated_at
)
blog_series_posts (
  series_id FK, content_id FK,   -- content_type = 'blog_post'
  position INT NOT NULL,
  PRIMARY KEY (series_id, content_id)
)
```

Series route: `/blog/series/[slug]` — lists posts in `position` order, with next/previous navigation at the end of each post in the series. A post can belong to multiple series. Series status `active`/`archived` — archived series still render their posts normally, just don't appear in the "active series" sidebar/discovery surface.

## See Also
Section 14 (scheduling reuse), ADR-029.
