# JapaConnect Monorepo

## Package management

- **Bun only, at the repo root.** Run `bun install` in the repo root — never inside `apps/*` or `packages/*`. A standalone install creates a per-app lockfile + isolated `node_modules` that drifts from the shared store.
- Per-app `bun.lock` files must not exist (root `bun.lock` is the only lockfile).
- **Do not mix in pnpm/npm.** Every app and package must resolve `svelte` to the single `.bun` store copy — a second `svelte/internal/client` in a bundle crashes hydration with `TypeError: Cannot read properties of undefined (reading 'call')` in `get_next_sibling`. All five `apps/*/vite.config.ts` carry `resolve.dedupe: ['svelte']` as a guard; keep it.

## Dev servers

| App | Port |
|---|---|
| apps/main | 3000 |
| apps/admin | 3001 |
| apps/control | 3002 |
| apps/academy | 3003 |
| apps/docs | 3004 |

Start per app: `cd apps/<app> && bunx vite dev --port <port>`. Backend API target for dev proxies: `http://localhost:8080`.
