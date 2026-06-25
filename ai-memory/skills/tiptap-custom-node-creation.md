# Skill: TipTap Custom Node Creation (Mentions, Tags, Interlinks)

## The pattern: store entity references, never resolved text
Every custom node type (mention, tag, interlink) stores `{ id, entity_type, label }` in its ProseMirror node attrs. `label` is a cached display value for editor UX only — render-time always re-resolves from `id` + `entity_type` against current data, never trusts the stored `label` as authoritative.

## Trigger character mapping (do not mix these up)
- `/` — slash/block commands
- `@` — user mentions only
- `#` — tags only
- `[[` — content/space interlinking only

## Handling a deleted referenced entity at render time
If the ID a node references no longer resolves (deleted content, deleted space), render a "deleted content" placeholder — never a broken link, never an empty string.

## Reuse search indexes for autocomplete
The `@` and `#`/`[[` dropdown autocompletes should query the existing Meilisearch indexes from Section 13, not a separate lookup query. Don't build a second search path for editor autocomplete.
