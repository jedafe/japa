# Skill: sqlc Query Patterns

## UUID handling
Always use `pgtype.UUID`, never `uuid.UUID` directly, in sqlc params and generated structs.

## Named params
Use `@param_name` syntax in queries, not positional `$1` placeholders — keeps generated Go function signatures self-documenting.

## Cursor pagination (use everywhere, not offset pagination)
```sql
SELECT ... FROM contents
WHERE (created_at, id) < (@cursor_created_at, @cursor_id)
ORDER BY created_at DESC, id DESC
LIMIT @limit
```

## JSONB field_values queries
Use `->>'key'` for text extraction, cast explicitly with `::type` — never trust a JSONB value's implicit type.
```sql
field_values->>'destination_country' AS destination_country
WHERE (field_values->>'min_followers')::int >= @min_followers
```

## Polymorphic references (purpose_type/purpose_id, target_type/target_id)
Always pair the discriminator column with the ID column in the same query's WHERE clause — never query target_id alone across a polymorphic relationship, since the same ID value can mean different things depending on target_type.
