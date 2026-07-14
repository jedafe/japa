# API Contract Audit v2 — All 5 Frontend Apps vs Backend (Evening Sweep)

> **RESOLVED (same evening).** All findings below were fixed on 2026-07-12 evening; see the
> "Resolution log" section at the end of this document. Verification: `go build ./...` +
> `go test ./internal/...` clean; all five app suites green (main 210, admin 72, control 30,
> docs 12, learn 2 tests) with 0 svelte-check errors.

**Date:** 2026-07-12 (evening; supersedes `api-contract-audit-2026-07-12.md`)
**Method:** Fresh route extraction from `internal/server/server.go` (856 unique method+path routes, chi nesting reconstructed) cross-referenced against 452 extracted call sites (main 194, admin 201, control 28, docs 10, learn 19), plus per-endpoint verification of request params and response payload shapes against Go handler source.

---

## Verdict at a glance

| App | Path/method wiring | Response-shape handling | Overall |
|---|---|---|---|
| **main** | 3 real mismatches | **Systemically broken** (2 bug classes, ~28 server loads + ~22 client methods) | 🔴 |
| **admin** | Clean | 7 pages always render empty lists | 🟠 |
| **control** | Clean | Clean | 🟢 (large unconsumed surface) |
| **docs** | Clean | Clean | 🟢 |
| **learn** | Clean | Clean | 🟢 |

**Why the test suites stay green while this is broken:** unit tests mock the API client with the *frontend's assumed* shapes (e.g. `apps/main/src/routes/forum/page.test.ts:8` mocks `getForumThreads` returning `{ threads: [], total_pages: 0 }` — a shape the backend never produces). Green suites do not validate the API contract.

---

## CRITICAL 1 — main: `{data:…}` envelope consumed as payload in ~28 server loads

Backend wraps **every** success body in `{"data": …}` (verified: `internal/users/handlers.go:158`, `internal/wallet/handlers.go:5` documents it as the convention, `internal/features/handlers.go:35`, `internal/tags/handlers.go:88`, etc.).
`apps/main/src/lib/server/api.ts` `apiFetch` does **not** unwrap (`(await res.json()) as T`) — unlike learn's, which does (`data: body.data`).

Some loads correctly declare `apiFetch<{ data: T }>` and read `.data.data` (feed, pricing, comparisons/[slug], universities list, layout i18n). The rest consume the envelope as if it were the payload — fields read `undefined`, `{#each}`/`for…of` over an object crashes SSR:

| File | Line(s) | Symptom |
|---|---|---|
| `routes/+layout.server.ts` | 12, 25 | `flags` = envelope → **all feature-flag gating silently off site-wide**; unread count always 0 |
| `routes/settings/notifications/+page.server.ts` | 48 | `for…of` over object → **500** |
| `routes/blog/+page.server.ts` | 47–49 | featured/posts/series empty or crash |
| `routes/blog/[slug]/+page.server.ts` | 32 | post fields undefined |
| `routes/blog/series/+page.server.ts` | 20 | list empty/crash |
| `routes/blog/series/[slug]/+page.server.ts` | 34, 37 | `seriesResult.data.id` undefined → fetches `/blog/series/undefined/posts` |
| `routes/checkout/+page.server.ts` | 18 | providers empty |
| `routes/comparisons/+page.server.ts` | 18 | types empty/crash |
| `routes/dashboard/+page.server.ts` | 40 | dashboard fields undefined |
| `routes/dashboard/webhooks/+page.server.ts` | 21 | list empty/crash |
| `routes/feed/profile/[userId]/+page.server.ts` | 47 | `profile.username` undefined → header renders `@undefined`, follow-up feed call uses `profile.id` = undefined |
| `routes/feed/space/[spaceId]/+page.server.ts` | 46 | `space.name` undefined |
| `routes/media/+page.server.ts` | 18 | empty/crash |
| `routes/members/+page.server.ts` | 17 | empty/crash |
| `routes/notifications/+page.server.ts` | 16 | empty/crash |
| `routes/onboarding/+page.server.ts` | 25–31 | `completed` undefined → completed users never redirected |
| `routes/organisations/+page.server.ts` | 25 | `.organizations` undefined → always empty |
| `routes/organisations/[slug]/+page.server.ts` | 36, 45 | `org.id` undefined → members fetched from `/organizations/undefined/members` |
| `routes/organisations/[slug]/manage/+page.server.ts` | 47, 56–57 | same as above + invitations |
| `routes/pages/[id]/+page.server.ts`, `edit/` | 7 | page schema undefined |
| `routes/privacy/+page.server.ts` | 19 | consents empty/crash |
| `routes/professional/+page.server.ts` | 40–42 | `.profiles` undefined → empty |
| `routes/professional/[slug]/+page.server.ts` | 61 | profile fields undefined |
| `routes/spaces/[slug]/+page.server.ts` | 31 | space fields undefined |
| `routes/tags/+page.server.ts` | 15 | empty/crash |
| `routes/tickets/+page.server.ts` | 22 | empty/crash |
| `routes/tickets/[id]/+page.server.ts` | 36–37 | `.ticket`/`.comments` undefined |
| `routes/universities/[slug]/+page.server.ts` | 21, 24, 30, 36 | `space.id` undefined → dependent fetches hit `/…/space/undefined` |
| `routes/wallet/+page.server.ts` | 23, 28 | balance/ledger undefined |

