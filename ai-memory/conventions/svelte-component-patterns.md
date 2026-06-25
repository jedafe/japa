# Convention: Svelte Component Patterns

- Shared, locked components (Entity Card, Feed Card, Comparison Matrix, Ad Slot, etc. — see the full catalog in AGENTS.md / project memory) live in a shared component library imported by all three frontend apps, not duplicated per app.
- Loading skeletons and empty states are visually distinct components — never use the same component for both (see Section 23).
- Components that render user-generated rich text always go through the shared TipTapRenderer, never re-implement ProseMirror-JSON-to-HTML rendering locally.
- Components reading translated strings use the runtime i18n resolver, never a hardcoded English string, even for v1's English-only default — string externalization happens at authoring time, not retrofitted later (see ADR-013).
