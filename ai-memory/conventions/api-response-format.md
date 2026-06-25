# Convention: API Response Format

- Success: `{ "data": {...} }` for single resources, `{ "data": [...], "meta": { "next_cursor": "..." } }` for cursor-paginated lists.
- Error: `{ "error": { "code": "string_code", "message": "human readable" } }` — `code` is machine-stable, `message` is for humans/logs, never the other way around.
- Cursor pagination only — no offset/page-number pagination anywhere (see `ai-memory/skills/sqlc-query-patterns.md`).
- Timestamps are always ISO8601 UTC.
- Polymorphic API responses (e.g. a feed mixing content types) include the discriminator field (`content_type`, `purpose_type`, etc.) at the top level of each item, not nested.