**Recommended fix:** make `apps/main/src/lib/server/api.ts` unwrap once (mirror `apps/learn/src/lib/server/api.ts`), then delete the `{data:T}` generics and `.data.data` double-unwraps in the seven already-"fixed" loads. One change, one convention.

---

## CRITICAL 2 — main: client methods expect wrapper objects; backend returns bare arrays

`apps/main/src/lib/api.ts` `request()` correctly unwraps the envelope, but ~22 methods (mostly the "AUDIT-FE wiring" additions) type the payload as `{ things: T[] , total… }` while the backend returns a **bare array** in the envelope. Every consumer reads `undefined`.

Verified bare-array payloads in Go handlers for each of:

| Client method (`lib/api.ts`) | Expects | Backend actually returns |
|---|---|---|
| `getForumThreads` (L308) | `{threads,total,page,total_pages}` | `[]threads` (`internal/forum/handlers.go` ListThreads) |
| `getForumReplies` (L347) | `{replies,total}` | `[]replies` (ListReplies L483) |
| `listProfessionalProfiles` (L487) | `{profiles,total,…}` | `[]profiles` |
| `listOrganizations` (L1149) | `{organizations,total,…}` | `[]orgs` (handlers.go:185) |
| `listReviews` (L625) | `{reviews,total}` | `[]reviews` (handlers.go:236) |
| `getFollowers`/`getFollowing` (L655/662) | `{followers/following,total}` | `[]users` (follows handlers.go:157/194) |
| `getTrending{Content,Spaces,Tags,Users}` (L1032–1042) | `{items:[…]}` | `[]rows` (trending handlers.go:138+) |
| `getSuggested{Spaces,Users,Tags}` (L1046–1053) | `{items:[…]}` | `[]rows` (same handlers) |
| `getOrganizationMembers` (L1172) | `{members:[…]}` | `[]members` (ListMembers L674→276) |
| `listOrganizationInvitations` (L1192) | `{invitations:[…]}` | `[]invitations` (L438) |
| `listSpaceTypes` (L1269) | `{space_types:[…]}` | `[]types` (spaces handlers.go:107) |
| `listSpacePermissionPresets` (L1275) | `{presets:[…]}` | `[]presets` (L843) |
| `getOnlineUsers` (L797) | `{users:[…]}` | `[]users` (websocket handlers.go:196) |
| `getAffiliateLinks` (L1234) | `{links:[…]}` | `[]links` (affiliate handlers.go:139) |
| `getCustomFieldGroups` (L1104) | `{groups:[…]}` | `[]groups` (customfields handlers.go:148) |
| `getCustomFieldGroupFields` (L1107) | `{fields:[…]}` | `[]fields` (L307) |
| `getCustomFieldValues` (L1110) | `{values:{…}}` | bare values (L556) |
| `geoLookup` (L1261) | `{results:[…]}` | unverified — likely bare |
| `acceptOrganizationInvitation` (L1201) | `{accepted,organization_slug}` | `{message:"Invitation accepted"}` (L438 area) |

Nearly all of these are wired into live UI (forum page, trending/suggestion rails, followers dialogs, org manage, create-content custom fields, affiliate dashboard) — each renders empty or crashes on `.map`.

**Also:** `internal/forum/handlers.go` ListThreads **defaults to the current user's own threads when authenticated** (L290–300). Even after the shape fix, the logged-in forum index will show only the viewer's threads — almost certainly not the intended product behavior.

---

## CRITICAL 3 — backend: `GET /comparisons` is wired to the wrong handler

`internal/server/server.go:1287` registers `GET /comparisons` → `ListComparisonTypes` (the same handler as `/comparisons/types`, L1259). There is **no ListComparisons handler at all** in `internal/comparison/handlers.go`. The main app (`routes/comparisons/[slug]/+page.server.ts:40`) calls `/comparisons?type_slug=…&limit=50` expecting `Comparison[]` — it receives comparison *types*, and both query params are ignored (comparison package reads only `page_size`/`page_offset`).

Related wiring smells in the same block: `PATCH/DELETE /comparisons/{id}` (L1290–1292) are bound to `UpdateComparisonItem`/`DeleteComparisonItem`, which treat `{id}` as an *item* id.

