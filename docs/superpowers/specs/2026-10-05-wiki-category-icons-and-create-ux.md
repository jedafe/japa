# Wiki category icons, template-aware create, structured rollup UI

Design for three admin/main wiki improvements. Approved by owner (2026-10-05):
template pick locks title AND slug.

## 1. Category icons

- `wiki_type_categories.icon text` (nullable) — migration `000753`.
- Thread `icon` through `ListWikiTypeCategories`, `GetWikiCategoryForSpace`,
  `ListWikiCategoriesBySpaceSlug`, `ListAllWikiCategories`,
  `CreateWikiTypeCategory`, `UpdateWikiTypeCategory` (COALESCE nullable arg).
- `WikiCategory.Icon`, `WikiCategoryDTO.Icon`, create/update request fields.
- Public `GET /wiki/categories` emits `icon` per item.
- Admin category form gains a text `icon` field (lucide kebab name, same
  convention as `wiki_type_configs.icon`).
- Main app: category tabs/cards resolve `resolveIcon(item.icon)` when set,
  else the existing slug map fallback.

## 2. Template-aware member create

`/wiki/{space}/create` keeps two paths under the same form:

- Category select first (unchanged).
- When the chosen category's `page_templates` (already returned by
  `/wiki/categories?space=`) is non-empty, a "Page" select appears listing
  each template plus "Custom page…".
- Template pick: slug input locked to `template.slug`, title input locked to
  `template.label` (`{name}` is substituted server-side in the label). Submit
  creates the page bound to the template — `CreatePageWithRevision` already
  matches `(category, slug)` → `page_template_id`, no backend change.
- "Custom page…": today's free-form title + slug.
- Categories without templates behave exactly as today.

Per-space semantics: a template is type-wide; a created page materializes
only on the space it was submitted to. Other spaces keep lazy stubs.

## 3. Structured rollup editor

Replace `rollup_json` textarea with a `formSnippet` (`CategoryForm`) on the
categories `EntityPage` — same pattern as the types tab's `TypeForm`:

- Fields seeding keys stay flat: `rollup_enabled`, `rollup_mode`,
  `rollup_metrics` (string[]), `rollup_levels` (string[]),
  `rollup_section_limit` (number) — flattened from `rollup` on load rows.
- UI: Enabled checkbox → when on: Mode select (Aggregate field values /
  Directory child spaces); Metrics checkboxes min/avg/max (aggregate only);
  Levels checkboxes = the selected type's declared child wiki types
  (empty = all declared children); Section limit number (aggregate only).
- Submit assembles the same JSONB shape `validateRollupInput` accepts;
  disabled → `rollup: null` clears.
- The "directory"/"on" table badge stays.

## Spec update

`.jeos/sections/80-destination-wiki.md`: category `icon` field, template
picker in member create, structured rollup editor.
