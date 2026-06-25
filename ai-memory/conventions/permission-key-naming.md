# Convention: Permission Key Naming

- Format: `{domain}.{subdomain?}.{action}` — lowercase, dot-separated.
- Examples already in use: `content.page_builder.create`, `blog.publish`, `blog.edit_own`, `blog.edit_any`, `ai.editor.assist`, `moderation.appeals.review`.
- Action verbs are consistent across domains where the same shape of action recurs: `create`, `edit_own`, `edit_any`, `publish`, `review`, `delete`, `manage`.
- Scoped (per-entity) access does not get a permission-node-per-entity explosion — use a join table instead (see `ai-memory/skills/rbac-permission-node-pattern.md`).
