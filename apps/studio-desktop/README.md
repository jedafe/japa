# Japa Content Studio Desktop

Tauri + SvelteKit 5 desktop client for JapaConnect content operations.

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

## Build

```bash
bun run tauri:build
```

## Project layout

- `src/` — SvelteKit frontend
- `src-tauri/` — Tauri Rust shell
- `src/lib/api/client.ts` — JapaConnect public API client
- `src/lib/stores/project.svelte.ts` — reactive project state

## Ref

- `sections/105-japa-content-studio-desktop.md`
- `adr/ADR-115-japa-content-studio-desktop.md`
