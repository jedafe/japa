# ADR-011: Page Builder Layered Over TipTap, Not Parallel To It

**Status:** Accepted

**Context:** Page Builder needed a way to handle free-form text within composed blocks (Hero, Rich Text blocks). Building a second text-editing system was on the table implicitly until this was clarified.

**Decision:** Page Builder is purely a layout/composition layer built on dnd-kit. Its Rich Text block type embeds a TipTap instance for actual prose content rather than reimplementing text editing.

**Rationale:** Maintaining two rich-text editing systems doubles the surface area for bugs, extension maintenance, and rendering edge cases for no real benefit — TipTap already does this job.

**Consequences:** Page Builder block storage and ordinary content storage both live in `field_values` JSONB, keeping the two systems' data shapes conceptually aligned even though they serve different purposes.
