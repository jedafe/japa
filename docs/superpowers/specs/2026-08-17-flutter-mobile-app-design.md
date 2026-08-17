# Flutter Mobile App v2.1 — Design Spec

**Date:** 2026-08-17
**Status:** Approved (pending user spec review)
**Source idea:** JEOS §77, ADR-067, ADR-074, ADR-086, ADR-113
**Scope:** Full v2.1 native iOS + Android Flutter client for JapaConnect. This is a design document only; implementation follows the separate implementation plan.

---

## 1. Overview

The JapaConnect mobile app is a thin `/api/v1` client built in Flutter (Dart) for iOS and Android. It reuses the existing Go backend and public API surface defined in `conventions/api-routes-catalog.md`. The app ships as a v2.1 fast-follow after the v2.0 web milestone, per ADR-067 and ROADMAP-V2.

Two new repository artifacts are created:

- `packages/japaconnect_core` — shared Dart package consumed by the main app and any future standalone tool apps.
- `apps/mobile` — main JapaConnect Flutter app.

The mobile surface is not a new backend; the only net-new backend routes are the four device-token/push routes in ADR-074, which are implemented in Backend Phase 47.

### v2.1 scope

- **In scope:** auth/onboarding, home feed, discover, blog, guides, comparisons, forum, JapaBot chat, spaces, follow/social, notifications, DMs, Japa University (courses + lessons), user profile, settings, wallet, bookmarks, badges/challenges/leaderboard, jobs finder, and the five v2 identity tools (visa checklist, readiness, cost of living, scholarships, remittance).
- **Out of scope:** Admin Panel, Control Panel, AI Content Studio, Persona management, Creator/Instructor dashboard.

---

## 2. Decisions locked during brainstorming

| Decision | Choice |
|---|---|
| Framework | Flutter (Dart), one codebase for iOS + Android, ADR-067 unchanged. |
| State management | `flutter_riverpod` — `AsyncNotifier` / `FutureProvider` / `StateProvider`. No `setState` soup in feature screens. |
| Navigation | `go_router` with deep links and shell routing. |
| HTTP | Typed `japaconnect_core` API client only; no raw `http` or `dio` in feature code. |
| Shared package | `packages/japaconnect_core` holds API client, auth, models, widgets, design tokens. |
| API client | Generated from the backend OpenAPI spec (`/docs/openapi.json`, §47) using the `openapi_generator` Dart package with the `dart` generator; generated code lives in `lib/api/generated/` and is checked in. A pinned spec is kept at `packages/japaconnect_core/openapi/openapi.json`. Wrapped by `JcApiClient` for base URL, auth header, `X-Device-Id` injection, and 401 refresh. |
| Auth | Device-scoped JWT; access token in memory; refresh token + device ID in `flutter_secure_storage`; `X-Device-Id` header; TOTP prompt; CSRF-exempt Bearer auth. |
| Push | FCM (Android) and APNs (iOS) via `firebase_messaging`; device token registration via `POST /api/v1/me/devices`. |
| Real-time | WebSocket hub (§10) for in-app notifications and feed updates. |
| Offline | In-memory Riverpod cache; no SQLite/Hive database; optimistic UI for reactions/follows/bookmarks; compose drafts in `flutter_secure_storage`. |
| Implementation order | Shell-first vertical slice: minimal core + app shell + auth/feed first, then grow core and feature modules incrementally. |

---

## 3. Architecture

```mermaid
graph TD
    A[apps/mobile] --> B[packages/japaconnect_core]
    B --> C[API client<br/>OpenAPI-generated]
    B --> D[AuthService<br/>JWT + device ID]
    B --> E[Typed models]
    B --> F[Shared widgets<br/>+ design tokens]
    A --> G[app/ shell + router]
    A --> H[features/ modules]
    G --> I[go_router + deep links]
    H --> J[auth, feed, content, forum, tools, spaces, social, learning, profile, jobs, gamification]
    J --> K[/api/v1 endpoints]
    L[FCM/APNs] --> M[notification:send_push Asynq]
    J --> N[WebSocket hub §10]
```

### Design principles

- The backend is the single source of truth. No hardcoded data, mock objects, or static option lists in production code (AGENTS.md Rule 16).
- Each feature module is self-contained: it owns its screens, providers, services, and widgets, and depends only on `japaconnect_core`.
- Source-level sharing only — no runtime module federation. This enables future standalone tool app extraction (ADR-086 §7).
- The API-Readiness Gate runs before every feature module: every consumed endpoint must exist in `conventions/api-routes-catalog.md` or be provable on the running backend before implementation starts.

