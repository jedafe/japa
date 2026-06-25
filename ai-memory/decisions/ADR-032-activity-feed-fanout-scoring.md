# ADR-032: Activity Feed — Hybrid Fan-Out & Content Scoring

**Status:** Accepted

**Context:** The platform needs a personalised activity feed (Primary timeline) that scales from launch (low volume) to large Spaces (5,000+ members) without a complete rewrite. Four sort modes were requested: Latest, Top, Rising, and Relevant (personalised). The Rising sort needs to surface high-velocity content without privileging older high-score posts permanently.

**Decision:** Hybrid fan-out: push to `feeds` table for Spaces with ≤ 5,000 members (handled by `content:fan_out_to_feeds` Asynq task, default tier), pull at read-time for larger Spaces. Threshold stored in `system_settings` as `feed.fanout_threshold` (default 5000, tunable without redeploy). Rising sort uses Hacker News gravity formula: `velocity = engagement / (hours_old + 2)^1.8` — gravity constant `1.8` is also in `system_settings` so it can be adjusted without a code deploy. Relevant sort uses pgvector cosine similarity between `content_scores.relevance_embedding` and `user_profiles.interest_embedding` (both `vector(1536)`, populated by `ai:generate_content_embedding` Asynq task post-publish). Explore timeline (guests + logged-out) is pure pull — no per-user `feeds` rows generated.

**Rationale:** Pure push-to-all (fan-out-on-write) would overwhelm the DB for large Spaces with thousands of members. Pure pull (fan-out-on-read) is slow for small, active feeds. The hybrid threshold is the industry-standard pattern (Twitter used ~1,000 followers; we set 5,000 to err toward push for longer). The HN gravity formula is time-tested for forum/news velocity ranking with a single tunable constant.

**Consequences:** The `feeds` table needs regular cleanup (`maintenance:prune_old_feeds` task, low tier, weekly) to prevent unbounded growth. Posts by users with follower_count > fanout_threshold are pull-only by definition — they don't get push fan-out. The `relevance_embedding` for new posts takes minutes to generate (async); Relevant sort falls back to Latest until the embedding exists.
