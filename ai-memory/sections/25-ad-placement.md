# Section 25 — Ad Placement

**Status:** ✅ Locked

## Decided
`ad_slot_types` registry (feed inline, sidebar, search sponsored, comparison sponsored row, forum inline). Ad rendering shows nothing — never an empty placeholder — when no eligible campaign matches; frequency capping per user/day. Batched impression/click ingestion via `ad_event_ingest` (low tier), never synchronous writes on every view. Admin-only campaign management in v1, explicitly the foundation for v2 self-serve. Organization boosts (Section 17) run through this same campaign infrastructure, targeting a featured-placement slot, rather than a separate boost-specific system.

## V2 Note
Self-serve Japa Ads uses flat-rate pricing, not CPM/CPC — see ADR-018.
