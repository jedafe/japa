# Section 09 — Onboarding Flow

**Status:** ✅ Locked

## Decided
Five-step flow:
1. Welcome & Core Identity (mandatory) — name, japa status, current location.
2. Destination & Goals (mandatory, with an "I'm not sure yet" escape valve) — destination, primary goal vertical.
3. Interests/Tags (admin-configurable minimum) — shared with the content tagging taxonomy, not a separate onboarding-only set.
4. Recommended Spaces & Connections (admin-configurable minimums for Spaces/Users/Organizations to follow) — tiered deterministic algorithm: admin-pinned promotions first, then exact geo match, then interest-tag overlap, then reputation-based fallback fill. Zero AI tokens, consistent with Search's matching philosophy.
5. First Action Nudge (soft, skippable).

Skip semantics: per-step skip exists, but skipping a step with a minimum greater than zero auto-fulfills that minimum using the same ranked algorithm rather than bypassing the requirement — the user always ends up meeting the platform's baseline density target, they just choose between manual curation or accepting defaults.

## Product Tour (restored post-audit)

A lightweight contextual walkthrough (Driver.js or equivalent) triggers immediately after the five-step wizard completes, highlighting the primary nav, the Space switcher, and the post-creation entry point — distinct from the wizard itself, which collects data, while the tour orients the user inside the actual UI they're about to use. Dismissible at any point, never re-triggered automatically; a "Replay tour" link lives in profile settings for users who want to see it again.

## See Also
Reuses Section 13's Meilisearch indexes for ranking. Admin Panel needs an Onboarding Settings surface for the minimum-count fields and the promoted-entities picker. ADR-029.
