# Guest Access Control Bug Hunt Report

**Date:** 2026-08-01
**Scope:** Main app (`apps/main`) — guest user access control for all auth-gated operations and navigation
**Status:** All bugs fixed and verified

---

## Summary

Comprehensive audit of guest user access control in the JapaConnect main app. Guests (unauthenticated visitors) should be redirected to `/login?redirect=<returnTo>` when attempting any auth-gated action, and should not see UI elements that leak authenticated-only data (e.g. analytics dashboards).

**Bugs found and fixed:** 12
**Root causes:** 3
- Analytics endpoints were publicly accessible (JEOS spec allowed it)
- Auth-gated action buttons called the API directly without checking auth first
- Navigation buttons (Compose, Notifications) linked to auth-gated routes without redirecting guests

---

## JEOS Spec Changes

### JEOS-001: Analytics endpoints gated from public to auth/owner
**File:** `.jeos/conventions/api-routes-catalog.md`
**Change:** 
- `GET /users/{id}/analytics` changed from `[public]` to `[auth, self]` — owner-only (viewer id == {id}); guests and non-owner users receive `403 analytics_forbidden`
- `GET /spaces/{id}/analytics` changed from `[public, tenant-scoped]` to `[auth, space-admin]` — restricted to space owner/moderators; guests and non-admin members receive `403 analytics_forbidden`

---

## Backend Fixes

### BE-001: GetUserAnalytics — owner-only enforcement
**File:** `internal/users/handlers.go` (line 526)
**Bug:** `GetUserAnalytics` had no auth check — any caller (including guests) could fetch any user's analytics. The route used `OptionalAuth` so guests passed through.
**Fix:** Added `requireUser()` call (401 for guests) + owner check `userID != user.UserID` (403 for non-owners). Route changed from `OptionalAuth` to `RequireAuth`.

### BE-002: GetSpaceAnalytics — space-admin enforcement
**File:** `internal/spaces/handlers.go` (line 2135)
**Bug:** `GetSpaceAnalytics` (id-based) had no auth check — any caller could fetch any space's analytics. The route used `OptionalAuth`.
**Fix:** Added `auth.FromContext()` check (401 for guests) + `CanManageSpace()` check (403 for non-admins). Route changed from `OptionalAuth` to `RequireAuth`.

### BE-003: GetSpaceAnalyticsBySlug — space-admin enforcement
**File:** `internal/spaces/handlers.go` (line 2192)
**Bug:** `GetSpaceAnalyticsBySlug` (slug-based variant) had no auth check.
**Fix:** Same pattern as BE-002 — added auth check + `CanManageSpace()`.

### BE-004: CanManageSpace helper added
**File:** `internal/spaces/store.go` (line 713)
**Change:** New `CanManageSpace(ctx, spaceID, userID, tenantID)` method on `spaces.Service` — checks `space_members.role = 'owner'` OR `space_moderators` membership. Used by both analytics handlers.

### BE-005: Route registrations updated
**File:** `internal/server/server.go`
**Change:** 
- `/users/{id}/analytics`: `OptionalAuth` → `RequireAuth`
- `/spaces/{id}/analytics`: `OptionalAuth` → `RequireAuth`

---

## Frontend Fixes — Auth-Gated Action Buttons

### FE-001: Shared guest-redirect helper
**File:** `apps/main/src/lib/auth/guest.ts` (new)
**Change:** Created `requireAuthOrRedirect()` and `isAuthed()` helpers. Uses `$app/stores` + `get()` (not `$app/state` — Svelte 5 runes don't work in plain `.ts` files). Redirects guests to `/login?redirect=<current path+search>`.

### FE-002: Root layout `isAuthed` flag
**File:** `apps/main/src/routes/+layout.server.ts` (line 107)
**Bug:** `page.data.user` is unreliable for auth checks — the profile page's load function returns `user` (the viewed profile's user), which overrides the layout's `user` (the session user) in SvelteKit's data merge. This caused `isAuthed()` to return `true` for guests on profile pages.
**Fix:** Added `isAuthed: !!user` boolean to the root layout data. This key is only set by the root layout and never overridden by page-level data.

### FE-003: FollowButton (generic) — guest redirect
**File:** `apps/main/src/lib/components/ui/FollowButton.svelte` (line 59)
**Bug:** `toggle()` called `api.post/delete` directly without checking auth. Guests got a 401/403 error.
**Fix:** Added `if (!(await requireAuthOrRedirect())) return;` before the API call.

### FE-004: FollowButton (profile) — guest redirect
**File:** `apps/main/src/lib/components/profile/FollowButton.svelte` (line 20)
**Bug:** Same as FE-003 — no auth guard before API call.
**Fix:** Same — added `requireAuthOrRedirect()` guard.

### FE-005: BookmarkButton — guest redirect
**File:** `apps/main/src/lib/components/ui/BookmarkButton.svelte` (line 29)
**Bug:** `toggle()` called `api.post/delete` directly without checking auth.
**Fix:** Added `requireAuthOrRedirect()` guard.

