# Skill: Meilisearch Sync Patterns

## Two-tier sync — don't treat all field updates the same
Content changes (create/edit/delete) trigger immediate re-index via `search_index_sync` (default tier). Engagement score changes (views, reactions) do NOT trigger immediate re-index — they batch into a periodic partial-document update (every ~5 min) using Meilisearch's partial update support. Re-indexing the whole document on every view event will flood the queue for no benefit.

## Index-only-what's-public
Only `published` status content gets indexed. Drafts and removed/flagged content never enter the index — don't filter them out at query time, exclude them at index time.

## Privacy-respecting user indexing
Users who've opted out of profile discoverability are excluded from the users index entirely, not just hidden in UI. Check the visibility flag before indexing, not after.

## Disaster recovery
Meilisearch has no separate backup requirement — it's a derived index. Full restore procedure: restore Postgres, then re-run the sync indexer against the restored data.
