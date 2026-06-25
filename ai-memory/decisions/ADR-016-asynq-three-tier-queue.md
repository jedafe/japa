# ADR-016: Asynq Job Queue With Three Priority Tiers

**Status:** Accepted

**Context:** Background work spans wildly different urgency levels — webhook delivery needs to happen within seconds, sitemap regeneration can wait hours.

**Decision:** Three queue tiers (critical, default, low) plus a dead-letter queue with exponential backoff, with worker concurrency configured per tier via environment variables rather than hardcoded.

**Rationale:** A single undifferentiated queue means a backlog of low-priority work (digest emails, sitemap regen) can delay genuinely time-sensitive work (webhook retries, real-time notifications) behind it. Tiering with separate concurrency budgets prevents that.

**Consequences:** Every new background task type added to the system needs an explicit tier assignment as part of its design, not an afterthought.
