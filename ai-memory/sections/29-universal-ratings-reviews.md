# Section 29 — Universal Ratings & Reviews

**Status:** ✅ Locked (restored post-audit — see ADR-029)

## Decided

A polymorphic ratings/reviews system, not one bolted onto a single entity type — Organizations, Universities/Mentors (Universal Custom Field Builder entities), Services, and any future rateable entity all share one schema rather than each growing its own review table.

```sql
reviews (
  id, site_id, entity_type, entity_id,   -- polymorphic target
  reviewer_user_id FK,
  rating SMALLINT NOT NULL,              -- 1-5
  title TEXT, body TEXT,
  status TEXT NOT NULL DEFAULT 'published', -- published | flagged | removed
  created_at, updated_at,
  UNIQUE(entity_type, entity_id, reviewer_user_id)  -- one review per user per entity
)
review_aggregates (
  entity_type, entity_id,
  review_count INT NOT NULL DEFAULT 0,
  average_rating NUMERIC(3,2) NOT NULL DEFAULT 0,
  updated_at,
  PRIMARY KEY (entity_type, entity_id)
)
```

`review_aggregates` is a maintained rollup, not computed on every read — recalculated via the `review_aggregate_recalc` task (default tier) on review create/update/status-change, not a live `AVG()` query on the hot path. One review per user per entity, editable in place rather than allowing duplicate reviews — this keeps `average_rating` meaningful and matches how every major review platform (Google, Amazon) behaves.

Reviews reuse the existing Reporting & Appeals system (Section 22) rather than a review-specific moderation flow — a flagged review enters the same unified moderation queue as everything else, and `status = 'removed'` excludes it from `review_aggregates` recalculation.

This is the data source for Section 26's `Review`/`AggregateRating` JSON-LD structured data, which already assumed this system's existence — that reference is now backed by an actual schema rather than a dangling assumption.

## See Also

ADR-029. Section 22 (Reporting & Appeals reuse), Section 26 (structured data consumer).
