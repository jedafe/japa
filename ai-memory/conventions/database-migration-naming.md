# Convention: Database Migration Naming

- Format: `{timestamp}_{verb}_{description}.sql` — e.g. `20260301120000_add_locale_to_users.sql`.
- One logical change per migration file. Don't bundle an unrelated index addition into a migration about adding a column.
- Breaking changes (renaming/dropping columns) are split across two migrations minimum, following the two-phase deploy pattern in `RULES.md` — never a single migration that both adds the new shape and drops the old shape in one step.
- Every migration that adds a new table includes `tenant_id` unless there's a specific documented reason it's a global, non-tenant-scoped table.
