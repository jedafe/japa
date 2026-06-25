# ADR-043: Bot & Crawler Management

**Status:** Accepted

**Context:** The platform needs to: (1) control which crawlers can access content (robots.txt), (2) serve structured content to AI crawlers (llms.txt), (3) exclude low-quality pages from sitemaps, and (4) let admin manage these settings without code deploys.

**Decision:** `known_bots` table (pre-seeded with major crawlers: Googlebot, GPTBot, ClaudeBot, PerplexityBot, AhrefsBot, etc.) with admin-editable `policy` (allow/block/limit). `robots.txt` generated dynamically from the bot registry, cached in Valkey TTL 24h, invalidated on bot policy change. `llms.txt` (brief) and `llms-full.txt` (detailed) generated hourly by `maintenance:regenerate_llms_txt` task per llmstxt.org spec. Sitemap quality gates: `IsSpaceIndexable()` checks `member_count` and `post_count` against admin-configurable thresholds; `IsContentIndexable()` checks word count threshold and UserGroup-level indexing permission — enabling dev.to-style quality gating where new/unverified member content is excluded until they meet reputation thresholds.

**Rationale:** Static `robots.txt` cannot be updated without a code deploy. Admin-managed bot policy (via a DB table + Valkey cache) is flexible and immediate. llms.txt is an emerging standard for AI crawler guidance — building it into the platform from day one positions JapaConnect well for AI-era content discovery. Sitemap quality gates prevent empty geo space pages (which would harm SEO via thin content signals) from being submitted to Google.

**Consequences:** The bot policy cache must be invalidated when `known_bots.policy` changes (handled by the Admin Panel save handler). llms-full.txt can become large for platforms with extensive content — the hourly regeneration task must handle timeouts gracefully.
