# Section 23 — Main Site Frontend Foundation

**Date:** 2026-06-27
**Phase:** 7
**Status:** Complete

## Overview

Section 23 establishes the main site's frontend foundation for JapaConnect: the server-side hooks that hydrate user data, the navigation shell (Header, MobileNav, Footer, SideNav, SpaceSwitcher), shared UI primitives (ErrorState, EmptyState, LoadingSkeleton), a NotificationCentre component, a theme store with dark mode persistence, and extended CSS custom properties for theming. The backend adds the `internal/users` package with profile and preferences endpoints.

## Components

### Backend (Go)

1. **internal/users/types.go** — Profile and preferences types, request/response structs
2. **internal/users/store.go** — Profile and preferences CRUD against `users` and `user_preferences` tables
3. **internal/users/service.go** — Service wrapping store with GetMyProfile, UpdateMyProfile, GetMyPreferences, UpdateMyPreferences
4. **internal/users/handlers.go** — 4 HTTP handlers: GetMyProfile, UpdateMyProfile, GetMyPreferences, UpdateMyPreferences
5. **internal/users/users_test.go** — Tests for store, service, and handler layers

### Migration

- **000030_user_preferences.sql** — `user_preferences` table with `dark_mode` (default `system`), `language` (default `en-NG`), unique constraint on `(tenant_id, user_id)`

### Frontend (SvelteKit / Svelte 5)

1. **hooks.server.ts** — Reads `jc_token` cookie, fetches `/users/me`, populates `event.locals.user` with id, email, username, displayName, avatarUrl, profileType, status
2. **+layout.server.ts** — Server load function fetching feature flags and unread notification count
3. **+layout.svelte** — Root layout composing Header, SideNav, MobileNav, Footer; initializes theme store

#### Navigation Components
4. **src/lib/components/nav/Header.svelte** — Top bar with logo, search, notification bell, user avatar, space switcher trigger
5. **src/lib/components/nav/MobileNav.svelte** — Bottom navigation bar for mobile viewports
6. **src/lib/components/nav/Footer.svelte** — Site footer with links, copyright
7. **src/lib/components/nav/SideNav.svelte** — Slide-over navigation panel (collapses on mobile)
8. **src/lib/components/nav/SpaceSwitcher.svelte** — Dropdown to switch between geo-spaces

#### Shared UI Components
9. **src/lib/components/ui/ErrorState.svelte** — Reusable error display with retry action
10. **src/lib/components/ui/EmptyState.svelte** — Empty state placeholder with icon and message
11. **src/lib/components/ui/LoadingSkeleton.svelte** — Skeleton loading indicator

#### Notifications
12. **src/lib/components/notifications/NotificationCentre.svelte** — Dropdown notification panel with unread/read states

#### Theme System
13. **src/lib/stores/theme.ts** — `getTheme()` factory returning `current`, `set(theme)`, and `init(serverTheme)` methods; persists preference to `/users/me/preferences`

#### API Helpers
14. **src/lib/server/api.ts** — `apiFetch<T>()` wrapper for server-side API calls with error handling

## Files Created

### Backend
1. `internal/users/types.go`
2. `internal/users/store.go`
3. `internal/users/service.go`
4. `internal/users/handlers.go`
5. `internal/users/users_test.go`
6. `migrations/000030_user_preferences.sql`

### Frontend
7. `apps/main/src/hooks.server.ts`
8. `apps/main/src/routes/+layout.server.ts`
9. `apps/main/src/routes/+layout.svelte`
10. `apps/main/src/lib/components/nav/Header.svelte`
11. `apps/main/src/lib/components/nav/MobileNav.svelte`
12. `apps/main/src/lib/components/nav/Footer.svelte`
13. `apps/main/src/lib/components/nav/SideNav.svelte`
14. `apps/main/src/lib/components/nav/SpaceSwitcher.svelte`
15. `apps/main/src/lib/components/nav/index.ts`
16. `apps/main/src/lib/components/ui/ErrorState.svelte`
17. `apps/main/src/lib/components/ui/EmptyState.svelte`
18. `apps/main/src/lib/components/ui/LoadingSkeleton.svelte`
19. `apps/main/src/lib/components/ui/index.ts`
20. `apps/main/src/lib/components/notifications/NotificationCentre.svelte`
21. `apps/main/src/lib/components/notifications/index.ts`
22. `apps/main/src/lib/stores/theme.ts`
23. `apps/main/src/lib/server/api.ts`

## Files Modified

1. `internal/server/server.go` — Added users import, service/handler constructors, 4 route registrations
2. `internal/db/gen/models.go` — Regenerated for `user_preferences` table
3. `internal/db/gen/db.go` — Regenerated for new query methods

## API Routes

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/users/me` | Required | Get authenticated user's profile |
| PATCH | `/users/me` | Required | Update authenticated user's profile |
| GET | `/users/me/preferences` | Required | Get user preferences |
| PUT | `/users/me/preferences` | Required | Update user preferences |

## Key Decisions

1. **Cookie-based SSR hydration** — `hooks.server.ts` reads the `jc_token` cookie and fetches user data server-side. This avoids client-side waterfalls and ensures the navigation shell renders with user context on first paint.

2. **Theme store as factory function** — `getTheme()` returns an object with reactive `current` getter, `set()` for mutations, and `init()` for server-side hydration. This avoids module-level side effects and works with Svelte 5 runes.

3. **Dark mode via CSS class toggling** — The `dark` class on `<html>` toggles CSS custom properties. No Tailwind `darkMode: 'class'` configuration needed since Tailwind v4 uses CSS-native `@media (prefers-color-scheme)` or custom properties.

4. **SideNav as slide-over on mobile** — Collapses to a hamburger toggle on mobile, appears as a fixed sidebar on desktop. Avoids dedicated mobile layout duplication.

5. **SpaceSwitcher in Header** — Quick-switching between geo-spaces from the header rather than a separate settings page.

## Verification

### Backend
- `go build ./...` — Clean build
- `go vet ./...` — No issues
- `go test ./internal/users/...` — All tests pass

### Frontend
- `bun run check` — Clean (main app)
- `bun run build` — Build succeeds

## Known Deferrals

- Real-time WebSocket delivery for notification count updates (Section 10 dependency)
- Full search UI in header (Meilisearch integration pending)
- NotificationCentre filtering by type (requires notification category taxonomy)
- Profile avatar upload (uses existing media system, no new handler needed)
- SpaceSwitcher API integration (geo-space list endpoint already exists)

## Next Steps

Section 09 (Onboarding Flow) builds on this foundation with a 5-step wizard and product tour component.