---

## 4. `packages/japaconnect_core` package layout

```text
packages/japaconnect_core/
├── pubspec.yaml
├── lib/
│   ├── api/
│   │   ├── client.dart              # JcApiClient wrapper (base URL, auth header, device ID, refresh)
│   │   ├── generated/               # OpenAPI-generated client and models
│   │   └── api_error.dart           # typed error parsing
│   ├── auth/
│   │   ├── service.dart             # login, refresh, logout, token state
│   │   ├── device_id.dart           # stable UUID generation/persistence
│   │   └── token_store.dart         # flutter_secure_storage abstraction
│   ├── models/
│   │   └── ...                      # re-exports and hand-written convenience extensions
│   ├── widgets/
│   │   ├── tokens/
│   │   │   ├── colors.dart          # JcColors
│   │   │   ├── typography.dart      # JcTypography
│   │   │   └── spacing.dart         # JcSpacing
│   │   ├── entity_card.dart
│   │   ├── feed_card.dart
│   │   ├── avatar_widget.dart
│   │   ├── loading_shimmer.dart
│   │   ├── jc_button.dart
│   │   ├── jc_icon_button.dart
│   │   ├── jc_text_field.dart
│   │   ├── jc_badge.dart
│   │   ├── jc_pill.dart
│   │   ├── jc_error_widget.dart
│   │   └── empty_state.dart
│   └── japaconnect_core.dart        # public barrel export
└── test/
    ├── api/
    ├── auth/
    ├── models/
    └── widgets/
```

### Responsibilities

- **API client** — typed methods for every `/api/v1` route, auto-generated from the OpenAPI spec. Handles 401 refresh, `X-Device-Id` injection, and standard error format (`{ error: { code, message, status } }`).
- **Auth service** — manages in-memory access token, persisted refresh token, device ID, and automatic refresh on 401. Max one refresh retry per request.
- **Models** — typed Dart classes for all API response shapes. Nullable where the schema is nullable.
- **Shared widgets** — JapaConnect brand tokens and reusable UI primitives. Feature modules consume these; they do not define duplicate components.

---

## 5. `apps/mobile` package layout

```text
apps/mobile/
├── pubspec.yaml
├── lib/
│   ├── main.dart                    # ProviderScope, GoRouter, app entry
│   ├── app/
│   │   ├── app_shell.dart           # bottom nav + top app bar + safe area
│   │   ├── router.dart              # go_router configuration
│   │   ├── theme.dart               # JapaConnect tokens + dark mode
│   │   ├── auth_guard.dart          # redirect to /login when unauthenticated
│   │   └── global_providers.dart    # providers that are truly app-wide
│   └── features/
│       ├── auth/
│       │   ├── screens/
│       │   ├── providers/
│       │   ├── services/
│       │   └── widgets/
│       ├── feed/
│       ├── content/                 # blog, guides, comparisons
│       ├── forum/
│       ├── tools/
│       │   ├── hub/
│       │   ├── visa_checklist/
│       │   ├── readiness/
│       │   ├── cost_of_living/
│       │   ├── scholarships/
│       │   ├── remittance/
│       │   └── japabot/
│       ├── spaces/
│       ├── social/                  # follow, notifications, DMs
│       ├── learning/                # Japa University
│       ├── profile/                 # profile, settings, wallet, bookmarks
│       ├── gamification/            # badges, challenges, leaderboard
│       └── jobs/
├── test/
│   └── features/
└── integration_test/
    └── auth_flow_test.dart
```

### Feature module rules

Each feature module under `lib/features/<name>/` must:

- Contain its own `screens/`, `providers/`, `services/`, and `widgets/`.
- Call the API exclusively through `japaconnect_core`.
- Not import from sibling `features/` modules.
- Run the API-Readiness Gate before implementation.

---

## 6. Feature module inventory