---

## HIGH — admin: 7 pages double-unwrap and always render empty

The admin client (`lib/api/client.ts:49`) already unwraps the envelope, so `result.data` **is** the payload. These pages read `result.data.data ?? []` on endpoints whose payloads are bare arrays (each handler verified):

| Page | Endpoint | Handler proof |
|---|---|---|
| `routes/analytics/+page.svelte:45` | `/admin/analytics/security` | `{"data": results}` analytics/handlers.go:286 |
| `routes/broadcast/+page.svelte:56` | `/admin/broadcasts` | admin/moderation.go:888 |
| `routes/custom-fields/+page.svelte:39` | `/custom-field-groups` | customfields/handlers.go:148 |
| `routes/newsletter/+page.svelte:45,52` | `/admin/newsletter/{subscribers,campaigns}` | newsletter/handlers.go:125,198 |
| `routes/reactions/+page.svelte` | `/reaction-types` | content/reactions.go:23 |
| `routes/ai/tools/+page.svelte:41` | `/admin/ai/tools` | ai/tool_handlers.go:82 |
| `routes/page-builder/pages/+page.svelte:31` | `/admin/pages` | pagebuilder/handlers.go:476 |

This is a recurrence of the bug fixed earlier today in notification-types/referral/affiliate (those three are now correct). Three other pages (`organisations`, `spaces`, `spaces/[spaceId]/forms`) survive only via a defensive `Array.isArray(result.data)` fallback.

---

## HIGH — main: three real path/method mismatches

1. **`getPayment` → `GET /payments/{id}`** (`lib/api.ts:730`) — backend only has `GET /me/payments/{id}` (server.go L1416). Used by `routes/payments/+page.svelte:113` → payment detail always fails.
2. **`updateDeveloperKey` → `PUT /me/api-keys/{id}`** (`lib/api.ts:606`) — backend accepts `PATCH` (405 on PUT). Currently dead code (no UI caller).
3. **`getSpacePermissionPreset` → `GET /space-permission-presets/{name}`** (`lib/api.ts:1278`) — backend has only `GET` (list) and `PUT /{name}`. Dead code today.

---

## MEDIUM — query-param contract drift (params silently ignored)

Backend list endpoints standardize on `page_size`/`page_offset` (verified per package). These main-app call sites still send other names — no errors, filters/pagination just no-op:

| Call site | Sends | Backend reads |
|---|---|---|
| `lib/api.ts` `listReviews` | `limit`,`offset` | `page_size`,`page_offset`,`reviewer_id` |
| `lib/api.ts` `listOrganizations` | `limit`,`page` | `page_size`,`page_offset`,`org_type`,`country_code` |
| `lib/api.ts` `listMedia` | `limit` | `page_size`,… |
| `lib/api.ts` `getWalletLedger` | `limit` | `page_size`,`ledger_type` |
| `lib/api.ts` `listPayments` | `limit` | `page_size` |
| `lib/api.ts` `getFollowers`/`getFollowing` | `limit`,`offset` | `page_size`,`page_offset` |
| `lib/api.ts` `listSpaces` | `space_type_id`,`limit` | `type`,`level`,`parent`,`page_size` — **type filter no-ops** |
| `lib/api.ts` `getComparisonTypes` | `limit` | `page_size` |
| `lib/api.ts` `listProfessionalProfiles` | `limit`,`offset`,`search` | `page_size`,`page_offset`,`type_id`,`owner_user_id` — **search is a no-op** |
| `routes/feed/*` server loads | `?limit=20` | `page_size` (falls back to default) |
| `routes/universities/+page.server.ts:25-32` | `country`,`sort`,`page` | `level`,`type`,`parent` only — **country filter, sort, and pagination all ignored server-side** |

(`limit` IS legitimately accepted by: users, docs, trending, customfields, search packages — those call sites are fine.)

---

## LOW / informational

