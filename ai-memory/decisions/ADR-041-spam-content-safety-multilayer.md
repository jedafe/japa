# ADR-041: Spam & Content Safety — Multi-Layer Defence

**Status:** Accepted

**Context:** A community platform serving a Nigerian diaspora audience at global scale is a target for spam, IP farming, duplicate content, and NSFW image uploads. A single defence mechanism (e.g. CAPTCHA only) would be insufficient.

**Decision:** Ten layers applied in sequence, each independently bypassable by a determined actor but collectively providing strong deterrence: (1) Cloudflare Turnstile at registration/forms, (2) honeypot fields, (3) new-user link restriction (configurable first-N-posts), (4) duplicate content SHA-256 hash check (UNIQUE index on `contents.content_hash`), (5) IP blocking via `ip_blocks` CIDR table with GiST inet_ops index, (6) user-to-user blocking with mutual visibility suppression at query level, (7) censored word filter (server-side, Valkey-cached patterns), (8) NSFW.js client-side pre-check for images, (9) optional async AI moderation scan, (10) manual moderation queue. The `user_blocks` mutual invisibility is enforced via a `NOT EXISTS` subquery injected by an `ExcludeBlockedUsers` helper applied to every content list query for authenticated users.

**Rationale:** No single layer is foolproof; defence in depth is the correct posture. Layers 1–4 are zero-cost at runtime (CAPTCHA is handled by Cloudflare, hash check is a DB index lookup). IP blocking with CIDR GiST index supports entire subnet blocks efficiently. Valkey-cached censored words avoid a DB hit per post submission. Client-side NSFW.js catches obvious cases before upload; PhotoDNA-style server-side hashing for CSAM is the nuclear-option layer.

**Consequences:** The `ExcludeBlockedUsers` query helper must be applied consistently — a missing application in one endpoint creates a bypass. This is a convention enforced in RULES.md and code review, not a compile-time check.
