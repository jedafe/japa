# API Contract Audit — All 5 Frontend Apps vs Backend

**Date:** 2026-07-12  
**Scope:** admin, control, docs, learn, main apps  
**Backend:** Go (Chi router) at `/api/v1`  
**Frontends:** SvelteKit (SSR + client-side)

---

## CRITICAL: Request Body Shape Mismatches

These will cause runtime failures — the backend will reject or misinterpret the request.

### 1. MAIN — Comment vote body uses `value` instead of `direction`

| | Frontend (`apps/main/src/lib/api.ts:~line 350`) | Backend |
|---|---|---|
| **Endpoint** | `POST /comments/{id}/vote` | Same |
| **Body sent** | `{ "value": 1 \| -1 }` | Expects `{ "direction": "up" \| "down" }` |

The frontend sends a numeric `value` field, but the backend expects a string `direction` field with values `"up"` or `"down"`.

### 2. MAIN — Content creation uses `content` field instead of `body`

| | Frontend (`apps/main/src/lib/api.ts`) | Backend |
|---|---|---|
| **Endpoint** | `POST /content` | Same |
| **Body sent** | `{ content_type, title, "content": "...", status, field_values }` | Expects `{ space_id, content_type, title, "body": "...", visibility, status }` |

Two issues:
- Field name mismatch: `content` vs `body`
- Missing required field: `space_id` is not sent by the frontend

### 3. MAIN — Content reactions use `reaction_type_id` instead of `key`

| | Frontend (`apps/main/src/lib/api.ts`) | Backend |
|---|---|---|
| **Endpoint** | `POST /content/{id}/reactions` | Same |
| **Body sent** | `{ "reaction_type_id": "..." }` | Expects `{ "key": "..." }` |

### 4. MAIN — Forum vote uses `value` instead of `direction`

| | Frontend (`apps/main/src/lib/api.ts`) | Backend |
|---|---|---|
| **Endpoint** | `POST /forum/vote` | Same |
| **Body sent** | `{ entity_type, entity_id, "value": 1 \| -1 }` | Expects `{ entity_type, entity_id, "direction": "up" \| "down" }` |

### 5. MAIN — TOTP disable sends `password` instead of `code`

| | Frontend (`apps/main/src/lib/api.ts`) | Backend |
|---|---|---|
| **Endpoint** | `DELETE /me/totp` | Same |
| **Body sent** | `{ "password": "..." }` | Expects `{ "code": "..." }` |

### 6. LEARN — Enrollment calls non-existent endpoint

| | Frontend (`apps/learn/src/lib/api.ts:77`) | Backend |
|---|---|---|
| **Endpoint called** | `POST /me/enrollments` with `{ track_id }` | Has `POST /learn/tracks/{id}/enroll` |
| **Result** | **404 Not Found** | Endpoint doesn't exist at this path |

The backend provides `POST /learn/tracks/{id}/enroll`, but the frontend calls `POST /me/enrollments` which doesn't exist. Enrollment creation is completely broken.

### 7. LEARN — Progress endpoint response shape conflict

| | Frontend client (`apps/learn/src/lib/api.ts:68`) | Frontend server (`apps/learn/src/routes/tracks/[slug]/+page.server.ts:6`) |
|---|---|---|
| **Endpoint** | `GET /learn/progress/{trackId}` | Same |
| **Expected shape** | `{ completed, total, percent }` | `{ track_id, progress, completed_lessons[] }` |

The lesson page uses `progress.percent` (client shape) while the track page uses `progress.completed_lessons` (server shape). One of these will always be `undefined`.

---

## HIGH: Response Envelope / Shape Mismatches

These won't crash but will cause incorrect UI state or missing data.

### 8. MAIN — Login response expects `token` but backend returns `access_token`

| | Frontend | Backend |
|---|---|---|
| **Endpoint** | `POST /auth/login` | Same |
| **Expected** | `{ token: string, user: UserProfile }` | `{ access_token, refresh_token, user }` |

Frontend reads `result.token` → will be `undefined`. Auth token will not be captured.

### 9. MAIN — Register response shape mismatch

| | Frontend | Backend |
|---|---|---|
| **Endpoint** | `POST /auth/register` | Same |
| **Expected** | `{ success: boolean }` | `{ access_token, refresh_token, user }` |

Frontend ignores the returned tokens and user object.

