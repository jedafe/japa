# Section 16 — Page Builder

**Status:** ✅ Locked

## Decided
dnd-kit foundation, layered over TipTap rather than a parallel editor — the Rich Text block type embeds a TipTap instance. Block catalog: Hero, Rich Text, Image/Gallery, CTA/Button, Stats, FAQ/Accordion, Testimonial/Quote, Comparison Embed, Form Embed, Spacer/Divider, Dynamic Listing (data-source-driven, for listing-page templates). Storage: ordered block array `{ block_id, block_type, order, props }` inside `field_values` JSONB, with a page-level `schema_version`. Access gated by `content.page_builder.create` through dynamic RBAC, defaulting to `super_admin` only at launch but extensible without code changes.

## See Also
ADR-011, ADR-003.
