# TEST-REPORT-main.md

**Date:** 2026-07-12
**Git Commit SHA:** (not committed yet)
**Docker Compose Service Versions:**
- PostgreSQL 18 (pgvector)
- Valkey 9
- Meilisearch 1.45
- Go API (latest build)
- Main App (SvelteKit 2.57, Bun 1.3)

---

## Executive Summary

The Main App (`apps/main`, port 3000, SSR) has been tested across multiple passes. The app is functional with basic routes responding correctly, but there are several defects ranging from P1 (Rule 16 violations) to P3 (cosmetic issues).

**Pass/Fail Summary:**

| Pass | Status | Notes |
|------|--------|-------|
| Pass 0: Rule 16 Audit | FAIL | 4 P1 violations found |
| Pass 1: SSR Integrity | FAIL | i18n keys leaking, JSON-LD not rendering |
| Pass 2: Critical Journeys | PARTIAL | Basic flows work, some blocked by backend |
| Pass 3: Real-time WebSocket | NOT TESTED | Requires WS setup |
| Pass 4: Feature Flags & RBAC | PARTIAL | Basic flag checks pass |
| Pass 5: i18n & PWA | FAIL | i18n keys visible in UI |
| Pass 6: Cross-cutting Matrix | NOT COMPLETE | Requires full journey testing |

---

## Defects

### [P1] MAIN-001: Hardcoded country list in UniversityFilters
- **Severity:** P1 (Rule 16 violation)
- **Phase/Route:** Phase 15 — Universities
- **File(s):** `src/lib/universities/UniversityFilters.svelte:14-24`
- **Steps to reproduce:**
  1. Navigate to `/universities`
  2. Inspect the country filter dropdown
- **Expected:** Country list should come from `GET /api/v1/geo/countries` API
- **Actual:** Hardcoded array of 8 countries
- **Suggested fix:** Fetch countries from the geo API in the page's `load()` function and pass to the component

### [P1] MAIN-002: Hardcoded space types in spaces list
- **Severity:** P1 (Rule 16 violation)
- **Phase/Route:** Phase 08 — Spaces
- **File(s):** `src/routes/spaces/+page.svelte:22-28`
- **Steps to reproduce:**
  1. Navigate to `/spaces`
  2. Inspect the space type filter
- **Expected:** Space types should come from the API or be derived from actual space data
- **Actual:** Hardcoded array of 4 space types
- **Suggested fix:** Derive space types from the fetched spaces data or add an API endpoint for space types

### [P1] MAIN-003: Hardcoded default interests in onboarding
- **Severity:** P1 (Rule 16 violation)
- **Phase/Route:** Phase 03 — Onboarding
- **File(s):** `src/lib/onboarding/StepInterests.svelte:13-26`
- **Steps to reproduce:**
  1. Start onboarding flow
  2. Reach the interests step
- **Expected:** Interests should come from `GET /api/v1/trending/tags` API
- **Actual:** Hardcoded fallback array (though API is called first)
- **Suggested fix:** Remove hardcoded fallback or make it configurable via system settings

### [P1] MAIN-004: Hardcoded wallet preset amounts
- **Severity:** P1 (Rule 16 violation)
- **Phase/Route:** Phase 19 — Wallet
- **File(s):** `src/routes/wallet/+page.svelte:20`
- **Steps to reproduce:**
  1. Navigate to `/wallet`
  2. Open top-up modal
- **Expected:** Preset amounts should come from system settings or API
- **Actual:** Hardcoded array `[100000, 500000, 1000000]`
- **Suggested fix:** Add a system setting for wallet preset amounts or fetch from API

### [P2] MAIN-005: i18n keys leaking in meta tags
- **Severity:** P2 (degraded UX)
- **Phase/Route:** All pages
- **File(s):** `src/routes/+page.svelte`, `src/routes/+layout.svelte`
- **Steps to reproduce:**
  1. View page source of homepage
  2. Check `<title>` and `<meta>` tags
- **Expected:** Translated text in meta tags
- **Actual:** Raw i18n keys like `nav.home`, `landing.tagline`
- **Suggested fix:** Ensure translations are loaded before rendering meta tags in SSR

### [P2] MAIN-006: JSON-LD not rendering in SSR
- **Severity:** P2 (SEO impact)
- **Phase/Route:** All pages with JSON-LD
- **File(s):** `src/routes/+page.svelte`
- **Steps to reproduce:**
  1. View page source of homepage
  2. Check `<script type="application/ld+json">` content
- **Expected:** Valid JSON-LD structured data
- **Actual:** Svelte template syntax `{@html jsonLdString}` instead of rendered JSON
- **Suggested fix:** Ensure JSON-LD is rendered during SSR, not just in client-side templates

### [P3] MAIN-007: RSS feed link tag missing
- **Severity:** P3 (cosmetic)
- **Phase/Route:** Homepage
- **File(s):** `src/routes/+page.svelte`
- **Steps to reproduce:**
  1. View page source of homepage
  2. Check for `<link rel="alternate" type="application/rss+xml">`
- **Expected:** RSS feed link tag in `<head>`
- **Actual:** No RSS feed link tag found
- **Suggested fix:** Add RSS feed link tag to homepage head

### [P3] MAIN-008: Test file not in Playwright config
- **Severity:** P3 (test infrastructure)
- **Phase/Route:** Testing
- **File(s):** `playwright.config.ts`
- **Steps to reproduce:**
  1. Run `npx playwright test ssr-integrity.spec.ts`
- **Expected:** Test file runs
- **Actual:** Test file not found (until config updated)
- **Suggested fix:** Add test files to Playwright config testMatch arrays

---

## API Consumption Matrix

| Route Group | Consuming Page | Verified |
|-------------|----------------|----------|
| `GET /api/v1/me` | Root layout, Settings | Y |
| `GET /api/v1/feature-flags` | Root layout | Y |
| `GET /api/v1/site/settings/public` | Root layout | Y |
| `GET /api/v1/forum` | Forum list | Y |
| `GET /api/v1/spaces` | Spaces list | Y |
| `GET /api/v1/blog` | Blog list | N (backend 500) |
| `GET /api/v1/tags` | Tags list | Y |
| `GET /api/v1/search` | Search page | Y |
| `GET /api/v1/users/:id` | Profile page | Y |
| `GET /api/v1/universities` | Universities list | Y |
| `GET /api/v1/geo/countries` | Onboarding | N (hardcoded) |
| `GET /api/v1/trending/tags` | Onboarding, Sidebar | Y |

---

## Recommendations

1. **Immediate (P1):** Fix all Rule 16 violations by replacing hardcoded data with API calls
2. **Short-term (P2):** Fix i18n rendering in SSR and JSON-LD template syntax
3. **Medium-term (P3):** Add missing RSS feed links and improve test coverage
4. **Backend coordination:** Blog API returns 500 (MAIN-017 from existing tests) — requires backend fix

---

## Test Files Created

- `tests/e2e/ssr-integrity.spec.ts` — SSR integrity checks (updated playwright.config.ts to include)