### 10. MAIN — Auth status responses use wrong shape

| Endpoint | Frontend expects | Backend returns |
|---|---|---|
| `POST /auth/forgot-password` | `{ sent: boolean }` | `{ status: "reset_email_sent" }` |
| `POST /auth/reset-password` | `{ success: boolean }` | `{ status: "password_reset" }` |
| `GET /auth/verify-email` | `{ verified: boolean }` (JSON) | HTML redirect page |
| `POST /auth/resend-verification` | `{ sent: boolean }` | `{ status: "verification_sent" }` |

### 11. MAIN — TOTP response shape mismatches

| Endpoint | Frontend expects | Backend returns |
|---|---|---|
| `POST /me/totp/enable` | `{ secret, qr_uri }` | `{ secret, otpauth_url }` (field name differs) |
| `POST /me/totp/confirm` | `{ verified: boolean, backup_codes[] }` | `{ status: "enabled" }` |
| `DELETE /me/totp` | `{ disabled: boolean }` | `{ status: "disabled" }` |

### 12. MAIN — Notification response shapes

| Endpoint | Frontend expects | Backend returns |
|---|---|---|
| `POST /me/notifications/{id}/read` | `{ id: string }` | `{ status: "read" }` |
| `POST /me/notifications/read-all` | `{ marked }` | `{ status: "all_read" }` |

### 13. MAIN — Comment/Content mutation responses

| Endpoint | Frontend expects | Backend returns |
|---|---|---|
| `POST /comments/{id}/vote` | `{ score: number }` | `{ status: "voted" }` |
| `DELETE /comments/{id}/vote` | `{ score: number }` | `{ status: "removed" }` |
| `POST /comments/{id}/pin` | `{ pinned: boolean }` | `{ status: "pinned" }` |
| `DELETE /comments/{id}/pin` | `{ pinned: boolean }` | `{ status: "unpinned" }` |
| `DELETE /comments/{id}` | `{ deleted: boolean }` | `{ status: "deleted" }` |
| `DELETE /content/{id}/reactions/{key}` | `{ removed: boolean }` | `{ status: "removed" }` |
| `POST /content/{id}/pin` | `{ pinned }` | `{ status: "pinned" }` |
| `DELETE /content/{id}/pin` | `{ pinned }` | `{ status: "unpinned" }` |
| `DELETE /media/{id}` | `{ deleted: boolean }` | `{ status: "deleted" }` |

### 14. MAIN — Pagination param inconsistency

The main app mixes pagination conventions across different domains:

| Domain | Params used | Backend expects |
|---|---|---|
| Notifications | `limit`/`offset` | `page_size`/`page_offset` |
| Feed | `limit`/`cursor`/`offset` | `page_size`/`page_offset` |
| Explore | `limit`/`offset` | `page_size`/`page_offset` |
| Forum | `limit`/`offset` | `page_size`/`page_offset` |
| DMs | `page_size`/`page_offset` | `page_size`/`page_offset` ✓ |
| Tickets | `page_size`/`page_offset` | `page_size`/`page_offset` ✓ |

Most list endpoints won't paginate correctly because the query param names don't match.

### 15. ADMIN — Double data envelope bug

Several admin pages wrap the API result type in `{ data: T[] }` causing a double-unwrap issue:

```typescript
// src/routes/notification-types/+page.svelte
const result = await api<{ data: NotificationType[] }>('/admin/notification-types');
types = result.data.data ?? [];  // BUG: result.data is already NotificationType[]
```

The `apiFetch` client already unwraps `body.data`, so `result.data` is `NotificationType[]`. Accessing `.data` on it yields `undefined`.

Affected files:
- `apps/admin/src/routes/notification-types/+page.svelte`
- `apps/admin/src/routes/referral/+page.svelte`
- `apps/admin/src/routes/affiliate/+page.svelte`

### 16. ADMIN — Raw fetch() bypasses error handling

8 raw `fetch()` calls in the admin app bypass the `apiFetch` wrapper, missing:
- Consistent error envelope handling
- 401 redirect logic
- Standardized response parsing

Locations:
- `apps/admin/src/lib/stores/notifications.ts` (3 calls)
- `apps/admin/src/lib/websocket.ts` (1 call)
- `apps/admin/src/hooks.server.ts` (1 call)
- `apps/admin/src/routes/+layout.server.ts` (1 call)
- `apps/admin/src/routes/login/+page.server.ts` (1 call)
- `apps/admin/src/lib/stores/i18n.svelte.ts` (1 call)

