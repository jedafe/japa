# Studio Desktop — SQLite State Layer + E2E Implementation Plan

> **For agentic workers:** Inline execution. Tasks use checkbox syntax for tracking.

**Goal:** Replace localStorage with a Tauri SQLite state layer, add Playwright static-build e2e smoke tests, and add a Tauri WebDriver e2e harness.

**Architecture:**
- Use `@tauri-apps/plugin-sql` with `tauri-plugin-sql` (`sqlite` feature) and Rust migrations for `projects`, `file_states`, `sync_log`, and `editor_drafts`.
- Create `src/lib/services/db.ts` with a `StorageAdapter` abstraction: SQLite in Tauri, localStorage fallback in browsers/Vitest.
- Keep the `settings.ts` API as a synchronous in-memory cache that is hydrated from SQLite on first load and persisted on save, preserving the existing `loadSettings()` call sites.
- Refactor `src/lib/stores/queue.svelte.ts` and `src/lib/stores/sync-log.svelte.ts` to read/write through `db.ts`.
- Add Playwright against the built Vite static site for route rendering smoke tests.
- Add a Tauri WebDriver harness that attempts to launch the built binary when `tauri-driver` and a native WebDriver server (`WebKitWebDriver`, `safaridriver`, etc.) are present; skips gracefully otherwise.

**Tech stack:** Bun, Svelte 5, SvelteKit, Tauri 2, `tauri-plugin-sql`, Playwright, `selenium-webdriver`.

---

## Global constraints
- Use Bun for all JS/TS tooling (Rule 18).
- Never hardcode data (Rule 16).
- No AI-attribution in commits (Rule 17).
- Align local schema with Section 105 — `projects`, `file_states`, `sync_log`, `editor_drafts`.
- SQLite uses `$1`, `$2` placeholders; arrays stored as JSON text.

---

### Task 1: Add Tauri SQL dependencies
**Files:**
- Modify: `apps/studio-desktop/package.json`
- Modify: `apps/studio-desktop/src-tauri/Cargo.toml`
- Modify: `apps/studio-desktop/src-tauri/tauri.conf.json`
- Modify: `apps/studio-desktop/src-tauri/src/lib.rs`

- [ ] Add `@tauri-apps/plugin-sql` to dependencies.
- [ ] Add `tauri-plugin-sql = { version = "2", features = ["sqlite"] }` to `Cargo.toml`.
- [ ] Register the plugin in `lib.rs` with migrations for `projects`, `file_states`, `sync_log`, `editor_drafts`.
- [ ] Add `plugins.sql.preload = ["sqlite:state.db"]` to `tauri.conf.json`.

---

### Task 2: Implement `src/lib/services/db.ts`
**Files:**
- Create: `apps/studio-desktop/src/lib/services/db.ts`
- Create: `apps/studio-desktop/src/lib/services/db.test.ts`

- [ ] Define a `StorageAdapter` interface and a default `TauriDbAdapter` that uses `Database.load('sqlite:state.db')`.
- [ ] Provide a `LocalStorageAdapter` fallback when `window.__TAURI__` is absent (browser, Vitest, Playwright static build).
- [ ] Export functions: `loadSettings`, `saveSettings`, `loadFileStates`, `saveAllFileStates`, `saveFileState`, `deleteFileState`, `loadSyncLog`, `appendSyncLog`, `clearSyncLog`, `loadEditorDraft`, `saveEditorDraft`, `initDb`, `setStorageAdapter` (for tests).
- [ ] Add unit tests for the localStorage fallback path.

---

### Task 3: Refactor settings persistence
**Files:**
- Modify: `apps/studio-desktop/src/lib/services/settings.ts`
- Modify: `apps/studio-desktop/src/routes/+layout.svelte`
- Modify: `apps/studio-desktop/src/routes/+page.svelte`
- Modify: `apps/studio-desktop/src/routes/settings/+page.svelte`
- Modify: `apps/studio-desktop/src/lib/api/auth.ts`

- [ ] Replace `localStorage` direct access in `settings.ts` with calls to `db.ts`; keep `loadSettings()` synchronous and make `saveSettings()` async.
- [ ] Add `initSettingsDb()` and `ensureSettingsLoaded()` and call them in `+layout.svelte` `onMount`.
- [ ] Update `settings/+page.svelte` `handleSave()` and `+page.svelte` `toggleAutoSync()` to `await saveSettings()`.
- [ ] Update `auth.ts` functions to `await saveSettings()` where it is called.
- [ ] Migrate legacy `localStorage` settings into SQLite on first Tauri run.

---

### Task 4: Refactor queue and sync-log stores
**Files:**
- Modify: `apps/studio-desktop/src/lib/stores/queue.svelte.ts`
- Modify: `apps/studio-desktop/src/lib/stores/sync-log.svelte.ts`
- Modify: `apps/studio-desktop/src/routes/+page.svelte`
- Modify: `apps/studio-desktop/src/routes/editor/[...path]/+page.svelte`
- Modify: `apps/studio-desktop/src/routes/sync/+page.svelte`

- [ ] Update `queue.svelte.ts` to load/save `FileState[]` through `db.ts` (`load`, `setFiles`, `updateFile`, `upsertFile`, `mergeScanned`).
- [ ] Update `sync-log.svelte.ts` to load/append/clear through `db.ts`.
- [ ] Make `load()` and `clear()` async and `await` them in `onMount`/`onclick` handlers.

---

### Task 5: Add Playwright e2e smoke tests
**Files:**
- Modify: `apps/studio-desktop/package.json`
- Create: `apps/studio-desktop/playwright.config.ts`
- Create: `apps/studio-desktop/tests/e2e/smoke.spec.ts`
- Create: `apps/studio-desktop/tests/e2e/fixtures.ts` (optional)

- [ ] Add `@playwright/test` as a dev dependency.
- [ ] Configure Playwright to build the static site and run `vite preview` on port 3010.
- [ ] Add a smoke test that visits `/`, `/prompt`, `/batch`, `/sync`, `/settings`, and `/editor/jc-content/guide/test.md` and checks for expected headings and no console errors.

---

### Task 6: Add Tauri WebDriver e2e harness
**Files:**
- Modify: `apps/studio-desktop/package.json`
- Create: `apps/studio-desktop/tests/e2e-tauri/binary.spec.ts`
- Create: `apps/studio-desktop/tests/e2e-tauri/setup.ts`
- Create: `apps/studio-desktop/scripts/e2e-tauri.ts` (optional)

- [ ] Add `selenium-webdriver` and `@types/selenium-webdriver` as dev dependencies.
- [ ] Write a WebDriver test that starts `tauri-driver` (if available) and checks the main window title and a few nav items.
- [ ] Add `test:e2e:tauri` script that builds the binary and runs the WebDriver tests, skipping if `tauri-driver` or the native WebDriver server is missing.

---

### Task 7: Verification
- [ ] `bun run check` (svelte-check) passes.
- [ ] `bun run test` (Vitest) passes.
- [ ] `bun run build` succeeds.
- [ ] `go build ./...` and `go test ./internal/auth/... ./internal/desktopauth/... ./internal/publicapi/...` pass.
- [ ] `bun run test:e2e` (Playwright) passes.
- [ ] `bun run test:e2e:tauri` is attempted and documented if drivers are missing.
- [ ] Update `apps/studio-desktop/AUDIT-REPORT.md` and Section 105 as needed.
