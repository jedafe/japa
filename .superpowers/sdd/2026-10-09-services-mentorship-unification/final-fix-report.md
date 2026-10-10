# Final Whole-Branch Review — Fix Report

Branch: `main` · Date: 2026-10-10 · Scope: the three Important findings from the
final whole-branch review of the services+mentorship unification (000824/000825).

## Finding 1 — Route-group flag gate dead-ended mentorship-only deployments

**Problem.** All §98/§111 routes sat inside a single
`features.Gate(featureChecker, "service_directory.enabled")` group in
`internal/server/server.go`. With `service_directory.enabled=false` +
`mentorship.enabled=true` — a permutation the handlers explicitly support —
every unified mentorship surface 404'd at routing while the legacy
`/mentorship/*` compat group stayed live (inverted intent).

**Fix.**

- `internal/features/features.go` — added `GateAny(checker, keys ...string)`:
  a sibling of `Gate` that 404s with `feature_disabled` only when EVERY listed
  key is disabled for the caller's scope. Generic middleware, no new flag.
- `internal/server/server.go` (~line 7140) — split the servicedir group:
  - **Composite gate** `GateAny(service_directory.enabled, mentorship.enabled)`:
    `GET /services`, `GET /services/{slug}`, `POST /services/{id}/bookings`,
    `GET /me/service-bookings`, `GET /me/service-bookings/{id}`,
    `POST /me/service-bookings/{id}/cancel`,
    `POST /me/service-bookings/{id}/respond`. Kind-appropriate flag checks
    still run inside the handlers, so a non-mentorship listing still 404s
    while the directory is off (packages can't outlive their parent feature).
  - **Directory gate kept** for: categories, reviews, `POST /services`
    (create — mentorship kind is rejected in the service layer per Finding 2),
    `PATCH /services/{id}` (edit — relies on the group gate per the
    `handlers.go:633` contract), claim, coupon attach, admin services,
    admin categories, price-preview, departures, `pay`, `dispute`, org
    routes, admin bookings. `pay`/`dispute` self-gate on pure
    `service_packages.enabled` in the handler (mentorship bookings are
    inquiry-only — no fee, no dispute arm), so they keep directory gating.

**Verification.** `GateAny` unit-tested in `internal/features` (first-key-on →
pass, second-key-on → pass, all-off → 404 `feature_disabled`, empty keys →
404). Route-level coverage: `internal/server` has no httptest harness for the
chi wiring (route registration lives in `NewServer` against live deps), so the
route split is covered structurally — `TestNoDuplicateRoutes` and
`TestNoOverlappingParamRoutes` parse `server.go` and pass; the handler-level
flag matrix is covered by the existing servicedir tests
(`TestListMentorshipKind_LiveDB`, `bookingSurfaceAvailable` paths) plus the
`GateAny` middleware tests.

## Finding 2 — `POST /services` accepted `listing_kind='mentorship'` unguarded

**Problem.** `Service.Create` admitted `listing_kind='mentorship'`, letting any
`service.create` holder forge a self-attributed shadow listing
(`provider_user_id = author`), bypassing `mentorship.become_mentor` + opt-in;
`GetMentorshipListingByAuthor` would adopt the forged row.

**Fix.** `internal/servicedir/service.go` `Create` — after the
`IsValidListingKind` check, `listing_kind='mentorship'` is rejected with the
new typed sentinel `ErrMentorshipListingManaged` → `400
mentorship_listing_managed` (mapped in `writeServiceError`,
`handlers.go:174`). Only `store.CreateMentorShadow` /
`EnsureMentorShadowListing` mints that kind.

## Finding 3 — `EditOwn` let a mentor corrupt their shadow listing

**Problem.** A mentor is the author of their shadow's contents row, so
`PATCH /services/{id}` (`service.edit_own`) could rewrite title/body/category
on the shadow and desync it from the mentor profile.

**Fix.** `Service.EditOwn` refuses `listing_kind='mentorship'` rows for every
caller (author and `service.edit_any` alike) with
`ErrMentorshipListingManaged` — mentors edit via `PATCH /me/mentor-profile`,
which refreshes the shadow through `EnsureMentorShadowListing`. The refusal
runs after `loadListingForEdit`, so non-owners still get `not_owner` (no
kind/existence leak). `EditOwn` is the only edit path (`HandleEdit` is the
sole route; there is no separate admin-edit variant — admin moderation uses
the contents status paths).

## Changes

- `internal/features/features.go` — new `GateAny` middleware.
- `internal/features/features_test.go` — `TestGateAny`; extended
  `TestRegistryCoversAllGatedKeys` to also scan `GateAny` literals.
- `internal/server/server.go` — route-group split (two hunks only; the file
  carries unrelated uncommitted edits staged selectively via patch).
- `internal/servicedir/service.go` — `ErrMentorshipListingManaged` sentinel;
  `Create` rejects kind=mentorship; `EditOwn` refuses mentorship-kind rows.
- `internal/servicedir/handlers.go` — `writeServiceError` maps the sentinel
  to `400 mentorship_listing_managed`.
- `internal/servicedir/service_bookings_test.go` — `readThroughContent` fake +
  `TestMentorshipKindCreateEditGuard_LiveDB` (create reject + no forged row,
  unknown-kind still rejected, author edit refused, edit_any refused,
  non-owner `not_owner` ordering, handler 400 code).
- `.jeos/sections/111-service-packages-bookings.md` — one-paragraph note under
  "Mentorship-kind semantics" (submodule `docs:` commit).
- This report.

## Test results

- `go build ./internal/features/ ./internal/servicedir/` — OK. `go build
  ./internal/server` verified in a clean HEAD worktree with only these
  changes — OK. The working-tree `go build ./...` fails on **pre-existing,
  unrelated uncommitted edits** (`desktopExchangeAuditor` lacks
  `LogExchangeIssuance`; `*db.DB` lacks `ExecAuditLog` at `server.go:2619`),
  not on anything in this change.
- `JC_DATABASE_URL=... go test ./internal/servicedir/ -v` — PASS (all,
  including the 6 new guard subtests).
- `go test ./internal/features/` — PASS (`TestGateAny`, extended registry
  coverage incl. the new `GateAny` call site).
- `go test -run 'TestNoDuplicateRoutes|TestNoOverlappingParamRoutes'
  ./internal/server/` (clean-worktree) — PASS.
- `go vet ./internal/features/ ./internal/servicedir/` — clean; repo-wide vet
  blocked only by the same unrelated `server.go:2619` errors.

## Concerns

- The working tree contains ~45 files of unrelated uncommitted edits,
  including a half-implemented `desktopExchangeAuditor` audit seam that
  **breaks `go build ./internal/server`**. Only this fix's hunks were staged.
  That other work needs finishing or reverting by its owner — flag to parent.
- Route-level flag gating is covered structurally + at middleware/handler
  level (see Finding 1 verification); there is no chi-wiring test harness for
  `NewServer`.
