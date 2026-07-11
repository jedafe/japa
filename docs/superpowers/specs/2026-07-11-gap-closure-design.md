# GAP Closure — Remaining Items Implementation Spec

2026-07-11

## Context

The GAP analysis (GAP-ANALYSIS-2026-07-11.md) identified ~20 remaining items. Codebase exploration revealed several already exist (CountdownBlock/TocBlock/SliderBlock, GDPR admin, impersonation, security scan handlers). This spec covers the 12 items that genuinely need work.

## Workstream 1: Backend — Activate Noop Processors

### 1a. Media Processor
- **What:** Wire `internal/media/processoradapter.go` ProcessorAdapter to worker instead of noop
- **How:** In `internal/server/server.go`, before `RegisterNoopProcessors()` call, register the real adapter:
  ```go
  mediaAdapter := media.NewProcessorAdapter(mediaSvc)
  worker.RegisterImageProcessor(mediaAdapter)
  ```
- **Files:** `internal/server/server.go` (add ~3 lines after mediaSvc creation)
- **Risk:** Low — adapter already implements the interface

### 1b. AI Translate Processor
- **What:** Wire `internal/i18n/processoradapter.go` ProcessorAdapter to worker
- **How:** Same pattern — register before noop:
  ```go
  i18nAdapter := i18n.NewProcessorAdapter(i18nSvc)
  worker.RegisterAITranslateProcessor(i18nAdapter)
  ```
- **Files:** `internal/server/server.go` (add ~3 lines after i18nSvc creation)
- **Note:** Current adapter does copy-diff, not actual AI translation. That's acceptable for now.

### 1c. Learning Processor
- **What:** Build a real adapter that calls the learning service to update track completion and issue certificates
- **How:** Create `internal/learning/processoradapter.go` implementing `worker.LearningProcessor`:
  - `UpdateTrackCompletion`: Call learning service to recalculate and persist track completion percentage
  - `IssueCertificate`: Call learning service to generate and store a certificate record
- **Files:** New file `internal/learning/processoradapter.go`, modify `internal/server/server.go` to register
- **Requires:** Learning service methods for completion calculation and certificate issuance (check if they exist in `internal/learning/service.go`)

### 1d. Space Auto-Populate Processor
- **What:** Build a real adapter that auto-populates a space with default content
- **How:** Create `internal/spaces/processoradapter.go` implementing `worker.SpaceAutoPopulateProcessor`:
  - `AutoPopulateSpace`: Create default forum categories, pin welcome post, set default settings
- **Files:** New file `internal/spaces/processoradapter.go`, modify `internal/server/server.go` to register
- **Requires:** Space service methods for creating default content

### 1e. Content Moderation Auto-Hide
- **What:** Build a task handler that auto-hides content when report count exceeds threshold
- **How:**
  1. Add task type `TypeModerationAutoHide = "moderation:auto_hide"` in `internal/worker/tasks.go`
  2. Register in mux in `internal/worker/tasks.go`
  3. Build handler in `internal/worker/handlers.go`: query reports for entity, if count >= `moderation.auto_hide_report_count` setting, soft-delete/hide the content
  4. Add an enqueuer call in the spam/moderation report handler to enqueue `moderation:auto_hide` after each report
- **Files:** `internal/worker/tasks.go`, `internal/worker/handlers.go`, `internal/spam/handlers.go` (or wherever reports are created)
- **Requires:** A report counting mechanism (check if `reports` table exists)

## Workstream 2: Backend — SQLC Dedup Cleanup

### 2a. Advertising Store Cleanup
- **What:** Remove 11 duplicate methods from `internal/advertising/store.go`
- **Methods to remove (pure pass-through wrappers):** `getAdSlotTypeByName`, `createAdSlotType`, `createAdCampaign`, `getAdCampaignByID`, `updateAdCampaignStatus`, `insertAdEvent`, `createOrganizationBoost`, `listOrganizationBoosts`, `listActiveBoosts`
- **Methods to remove (exact sqlc duplicates):** `countImpressionsForUserToday`, `endOrganizationBoost`
- **Methods to KEEP:** `updateAdSlotType`, `deleteAdSlotType`, `deleteAdCampaign`, `getCampaignStats` (no sqlc equivalents), plus 3 pagination-clamping wrappers
- **How:** For each removed method, find all callers in `internal/advertising/handlers.go` and `internal/advertising/service.go` and replace with direct `s.queries.MethodName()` calls
- **Files:** `internal/advertising/store.go`, `internal/advertising/handlers.go`, `internal/advertising/service.go`

### 2b. Auth Store Cleanup
- **What:** Remove 6 duplicate methods from `internal/auth/store.go`
- **Methods to remove (pure pass-through):** `CreateUser`, `StoreRefreshToken`, `RevokeAllRefreshTokens`, `StoreResetToken`, `StoreEmailVerification`, `InsertPasswordHistory`
- **Methods to KEEP:** `GetRefreshToken`, `RevokeRefreshToken`, `GetValidResetToken`, `MarkResetTokenUsed`, `GetValidVerificationToken` (intentional tenant_id divergence), `UpdateUserPassword`, `MarkEmailVerified`, `GetUserByEmail`, `GetRecentPasswordHashes`
- **How:** Replace callers with direct sqlc query calls
- **Files:** `internal/auth/store.go`, `internal/auth/service.go` (or wherever auth methods are called)

