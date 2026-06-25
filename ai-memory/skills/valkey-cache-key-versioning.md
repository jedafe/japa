# Skill: Valkey Cache Key Versioning

## Version-in-key, not explicit invalidation
For rendered content (TipTap HTML output), key on `jc:content_render:{content_id}:{updated_at_timestamp}` rather than a bare content_id with an explicit DEL on edit. An edit naturally produces a new key — one cache miss, one re-render — and the old key just ages out via TTL (24-48h is the default starting point). This avoids needing explicit cache-busting logic scattered across every write path.

## Accepted tradeoff: resolved labels can lag
Cached rendered HTML bakes in resolved mention/interlink display names at render time. If a mentioned user later renames, the cache shows the old name until that specific cache entry expires. This is an accepted staleness window, not a bug — chasing perfect consistency here isn't worth the complexity for a community platform.

## Namespace discipline
Every Valkey key in this codebase is prefixed `jc:`. Sub-namespace by domain: `jc:content_render:`, `jc:ratelimit:`, `jc:i18n:`, etc. — makes `SCAN`-based debugging and selective flushing tractable.