| Module | Primary route groups | Notes |
|---|---|---|
| `auth` | `POST /api/v1/auth/*`, `GET /api/v1/onboarding/*` | Login, register, forgot, TOTP, onboarding. |
| `feed` | `GET /api/v1/feed`, `GET /api/v1/feed/trending` | Home feed, pull-to-refresh, infinite scroll. |
| `content` | `GET /api/v1/blog`, `GET /api/v1/blog/:slug`, `GET /api/v1/guides`, `GET /api/v1/comparisons` | Blog, guides, comparisons. |
| `forum` | `GET /api/v1/forum/*`, `POST /api/v1/forum/*/replies` | Thread list, detail, reply. |
| `tools/hub` | `GET /api/v1/feature-flags`, various tool routes | Tools landing screen. |
| `tools/visa_checklist` | `GET /api/v1/checklist/*`, `PATCH /api/v1/checklist/*/progress/*` | Unlocked in Phase 2. |
| `tools/readiness` | `GET /api/v1/assessment`, `POST /api/v1/assessment/submit` | Unlocked in Phase 2. |
| `tools/cost_of_living` | `GET /api/v1/calculator/cost-of-living/*` | Unlocked in Phase 2. |
| `tools/scholarships` | `GET /api/v1/scholarships`, `POST /api/v1/scholarships/:id/save` | Unlocked in Phase 2. |
| `tools/remittance` | `GET /api/v1/remittance/*` (per §74) | Unlocked in Phase 2. |
| `tools/japabot` | `POST /api/v1/ai/chat`, `GET /api/v1/ai/conversations` | JapaBot chat. |
| `spaces` | `GET /api/v1/spaces`, `GET /api/v1/spaces/:slug/*`, `POST /api/v1/spaces/:id/join` | Space list, detail, feed. |
| `social` | `POST /api/v1/follow/*`, `GET /api/v1/notifications`, `GET /api/v1/messages/*` | Follow, notifications, DMs. |
| `learning` | `GET /api/v1/university/courses`, `POST /api/v1/university/courses/:id/enroll` | Japa University. |
| `profile` | `GET /api/v1/me`, `PATCH /api/v1/me`, `GET /api/v1/me/wallet`, `GET /api/v1/me/bookmarks` | Profile, settings, wallet, bookmarks. |
| `gamification` | `GET /api/v1/me/badges`, `GET /api/v1/challenges/*` (per §64) | Badges, challenges, leaderboard. |
| `jobs` | `GET /api/v1/jobs/*` (per §63) | Jobs finder, applications. |

---

## 7. Authentication and session model

- **Access token** — 15-minute JWT, held only in memory.
- **Refresh token** — persisted in `flutter_secure_storage` (iOS Keychain / Android Keystore), scoped to device.
- **Device ID** — stable UUID generated on first launch, persisted in `flutter_secure_storage`, sent as `X-Device-Id` on every request.
- **Refresh flow** — on 401, call `POST /api/v1/auth/refresh` with the refresh token and device ID. Update the in-memory access token and retry the failed request once. If refresh fails, clear tokens and redirect to `/login`.
- **TOTP** — when `POST /api/v1/auth/login` requires 2FA, show `TOTPPrompt` and call `POST /api/v1/auth/totp/verify-challenge`.
- **Logout** — call `POST /api/v1/auth/logout`, deregister push token with `DELETE /api/v1/me/devices/:device_id`, clear stored tokens.
- **CSRF** — Bearer-token auth is CSRF-immune; no `X-CSRF-Token` header from mobile.

---

## 8. Push notifications and real-time

### Push

- `firebase_messaging` for both Android and iOS.
- Request notification permissions on first launch.
- On token refresh, call `POST /api/v1/me/devices` with `{ device_id, platform, token }`.
- Store token locally only for re-registration; the backend owns the authoritative device list (`GET /api/v1/me/devices`).
- On logout, call `DELETE /api/v1/me/devices/:device_id`.
- Foreground messages show an in-app snackbar or update the notifications badge.
- Background/terminated messages route via deep link (`/notifications`, `/messages/:id`, `/spaces/:slug`, etc.).
- The `notification:send_push` Asynq task (Backend Phase 47) dispatches to FCM/APNs and marks invalid tokens inactive.

### WebSocket

- `web_socket_channel` connects to the existing hub.
- Obtain a ticket via `POST /api/v1/auth/ws-ticket`.
- Exponential-backoff reconnect on disconnect.
- Listen for `notification`, `feed_update`, `dm`, `presence`, `space_update`, `moderation_alert`, `system`.

---

## 9. Offline and caching policy

- **Feed and content** — in-memory Riverpod cache; show last-loaded state while a background refresh runs.
- **No local database** — SQLite and Hive are deferred to v3 if offline-first is ever warranted.
- **Optimistic UI** — reactions, follows, bookmarks update local state first and revert on API error with a snackbar.
- **Compose drafts** — small JSON blob in `flutter_secure_storage`, restored on reopen.
- **Image cache** — `CachedNetworkImage` disk cache.

---

## 10. API-Readiness Gate

Before any screen or feature module:

