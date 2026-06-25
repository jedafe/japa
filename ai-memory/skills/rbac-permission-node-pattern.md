# Skill: RBAC Permission Node Pattern

## Never hardcode a role check
Wrong: `if user.Role == "editor" { ... }`
Right: `if rbac.HasPermission(user, "blog.publish") { ... }`

## Naming convention
`{domain}.{action}` — e.g. `content.page_builder.create`, `blog.publish`, `ai.editor.assist`, `moderation.appeals.review`. See `ai-memory/conventions/permission-key-naming.md` for the full convention.

## Scoped permissions (per-entity, not just global)
Some permissions need to be scoped to a specific entity instance, not granted globally — e.g. a User Group managing only the Spanish language pack, not all language packs. Pattern: a dedicated join table (`language_pack_managers`: language_code, user_group_id) rather than trying to force per-entity scoping into the generic global permission-node system. Use this same join-table pattern for any future "this group manages this specific X" requirement.

## New feature checklist
Every new feature with any access control needs its permission node(s) defined at design time, registered as assignable in the dynamic RBAC admin UI, and never defaulted to "everyone" or "super_admin only" without an explicit reason.
