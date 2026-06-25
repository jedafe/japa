# ADR-007: Path-Based Routing Within a Surface, Subdomains for Genuinely Distinct Surfaces

**Status:** Accepted

**Context:** Early discussion considered subdomains more broadly before this was corrected to a narrower scope.

**Decision:** Content within the main product surface uses path-based routing. Subdomains (`docs.`, `help.`, `learn.`) are reserved for product surfaces that are actually separate applications/experiences, with cross-subdomain session sharing via `domain=.japaconnect.com` cookies.

**Rationale:** Subdomains carry real cost — separate SEO authority, separate session handling, separate deploy targets in some setups. Reserving them for genuinely distinct surfaces avoids fragmenting the main site's content graph and SEO equity for no benefit.

**Consequences:** Any new feature proposing a subdomain should be checked against "is this actually a separate product surface" before being granted one.