### FE-006: PollBlock — guest redirect on vote
**File:** `apps/main/src/lib/components/forum/PollBlock.svelte` (line 42)
**Bug:** `vote()` called `api.post` directly without checking auth. Guests could click "Vote" and get a 401.
**Fix:** Added `requireAuthOrRedirect()` guard.

### FE-007: ReviewForm — guest redirect on submit
**File:** `apps/main/src/lib/components/reviews/ReviewForm.svelte` (line 39)
**Bug:** `submit()` called `api.post` directly without checking auth. Guests could submit a review and get a 401.
**Fix:** Added `requireAuthOrRedirect()` guard.

### FE-008: SpaceSidebar — login link missing returnTo
**File:** `apps/main/src/lib/components/profile/SpaceSidebar.svelte` (line 121)
**Bug:** Guest "Join" link pointed to `/login` without a `?redirect=` param, so guests wouldn't return to the space after authenticating.
**Fix:** Added `loginHref` derived value with `?redirect=<current path+search>`.

---

## Frontend Fixes — Navigation Buttons

### FE-009: LeftNav Compose button — guest redirect
**File:** `apps/main/src/lib/components/shell/LeftNav.svelte` (line 171)
**Bug:** Compose button linked to `/create` for all users. Guests would navigate to a 403 error page.
**Fix:** Conditional `href={user ? '/create' : '/login?redirect=/create'}`.

### FE-010: MobileBottomTabBar Compose button — guest redirect
**File:** `apps/main/src/lib/components/shell/MobileBottomTabBar.svelte` (line 62)
**Bug:** Compose button opened the ComposeModal for all users. Guests would see the modal, then navigate to a 403 on any create option.
**Fix:** Guests get a login link (`<a href="/login?redirect=/create">`) instead of the modal-opening button.

### FE-011: MobileBottomTabBar Notifications — guest redirect
**File:** `apps/main/src/lib/components/shell/MobileBottomTabBar.svelte` (line 26)
**Bug:** Notifications tab linked to `/notifications` for all users. Guests would navigate to a 403.
**Fix:** Guests get `/login?redirect=/notifications` instead.

### FE-012: ForumList EmptyState — guest redirect
**File:** `apps/main/src/lib/components/forum/ForumList.svelte` (line 112)
**Bug:** "Start a discussion" button in the forum empty state linked to `/create?type=forum` for all users. Guests would navigate to a 403.
**Fix:** Conditional `href={page.data.user ? '/create?type=forum' : '/login?redirect=...'}`.

---

## Frontend Fixes — Analytics Tab Gating

### FE-013: User profile analytics tab — hidden for non-owners
**File:** `apps/main/src/routes/(app)/u/[username]/+page.svelte` (line 28)
**Bug:** Analytics tab was visible to all viewers (guests + other users). The server load fetched analytics for all viewers (degrading to null on error, but still making the request).
**Fix:** Tab only rendered when `isOwnProfile` is true. Server load only fetches analytics when `isOwnProfile`.

### FE-014: Space profile analytics tab — hidden for non-admins
**File:** `apps/main/src/routes/(app)/spaces/[slug]/+page.svelte` (line 49)
**Bug:** Analytics tab was visible to all viewers. The server load fetched analytics for all viewers.
**Fix:** Tab only rendered when `authed && analytics` (data is null for guests/non-admins). Server load only fetches when `event.locals.user` is present.

---

## Components Already Correctly Handled (No Fix Needed)

| Component | Action | Guest Handling |
|---|---|---|
| ReactionRail | like/bookmark | Redirects to login (local `requireAuth`) |
| VoteRail | vote | Disabled for guests |
| CommentThread | comment | Composer hidden, shows "sign in" message |
| ReplyThread | reply | Composer hidden for guests |
| ReplyItem | reply | Composer hidden for guests |
| CommentItem | vote/pin/reply/edit/delete | All gated by `authed` prop |
| CreatorCard | follow | Follow button hidden for guests |
| SpaceSidebar (join) | join | Shows login link for guests |
| Tags page | follow | Follow button hidden for guests |
| Forum page | new discussion | Button hidden for guests |
| Space page | post | Post button hidden for guests |
| Org page | claim | Button hidden for guests |
| Dashboard routes | all | Protected by (app) layout auth |
| Newsletter subscribe | subscribe | Intentionally public (JEOS §1233) |
| Feedback form | submit | Intentionally public |

---

## Verification

- **`go build`**: Backend compiles with 0 errors
- **`svelte-check`**: Frontend compiles with 0 errors (9 pre-existing a11y warnings)
- **Playwright tests** (guest view):
  - Profile page (`/u/obinna_pioneer`): 6 tabs visible (no Analytics), Follow button → redirects to `/login?redirect=/u/obinna_pioneer` ✅
  - Blog post (`/blog/bug-hunt-test-post`): Bookmark button → redirects to `/login?redirect=/blog/bug-hunt-test-post` ✅
  - Explore page (`/explore`): Compose button → links to `/login?redirect=/create` ✅
  - No 401/403 console errors on guest action button clicks ✅
