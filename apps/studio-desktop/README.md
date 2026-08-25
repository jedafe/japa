# Japa Content Studio Desktop

Tauri + SvelteKit 5 desktop client for JapaConnect content operations.

## Setup

1. Get a JapaConnect service account API key with these permissions:
   - `content.create`
   - `content.edit_any`
   - `content.assign_author`
   - `content.publish`
   - `persona.view`
   - `studio.view` or `space.view`
2. Open **Settings** and set:
   - JapaConnect base URL
   - API key
   - Project root (a folder that will contain `.claude/CLAUDE.md` and `jc-content/`)
3. (Optional) Configure **Project Defaults** so the prompt builder and new posts are pre-filled.

## Development

```bash
# Install JS deps
cd apps/studio-desktop
bun install

# Run Vite dev server (port 3010)
bun run dev

# Run Tauri dev shell (opens desktop window)
bun run tauri:dev
```

## Build and package

```bash
# Vite/SvelteKit production build
bun run build

# Build the Tauri desktop bundle
bun run tauri:build
```

The Tauri bundle produces platform packages in `src-tauri/target/release/bundle/`.

## Code signing

Tauri can sign update bundles with `TAURI_SIGNING_PRIVATE_KEY` and `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`. Platform-level signed installers (macOS notarization, Windows EV certificate, Linux trusted GPG key) require certificates and are not included in this repo. Configure them in `src-tauri/tauri.conf.json` or your CI environment before distributing.

## Workflow

1. **Generate** — Use the **Prompt Builder** or **Batch Prompt** page to create a Claude prompt. Run it in Claude Code to produce `jc-content/[content-type]/[slug].md` files.
2. **Review** — The **Content Queue** lists all local files with their sync state. Click a row to edit the frontmatter, Markdown body, or preview the rendered output.
3. **Publish** — Click **Publish** to create or update JapaConnect content, reassign authorship to the selected persona, and sync tags. Auto-sync can publish changes as soon as Claude writes them.

## Project layout

- `src/` — SvelteKit frontend
- `src-tauri/` — Tauri Rust shell
- `src/lib/services/` — API client, file scanner, publisher, markdown helpers, settings, templates, batch parser
- `src/lib/stores/` — reactive local state (queue, editor, prompt, sync log)
- `src/routes/` — pages (queue, editor, prompt, batch, sync log, settings)

## Ref

- `sections/105-japa-content-studio-desktop.md`
- `adr/ADR-115-japa-content-studio-desktop.md`
