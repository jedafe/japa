# ADR-015: Multi-Provider Email Abstraction With Category Routing

**Status:** Accepted

**Context:** Needed transactional email (password resets, notifications) and, looking ahead to the v2 Newsletter feature, bulk/marketing email — different providers are positioned for each.

**Decision:** Provider abstraction over Resend, Postmark, AWS SES, SendGrid, and SendPulse, plus an admin-addable generic SMTP implementation for any custom server. Providers are assigned a category (transactional vs. marketing) rather than one provider handling everything. Circuit-breaker fallback within a category mirrors the AI Gateway pattern exactly (ADR-010).

**Rationale:** SendPulse is positioned as a marketing/bulk-send platform, not a transactional one — forcing all email categories through a single provider ignores that providers have genuine differentiated strengths. Reusing the AI Gateway's exact circuit-breaker shape avoided inventing a second pattern for the same underlying problem (a critical external dependency that needs graceful degradation).

**Consequences:** Email credentials are stored encrypted at rest, same treatment as payment provider secrets.
