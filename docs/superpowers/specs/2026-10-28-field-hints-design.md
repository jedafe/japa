# Field Hints — All Frontend Apps

**Date:** 2026-10-28
**Status:** Approved approach (A — per-app conventions, surgical application)

## Goal

Every form field across every JapaConnect frontend surface gets:

1. **Helper text** — persistent small muted text rendered under the field,
   explaining purpose, format, or constraints.
2. **Placeholder** — a concrete example value inside the input, added where
   one is missing (existing placeholders are kept, not churned).

## Decisions (locked with user)

| Decision | Choice |
|---|---|
| Hint type | Helper text under field **+** placeholder |
| Scope | All web apps, mobile (Flutter), extension, studio-desktop |
| What counts as a field | **Form fields only** — labeled inputs in forms, settings, dialogs |
| Copy storage | i18n keys, `en.json` **only** (fr falls back to English) |
| Copy author | Devin writes contextual copy per field |
| Rollout | Per-app passes — one app completed before starting the next |

## Surfaces

`apps/tools` is an empty shell (contains only a `robots.txt` route); the tools
UI lives inside `apps/main`. Real surfaces, in pass order:

| # | Surface | Field pattern | ~Field count | Strategy |
|---|---|---|---|---|
| 1 | `apps/main` (SvelteKit SSR, port 3000) | Inline `<input>/<textarea>/<select>` | ~670 | i18n keys + muted helper element under each field, wired via `aria-describedby` |
| 2 | `apps/admin` (SvelteKit SPA, port 3001) | `EntityPage`/`EntityForm` via `FieldDef[]`, plus `formSnippet` custom fields | ~580 | Fill existing `FieldDef.help`/`placeholder` props; add helper spans in `formSnippet` fields |
| 3 | `apps/control` (SvelteKit SPA, port 3002) | Inline fields | ~154 | Same pattern as main |
| 4 | `apps/academy` (SSR, port 3003) | Inline fields | ~50 | Same pattern as main |
| 5 | `apps/docs` (SSR, port 3004) | Inline fields | ~32 | Same pattern as main |
| 6 | `apps/mobile` (Flutter) | `TextFormField`/`TextField` | ~12 | `InputDecoration.helperText` + `hintText` |
| 7 | `apps/studio-desktop` (Tauri + Svelte) | Shared `lib/components/prompt/` field components | ~91 | Add `hint` prop centrally to `TextField`, `SelectField`, `NumberField`, `CheckboxGroup` |
| 8 | `apps/extension` (WXT + Svelte) | Inline fields, ~3 files | ~1 | Inline hint |

## Exclusions (no helper text; placeholder only if clearly beneficial)

- Search boxes / command palettes
- Chat/message composers (forum reply, DM threads, JapaBot input)
- DataTable filter inputs and toolbar search
- Hidden inputs, submit buttons, checkboxes that are self-explanatory
- Read-only display values rendered as inputs

Checkboxes and select fields get helper text where a choice needs context;
placeholder is an input-only concept and is skipped for selects/checkboxes.

## Copy conventions

- **Key naming:** `<area>.fields.<name>_hint` and
  `<area>.fields.<name>_placeholder`, where `<area>` matches the app's
  existing key grouping (e.g. `auth.fields.email_hint`,
  `settings.profile.display_name_hint`).
- **Helper text:** one short sentence (≤ ~90 chars) explaining *why*, the
  expected *format*, or a *constraint* — never a restatement of the label.
  Good: "Used for sign-in and receipts — we never share it."
  Bad: "Enter your email."
- **Placeholder:** a concrete example value, not a label repeat.
  Good: `you@example.com`. Bad: `Email`.
- **Apps without i18n** (studio-desktop, extension — verify per app): inline
  English strings matching the same copy style.
- **Accessibility:** helper text is linked to its control with
  `aria-describedby` (`id="{field-id}-hint"`); on error the field references
  both hint and error ids, matching the existing `*-error` convention in
  `apps/main`.

## Per-app pass procedure

Each pass is independent and shippable:

1. **Enumerate** — inventory every form field in the app (route by route /
   component by component); classify: form field vs excluded transient input.
2. **Keys** — add all new `*_hint` / `*_placeholder` keys to `en.json`
   (or that app's locale file).
3. **Render** — apply helper text + placeholder per the surface strategy above.
   For `admin`, set `FieldDef.help`/`placeholder`; for `formSnippet` fields add
   the muted span manually.
4. **Verify** — run that app's typecheck/build (`bun run check`/`build` or
   `flutter analyze` for mobile) and spot-check at least one rendered form.

## Testing

- No new test infrastructure; verification is per-app typecheck + build +
  manual spot-check of a representative form per app.
- Mobile: `flutter analyze` clean on touched files.

## Out of scope

- French translations of hint copy (fr falls back to en).
- Refactoring fields into a shared `packages/ui` `FormField` component
  (deliberately rejected — approach B).
- Helper text on non-form inputs (search, chat, filters).
- Migrating legacy admin pages to `EntityPage` (hints applied in place).