---

## MEDIUM: Functional Bugs

### 17. CONTROL — API key revoke calls suspend endpoint

| | Frontend (`apps/control/src/routes/api-keys/+page.svelte`) | Backend |
|---|---|---|
| **Revoke action** | `POST /control/api-keys/{id}/suspend` | Expects `DELETE /control/api-keys/{id}` |
| **Suspend action** | `POST /control/api-keys/{id}/suspend` ✓ | Same |

Both suspend and revoke hit the same endpoint. Revoking a key actually suspends it instead.

### 18. LEARN — Certificate path inconsistency

The app uses two different base paths:
- `GET /me/certificates` — for listing
- `GET /certificates/{id}` — for detail/download

If the backend only registers one path, the other will 404.

### 19. ADMIN — Inconsistent pagination conventions

The admin app uses three different pagination schemes:

| Pages using | Param style | Backend expects |
|---|---|---|
| Moderation, Users, Tickets | `page` (zero-indexed) | Varies by endpoint |
| AI HITL, Notifications | `limit`/`offset` | Varies by endpoint |
| (none) | `page_size`/`page_offset` | Backend convention |

The `PaginatedResponse<T>` type is defined but never used.

---

## LOW: Missing Feature Consumption

### 20. CONTROL — Missing backend endpoints

| Backend Endpoint | Control App Status |
|---|---|
| `POST /control/health/db/vacuum` | Not called — no vacuum button in UI |
| `GET /control/cache/stats` | Not called — only flush button exists (type defined but unused) |
| `DELETE /control/queue/dlq/{task_id}` | Not called — only retry available, no discard |
| `POST /control/queue/dlq/discard-all` | Not called |
| `POST /control/system-settings/{key}/reset` | Not called |
| `POST /control/tenants` | Not called — read-only tenant list |
| `PATCH /control/tenants/{id}` | Not called |
| `DELETE /control/tenants/{id}` | Not called |
| `POST /control/tenants/{id}/suspend` | Not called |
| `POST /control/tenants/{id}/unsuspend` | Not called |
| `PATCH /control/ai-budget` | Not called — read-only display |
| `GET /control/deployments` | Not called |
| `GET /control/security/ip-blocks` | Not called |
| `POST /control/security/ip-blocks` | Not called |
| `DELETE /control/security/ip-blocks/{id}` | Not called |

### 21. LEARN — Missing quiz fetch

The backend has `GET /learn/quizzes/{id}` but the learn app never fetches quizzes independently — they're only embedded in lesson data.

### 22. LEARN — Unused type definitions

Types defined in `apps/learn/src/lib/api/types.ts` but not consumed:
- `ReviewAggregate`, `Review` — reviews not wired in UI
- `LessonQuiz` — standalone quiz fetching
- `PaginatedResponse` — pagination wrapper
- `LearnTrack.instructor`, `is_featured`, `price`, `is_pro` — pricing/instructor features

---

## Summary by App

| App | Critical | High | Medium | Low |
|---|---|---|---|---|
| **main** | 5 | 7 | 1 | 0 |
| **admin** | 0 | 2 | 1 | 0 |
| **control** | 0 | 0 | 1 | 15 |
| **learn** | 2 | 0 | 1 | 2 |
| **docs** | 0 | 0 | 0 | 0 |
| **Total** | **7** | **9** | **3** | **17** |

**Most impacted:** The `main` app has the most mismatches — nearly every auth, mutation, and pagination endpoint has incorrect request/response shapes.

**Cleanest:** The `docs` app has zero mismatches — it only consumes docs, API keys, feature flags, and i18n endpoints, all correctly.

---

## Recommended Priority

1. **Fix the 7 critical request body mismatches** — these cause 400/422 errors or wrong server behavior
2. **Fix auth response shapes in main** — login/register/forgot-password are completely broken
3. **Fix learn enrollment endpoint** — enrollment creation is non-functional
4. **Fix admin double-envelope bug** — notification-types, referral, affiliate pages show empty data
5. **Fix control API key revoke** — uses wrong endpoint
6. **Standardize pagination params** across all apps to `page_size`/`page_offset`
7. **Align all response shape expectations** to match the backend's `{ status: "..." }` pattern