## Workstream 3: Frontend — i18n Language Selector

### 3a. Language Toggle Component
- **What:** Add a language toggle button to the main site header
- **How:** Create a small `LanguageToggle.svelte` component that:
  - Reads current locale from `i18n.locale`
  - Shows current language (EN/FR)
  - On click, toggles between available locales
  - Sets cookie `jc_locale` and updates the store
  - Navigates to the equivalent page in the other locale (prefix with `/fr/` or strip it)
- **Files:** New `apps/main/src/lib/components/LanguageToggle.svelte`, modify `apps/main/src/lib/components/nav/Header.svelte` to include it
- **Pattern:** Follow existing header component patterns, use lucide `Globe` icon

## Workstream 4: Frontend — Admin Real-Time Moderation Alerts

### 4a. Polling-Based Alert System
- **What:** Add auto-refresh to the admin moderation page so new reports appear without manual refresh
- **How:**
  - Add a polling interval (every 30 seconds) to `apps/admin/src/routes/moderation/+page.svelte`
  - Use `setInterval` in `onMount` to re-fetch the moderation queue
  - Add a visual indicator (pulsing dot or badge) when new items arrive
  - Add a "New items" banner that appears when the count changes
- **Files:** `apps/admin/src/routes/moderation/+page.svelte`
- **Alternative considered:** WebSocket — overkill for this use case, polling is simpler and sufficient

### 4b. Wire DataTable in Admin
- **What:** Copy `DataTable.svelte` from control app to admin app and use it in moderation page
- **How:**
  - Copy `apps/control/src/lib/components/DataTable.svelte` to `apps/admin/src/lib/components/ui/DataTable.svelte`
  - Adapt the moderation page to use it instead of inline table
- **Files:** New `apps/admin/src/lib/components/ui/DataTable.svelte`, modify `apps/admin/src/routes/moderation/+page.svelte`

## Workstream 5: Frontend — Learn Platform Progress + Certificates

### 5a. Server-Side Progress Tracking
- **What:** Replace the client-only `markLessonComplete` with a server-persisted version
- **How:**
  - Add API endpoint `POST /api/v1/learning/lessons/{lessonId}/complete` (or use existing if present)
  - Modify `markLessonComplete` in `apps/learn/src/lib/stores/enrollments.ts` to call the API
  - Track completed lesson IDs in the enrollment object (add `completed_lesson_ids: string[]` field)
  - Calculate `progress_pct` server-side based on `completed_lesson_ids.length / total_lessons`
- **Files:** `apps/learn/src/lib/stores/enrollments.ts`, check backend for existing completion endpoint
- **Requires:** Backend endpoint for lesson completion (check `internal/learning/handlers.go`)

### 5b. Certificate PDF Generation
- **What:** Implement server-side PDF generation for certificate download
- **How:**
  - Check if `/api/v1/certificates/{id}/download` endpoint exists in backend
  - If not, build it: generate PDF using a Go PDF library (e.g., `signintech/gopdf`) or HTML-to-PDF
  - The PDF should include: certificate holder name, track title, issue date, certificate number, verification URL
- **Files:** Check `internal/learning/handlers.go` for existing download endpoint, potentially new handler
- **Note:** The frontend already has the download link pointing to `/api/v1/certificates/{id}/download` — just need the backend to serve the PDF

### 5c. Lesson Player Enhancements (Optional — Lower Priority)
- **What:** Add progress bar, reading time estimate, and prev/next navigation improvements
- **How:** Enhance the existing lesson page with:
  - A sticky progress indicator showing completion of current lesson
  - Estimated reading time based on content length
  - Keyboard shortcuts (arrow keys for prev/next)
- **Files:** `apps/learn/src/routes/tracks/[trackSlug]/modules/[moduleSlug]/lessons/[lessonSlug]/+page.svelte`

## Workstream 6: Frontend — Docs Developer API Keys

### 6a. API Keys Page
- **What:** Build a self-service API key management page in the docs app
- **How:**
  - Create `apps/docs/src/routes/api-keys/+page.svelte`
  - Page shows: list of user's API keys (name, created date, last used, scopes)
  - Create new key button → modal with name + scope selection → shows key once → copy button
  - Revoke key button with confirmation
  - Uses the existing backend API key endpoints (check `internal/publicapi/` or control panel routes)
- **Files:** New `apps/docs/src/routes/api-keys/+page.svelte`, new `apps/docs/src/routes/api-keys/+page.server.ts`
- **Pattern:** Follow the control panel API keys page pattern

## Implementation Order

1. **Wave 1 (Backend, parallel):** Workstream 1 (processors) + Workstream 2 (sqlc dedup)
2. **Wave 2 (Frontend, parallel):** Workstreams 3-6 can all run in parallel
3. **Wave 3 (Integration test):** Verify all changes work together

## Testing Strategy

- Backend: Run existing tests (`go test ./...`) after each change
- Frontend: Run `bun check` and `bun run build` for each app
- Integration: Manual verification of processor activation, language toggle, moderation alerts, progress tracking, certificate download, API keys page

## Out of Scope

- Lesson Player deep enhancements (speed controls, bookmarks) — can be a follow-up
- Quiz Engine enhancements (question bank, timed quizzes) — can be a follow-up
- Bits UI migration — too broad, separate effort
- Real WebSocket moderation alerts — polling is sufficient for now