1. List every API endpoint the module consumes (reads and mutations).
2. Verify each endpoint exists in `conventions/api-routes-catalog.md` or by probing the running backend.
3. If any endpoint is missing or returns 404, stop. Do not implement. Do not mock. Do not hardcode placeholder data.

Rule 16 applies to Flutter exactly as to SvelteKit:
- No hardcoded data arrays, mock objects, or static option lists.
- All user-owned or tenant-configurable values come from `/api/v1`.
- All mutations call `POST/PATCH/DELETE` backend routes and invalidate/refetch state.

---

## 11. Testing and CI

- **Widget tests** — `test/features/<name>/` for every feature module; `test/widgets/` for shared primitives.
- **Integration tests** — `integration_test/auth_flow_test.dart` and `integration_test/feed_flow_test.dart`.
- **Static analysis** — `flutter analyze` on every PR.
- **Unit tests** — `flutter test` for `japaconnect_core` and `apps/mobile`.
- **Build checks** — CI runs `flutter build apk` (Android) and `flutter build ios --no-codesign` (iOS compile-only) on PRs.
- **No `bun`** — Flutter uses `flutter` and `dart` CLI directly.

---

## 12. Phased delivery

### Phase 0 — Core + Shell + Auth + Feed

Goal: end-to-end vertical slice proving the stack.

- `packages/japaconnect_core` scaffold: `pubspec.yaml`, OpenAPI client generation, `AuthService`, device ID, base widgets, design tokens.
- `apps/mobile` scaffold: `pubspec.yaml`, `main.dart`, `go_router`, app shell, bottom nav, top app bar.
- Auth module: login, register, forgot, onboarding, TOTP.
- Feed module: home feed with `FeedCard`, pull-to-refresh, infinite scroll.
- Integration test: login → feed load.

### Phase 1 — v1 Community Screens

Goal: complete the v1 community and account experience.

- Discover, blog, guides, comparisons.
- Forum, JapaBot.
- Spaces list/detail/feed, join/leave.
- Follow, notifications list, DMs.
- Japa University course catalog, lesson player.
- Profile, settings, wallet, bookmarks, subscriptions.
- Gamification (badges, challenges, leaderboard).
- Jobs finder.

### Phase 2 — v2 Identity Tools

Goal: add the v2 tools as their backend phases (43–44) complete.

- Visa Checklist.
- Readiness Assessment.
- Cost of Living Calculator.
- Scholarship Finder.
- Remittance Rate Tracker.

### Phase 3 — Push + Polish + Extraction Path

Goal: production readiness and future standalone app extraction.

- FCM/APNs integration and `POST /api/v1/me/devices`.
- WebSocket reconnect and real-time in-app notifications.
- Device token invalidation handling.
- Expanded widget/integration tests.
- CI build pipeline.
- Demonstrate standalone tool extraction path (ADR-086 §7).

---

## 13. Risks and mitigations

| Risk | Mitigation |
|---|---|
| OpenAPI generator drift | Regenerate client from `docs/openapi.json` after backend API changes; add CI check that generated client is current. |
| iOS build/CI on Linux | Use `flutter build ios --no-codesign` for compile checks; macOS + Xcode still required for final archive/signing. |
| FCM/APNs credential management | Store credentials in `system_settings` (encrypted at rest), configured via Control Panel; no secrets in app bundle. |
| State management ramp-up | Use Riverpod consistently; avoid mixing patterns; document provider scoping in code comments. |
| Feature module boundary violations | Enforce via lint/custom rule; no imports from sibling `features/` modules. |

---

## 14. Dependencies (proposed)

`packages/japaconnect_core`:
- `flutter`
- `flutter_riverpod`
- `flutter_secure_storage`
- `firebase_messaging`
- `web_socket_channel`
- `cached_network_image`
- `freezed_annotation`
- `json_annotation`
- `http` (via generated client)
- `openapi_generator` (dev)
- `build_runner`, `freezed`, `json_serializable` (dev)

`apps/mobile`:
- `flutter`
- `japaconnect_core` (path: `../../packages/japaconnect_core`)
- `flutter_riverpod`
- `go_router`
- `firebase_messaging`
- `fl_chart`
- `cached_network_image`
- `dev: flutter_test`, `integration_test`, `mocktail`

---

## 15. See also

- JEOS §77 — Flutter Mobile App
- ADR-067 — Flutter for the Native Mobile App
- ADR-074 — Flutter Mobile App: Screen Contract & Surface Definition
- ADR-086 — Multi-Surface Client Architecture
- ADR-113 — Mobile Framework Agent-Ease Tradeoff
- `conventions/api-routes-catalog.md` — endpoint contract