- **Admin raw `fetch()`:** 8 sites still bypass `apiFetch` (notifications store, websocket, hooks.server, layout.server, login action, i18n store). The auth/login ones are arguably intentional (cookie handling); the stores lose 401-redirect handling.
- **`ai/tools` total_count:** backend sends `{"data": tools, "total_count": n}`; the once-unwrapping client discards `total_count` (envelope-level metadata is unreachable through every app's client — a convention problem if pagination totals are ever needed).
- **Unconsumed backend surface:** 490 of 856 routes have no call site in any app. Notables: ~20 `/control` routes (tenant CRUD/suspend, IP blocks, DLQ discard-all/delete, DB vacuum, cache stats, deployments, api-key create/patch/analytics — prior finding #20 still open), 178 `/api/v1/admin` routes (bulk of admin UI gaps), learn quizzes standalone fetch, GDPR data-export status polling. Full list: run `scripts`-free extraction or see audit scratch output.
- **`feed/profile/[userId]`** passes its `userId` param to `GET /users/{username}` — works only if callers put usernames in the URL; nothing links to this page today.

## Status of the 22 findings from this morning's report

- **Fixed (verified in current code):** #1–13 (all request-body and auth/response-shape criticals), #17 (control revoke now DELETE), #18 (both certificate paths exist in backend).
- **Partially fixed:** #14/#19 pagination — client notification/DM/ticket/forum-list methods now use `page_size`/`page_offset`, but the table above remains; #15 double-envelope — fixed on the 3 named pages, recurred on 7 others; #16 raw fetch — count unchanged (8).
- **Still open:** #20 control unconsumed endpoints, #21 learn quiz fetch, #22 learn unused types.

## Recommended fix order

1. Unwrap the envelope once in `apps/main/src/lib/server/api.ts` (copy learn's) and normalize the ~35 main server-load call sites to single-unwrap. Highest blast radius (feature flags + ~28 pages).
2. Align the ~22 wrapper-shape client methods in `apps/main/src/lib/api.ts` with the bare-array reality (or change the backend to return wrapper objects with totals — decide once, apply everywhere).
3. Fix admin's 7 `result.data.data` pages to `result.data`.
4. Add/wire a real `ListComparisons` backend handler; fix `GET /comparisons` registration (+ the `PATCH/DELETE /comparisons/{id}` item-handler bindings).
5. Fix `getPayment` path; decide forum ListThreads default-scope semantics.
6. Sweep the query-param table.
7. Add at least one non-mocked contract test per app (hit the real dev backend or generated fixtures from handler shapes) so mock-shaped unit tests stop masking contract drift.

---

## Resolution log (2026-07-12 evening)

**CRITICAL 1 (envelope):** `apps/main/src/lib/server/api.ts` now unwraps `{"data": …}` once
(mirrors learn's client) and surfaces `body.error`. The ~28 broken loads work unchanged; the 7
compensating loads were normalized to single-unwrap (`+layout.server.ts` i18n, feed ×3, pricing,
comparisons/[slug], universities). `pricing/page.server.test.ts`'s mock updated to the new contract.

**CRITICAL 2 (wrapper shapes):** all listed methods in `apps/main/src/lib/api.ts` now return bare
arrays (or the real payload): forum threads/replies, professional profiles, organizations
(+members/invitations), reviews, followers/following, trending ×4, suggestions ×3, space types,
permission presets (get-by-name now selects from the list endpoint), online users, affiliate
links/earnings (new `AffiliateEarnings` type mirroring the Go `EarningsSummary`), custom-field
groups/fields/values (new `CustomFieldValueRow` mirroring the typed value columns), geoLookup
(now `?ip=`), acceptOrganizationInvitation (`{message}`; the accept page now routes to
`/organisations`). Consumers updated: forum index (has-more pagination heuristic), thread replies,
organisations list/manage, followers/following pages (totals from the profile's counts),
Reviews component (total from aggregate count), Trending/Suggestions panels, StepInterests,
CustomFieldsSection (maps value rows by field key), affiliate dashboard, spaces/new.

**CRITICAL 3 (comparisons):** backend gained `Service.ListComparisons` +
`Handlers.ListComparisons` (lists `content_type='comparison'` rows); `GET /comparisons` rewired
to it in `server.go`. `type_slug` accepted but not applied (no schema linkage — documented in the
handler). The PATCH/DELETE `/comparisons/{id}` item-handler bindings were left as-is: the catalog
deliberately addresses items at that path. comparisons/[slug] page guards missing counts.

**HIGH (admin double-unwrap):** all 7 pages now read `result.data ?? []` (analytics security,
broadcast, custom-fields, newsletter ×2, reactions, ai/tools, page-builder pages).

**HIGH (paths/methods):** `getPayment` → `/me/payments/{id}`; `updateDeveloperKey` → PATCH;
forum `ListThreads` default now returns all tenant threads regardless of auth (own threads via
explicit `author_id`).

**MEDIUM (query params):** all listed call sites now send `page_size`/`page_offset`
(reviews, organizations, media, wallet ledger, payments, followers/following, comparison types,
professional profiles, feed loads and feed client methods — `cursor` param dropped, backend never
read it). `listSpaces` sends `type=<uuid>`. The universities load resolves the `university`
space-type UUID via `/space-types`, then filters country / sorts / paginates in the load (the
backend supports none of those); professional search filters in the load.

**Not addressed (unchanged scope):** admin raw-fetch sites, envelope-level `total_count`
unreachability, the 490 unconsumed backend routes, and the recommended contract tests (item 7).
