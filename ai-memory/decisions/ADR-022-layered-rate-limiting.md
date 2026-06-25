# ADR-022: Layered Rate Limiting & Abuse Protection

**Status:** Accepted

**Context:** Identified as a gap during the V1.0.0 production-readiness audit — no rate limiting or abuse protection existed anywhere in the original design, despite the platform exposing login, AI Gateway calls (real per-call cost), search, and password-reset endpoints publicly.

**Decision:** Two layers, not one:

1. **Nginx, coarse first line of defense.** Per-IP request rate limiting at the reverse proxy (`limit_req_zone`), catching basic flood/scraping behavior before it reaches the application at all.
2. **Valkey-backed application-level limiting.** Per-user and per-feature limits that need account context Nginx can't see — stricter on expensive paths (AI Gateway calls, password reset requests, search) than on simple reads.

Login specifically gets dedicated brute-force protection: a cap on failed attempts per account/IP within a rolling window, with exponential backoff on the lockout duration. The AI Gateway specifically gets a **hard per-user quota**, not just the admin-facing budget alert from Section 21 — a single user or bot account shouldn't be able to run up real provider costs just because no admin happened to be watching the dashboard at that moment.

**Rationale:** Nginx-level limiting alone can't distinguish between users or apply different limits per feature; application-level limiting alone misses obvious flood patterns Nginx catches for free. Doing both costs little and closes both gaps. The AI Gateway quota is a hard rule rather than just an alert specifically because the failure mode (a single account silently running up provider bills) is asymmetric — a budget alert tells you after the cost is already incurred, a hard quota prevents it.

**Consequences:** Every new endpoint added to the platform should have an explicit rate-limit tier assigned at design time (matching the convention: expensive/sensitive endpoints get tighter limits), not left to the application-default rate automatically. See the rate limiter pattern in `ai-memory/skills/abuse-protection-patterns.md`.
