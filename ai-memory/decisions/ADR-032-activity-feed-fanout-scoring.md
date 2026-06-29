# ADR-032: Activity Feed — Hybrid Fan-Out & Content Scoring

**Status:** Accepted (updated session 5 — multi-space fan-out, homepage routing, sticky filter bar)

**Context:** The platform needs a personalised activity feed (Primary timeline) that scales from launch (low volume) to large Spaces (5,000+ members) without a complete rewrite. Four sort modes were requested. A post can now be assigned to multiple spaces simultaneously (content_spaces table, Section 14), requiring the fan-out model to handle per-space dispatch rather than a single-space dispatch.

**Decision:** Hybrid fan-out: push to `feeds` table for Spaces with ≤ 5,000 members (handled by Asynq tasks), pull at read-time for larger Spaces. **Fan-out fires separately for each space a post is assigned to** (one `content:fan_out_space` Asynq task per row in `content_spaces`), plus one `content:fan_out_author_followers` task for the author's followers. Bubbling (`content:fan_out_bubble`) fires for city and custom spaces, fanning out to parent primary spaces up to `spaces.bubble_depth` levels.

**Homepage routing:** Authenticated users are served the `primary` timeline at the root path. Unauthenticated users are served the `explore` timeline. This is resolved at the SvelteKit server load function — no client-side redirect. This replaces the earlier ambiguous "explore for guests + logged-in" framing; logged-in users always start on their personalised primary feed.

**Sticky sort filter:** The four-mode sort selector (Relevant, Latest, Popular, Rising) is rendered with `position: sticky` on all timeline views, persisting below the fixed nav header on scroll. Active mode is per-tab session state, not a server-side preference.

**Threshold** stored in `system_settings` as `feed.fanout_threshold` (default 5000, tunable without redeploy). Rising sort uses Hacker News gravity formula: `velocity = engagement / (hours_old + 2)^1.8` — gravity constant in `system_settings`. Relevant sort uses pgvector cosine similarity between `content_scores.relevance_embedding` and `user_profiles.interest_embedding` (both `vector(1536)`).

**Rationale:** The multi-space fan-out naturally multiplies write load per post, but the same hybrid threshold (push for small, pull for large) is applied per space independently, so the load increase is bounded. The author-followers fan-out is kept as a separate task to maintain clean separation between social graph fan-out and space fan-out. Homepage routing at the server level avoids flash-of-wrong-content on initial load, which is especially important on mobile.

**Consequences:** The `feeds` table needs regular cleanup (`maintenance:prune_old_feeds` task, low tier, weekly). Posts assigned to many spaces generate proportionally more fan-out tasks. The `UNIQUE(user_id, content_id)` constraint on `feeds` prevents a user from receiving duplicate feed rows even if they follow both the author and a space the post is in — the first write wins.
