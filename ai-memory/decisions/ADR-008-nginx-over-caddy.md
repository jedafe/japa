# ADR-008: Nginx Over Caddy

**Status:** Accepted

**Context:** Initial recommendation was Caddy; this was challenged and changed.

**Decision:** Nginx as the reverse proxy.

**Rationale:** More mature, more battle-tested at scale, broader community troubleshooting resources available than Caddy for a solo developer who'll be debugging production issues alone.

**Consequences:** Slightly more verbose config than Caddy's, accepted as the tradeoff for maturity.
