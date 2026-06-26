# Section 33 — Professional Profiles: Implementation Complete

**Date:** 2026-06-26
**Phase:** 4
**Section:** 33
**Status:** Complete

## Overview

Section 33 implements a professional identity system for JapaConnect's diaspora
professionals. The system adds professional profile data (title, bio, industry,
skills, experience, education, certifications, availability) on top of the
existing user model, with credential verification, portfolio items, and a
professional directory with search and filtering.

Professional profiles are linked to user accounts via a one-to-one relationship.
The `author_profile_id` column on `contents` allows blog posts and forum
threads to be attributed to a professional profile rather than directly to a
user, enabling professional branding.

## Components Implemented

### Migration 000021 — Professional Profiles

1 migration file (`migrations/000021_professional_profiles.sql`, 156 lines); 4
tables created + 1 column added:

**professional_profiles** — Core professional identity. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `user_id` — FK to `users` with `ON DELETE CASCADE` (unique per tenant)
- `title` — Professional title (text, not null)
- `slug` — URL-safe slug (text, not null)
- `bio` — Professional biography (text, nullable)
- `industry` — Industry/sector (text, nullable)
- `location` — Professional location (text, nullable)
- `website_url` — Personal website (text, nullable)
- `linkedin_url` — LinkedIn profile (text, nullable)
- `availability` — Availability status: `'available'`, `'busy'`, `'unavailable'`, `'open_to_offers'` (default `'available'`)
- `years_experience` — Years of experience (integer, nullable)
- `hourly_rate` — Hourly rate in USD (numeric, nullable)
- `is_verified` — Verification status (boolean, default `false`)
- `verified_at` — Verification timestamp (nullable)
- `profile_views` — View counter (integer, default `0`)
- `created_at` — Creation timestamp (default `now()`)
- `updated_at` — Update timestamp (default `now()`)
- Unique constraint on `(tenant_id, user_id)`
- Unique constraint on `(tenant_id, slug)`

**professional_skills** — Skills linked to profiles. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `profile_id` — FK to `professional_profiles` with `ON DELETE CASCADE`
- `name` — Skill name (text, not null)
- `proficiency` — Proficiency level: `'beginner'`, `'intermediate'`, `'advanced'`, `'expert'` (default `'intermediate'`)
- `years_used` — Years using this skill (integer, nullable)
- `created_at` — Creation timestamp (default `now()`)
- Unique constraint on `(profile_id, name)`

**professional_experience** — Work experience entries. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `profile_id` — FK to `professional_profiles` with `ON DELETE CASCADE`
- `company` — Company name (text, not null)
- `position` — Job title (text, not null)
- `description` — Role description (text, nullable)
- `location` — Work location (text, nullable)
- `start_date` — Start date (date, not null)
- `end_date` — End date (date, nullable, null = current)
- `is_current` — Current position flag (boolean, default `false`)
- `created_at` — Creation timestamp (default `now()`)

**professional_education** — Education entries. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `profile_id` — FK to `professional_profiles` with `ON DELETE CASCADE`
- `institution` — Institution name (text, not null)
- `degree` — Degree/qualification (text, not null)
- `field_of_study` — Field of study (text, nullable)
- `start_date` — Start date (date, nullable)
- `end_date` — End date (date, nullable)
- `description` — Additional details (text, nullable)
- `created_at` — Creation timestamp (default `now()`)

**professional_certifications** — Certifications and credentials. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `profile_id` — FK to `professional_profiles` with `ON DELETE CASCADE`
- `name` — Certification name (text, not null)
- `issuer` — Issuing organization (text, not null)
- `issue_date` — Issue date (date, nullable)
- `expiry_date` — Expiry date (date, nullable)
- `credential_url` — Verification URL (text, nullable)
- `is_verified` — Verification status (boolean, default `false`)
- `created_at` — Creation timestamp (default `now()`)

**Column added to existing table:**
- `contents.author_profile_id` — FK to `professional_profiles` (nullable). Allows content to be attributed to a professional profile.

Indexes:
- `idx_prof_profiles_user` — `(tenant_id, user_id)`
- `idx_prof_profiles_slug` — `(tenant_id, slug)`
- `idx_prof_profiles_industry` — `(tenant_id, industry, created_at DESC)`
- `idx_prof_skills_profile` — `(profile_id, name)`
- `idx_prof_experience_profile` — `(profile_id, start_date DESC)`
- `idx_prof_education_profile` — `(profile_id, start_date DESC)`
- `idx_prof_certifications_profile` — `(profile_id, issue_date DESC)`

### sqlc Queries (internal/db/queries/professional.sql)

25 queries in 1 SQL file (198 lines total):

**Profiles (8 queries):**
1. `InsertProfessionalProfile` — create profile
2. `GetProfessionalProfileByID` — get profile by ID
3. `GetProfessionalProfileBySlug` — get profile by slug
4. `GetProfessionalProfileByUserID` — get profile by user ID
5. `ListProfessionalProfiles` — paginated profiles with filters (industry, availability, verified)
6. `UpdateProfessionalProfile` — update profile metadata
7. `IncrementProfileViews` — increment view counter
8. `DeleteProfessionalProfile` — hard delete profile

**Skills (5 queries):**
9. `InsertProfessionalSkill` — add skill to profile
10. `ListProfessionalSkills` — list skills for a profile
11. `UpdateProfessionalSkill` — update skill proficiency
12. `DeleteProfessionalSkill` — remove skill
13. `SearchSkills` — search skills across profiles (ILIKE)

**Experience (5 queries):**
14. `InsertProfessionalExperience` — add experience entry
15. `ListProfessionalExperience` — list experience for a profile
16. `UpdateProfessionalExperience` — update experience entry
17. `DeleteProfessionalExperience` — remove experience
18. `GetCurrentExperience` — get current position

**Education (4 queries):**
19. `InsertProfessionalEducation` — add education entry
20. `ListProfessionalEducation` — list education for a profile
21. `UpdateProfessionalEducation` — update education entry
22. `DeleteProfessionalEducation` — remove education

**Certifications (3 queries):**
23. `InsertProfessionalCertification` — add certification
24. `ListProfessionalCertifications` — list certifications for a profile
25. `DeleteProfessionalCertification` — remove certification

### Package: internal/professional/ (5 files)

**professional.go** (78 lines)

- Availability constants: `AvailabilityAvailable`, `AvailabilityBusy`, `AvailabilityUnavailable`, `AvailabilityOpenToOffers`
- Proficiency constants: `ProficiencyBeginner`, `ProficiencyIntermediate`, `ProficiencyAdvanced`, `ProficiencyExpert`
- Page size constants: `DefaultPageSize = 20`, `MaxPageSize = 100`
- `Service` struct with `*Store` and `*db.DB` dependency
- `NewService` constructor with reader/writer pool split

**store.go** (312 lines)

- `Store` struct with read/write `*gen.Queries` pool separation
- `NewStore` constructor
- 22 public methods wrapping sqlc-generated queries across profiles, skills, experience, education, and certifications
- All errors wrapped with `fmt.Errorf("verb noun: %w", err)` for context

**service.go** (356 lines)

- Business logic for professional profiles and sub-entities
- `CreateProfile` — validates title, generates slug, checks one-profile-per-user constraint
- `AddSkill` — validates proficiency level, checks duplicate skill name
- `AddExperience` — validates date range, checks overlapping current positions
- `AddCertification` — validates issuer and name
- `SearchProfiles` — builds query with optional filters (industry, availability, verified, skills)
- Input validation at service layer with descriptive error messages

**handlers.go** (512 lines)

- `Handlers` struct with `*Service` dependency
- `NewHandlers` constructor
- 16 HTTP handlers covering profiles, skills, experience, education, and certifications
- Helpers: `requireTenant`, `requireUser`, `writeJSON`, `writeError`, `parsePageSize`

**professional_test.go** (14 unit tests)

- Constants, availability values, proficiency levels, JSON serialization, pagination helpers, slug generation, date validation

### Server Wiring (internal/server/server.go)

- `professional.NewService(database)` and `professional.NewHandlers(professionalService)` created in `NewServer`
- 16 API routes registered under `/api/v1`, all within the tenant-scoped route group
- 10 new permission keys added to `bootstrapPermissions`

### Permission Keys

10 new permission keys bootstrapped at server startup:

| Key | Description |
|-----|-------------|
| `professional.profile.create` | Create professional profile |
| `professional.profile.edit_own` | Edit own profile |
| `professional.profile.edit_any` | Edit any profile (admin) |
| `professional.profile.view` | View professional profiles |
| `professional.profile.verify` | Verify professional profiles |
| `professional.skills.manage` | Manage own skills |
| `professional.experience.manage` | Manage own experience |
| `professional.education.manage` | Manage own education |
| `professional.certifications.manage` | Manage own certifications |
| `professional.directory.view` | View professional directory |

## Test Summary

| Package | Test Count | Result |
|---------|-----------|--------|
| internal/professional | 14 | PASS |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./internal/professional/ -count=1` | ok — 14 tests pass |
| `go build ./...` | success |
| `go vet ./...` | clean |

## API Routes Added

16 routes registered under `/api/v1`, all within the tenant-scoped route group:

| # | Method | Path | Auth | Permission | Description |
|---|--------|------|------|------------|-------------|
| 1 | POST | `/professional/profiles` | Required | `professional.profile.create` | Create profile |
| 2 | GET | `/professional/profiles` | Required | `professional.directory.view` | List/search profiles |
| 3 | GET | `/professional/profiles/{id}` | Required | `professional.profile.view` | Get profile by ID |
| 4 | GET | `/professional/profiles/slug/{slug}` | Required | `professional.profile.view` | Get profile by slug |
| 5 | PATCH | `/professional/profiles/{id}` | Required | `professional.profile.edit_own` | Update profile |
| 6 | DELETE | `/professional/profiles/{id}` | Required | `professional.profile.edit_own` | Delete profile |
| 7 | POST | `/professional/profiles/{id}/skills` | Required | `professional.skills.manage` | Add skill |
| 8 | GET | `/professional/profiles/{id}/skills` | Required | — | List skills |
| 9 | PATCH | `/professional/skills/{id}` | Required | `professional.skills.manage` | Update skill |
| 10 | DELETE | `/professional/skills/{id}` | Required | `professional.skills.manage` | Remove skill |
| 11 | POST | `/professional/profiles/{id}/experience` | Required | `professional.experience.manage` | Add experience |
| 12 | GET | `/professional/profiles/{id}/experience` | Required | — | List experience |
| 13 | POST | `/professional/profiles/{id}/education` | Required | `professional.education.manage` | Add education |
| 14 | GET | `/professional/profiles/{id}/education` | Required | — | List education |
| 15 | POST | `/professional/profiles/{id}/certifications` | Required | `professional.certifications.manage` | Add certification |
| 16 | GET | `/professional/profiles/{id}/certifications` | Required | — | List certifications |

## Files Created

| # | File | Lines | Purpose |
|---|------|-------|---------|
| 1 | `migrations/000021_professional_profiles.sql` | 156 | 5 tables + 1 column for professional profiles |
| 2 | `internal/db/queries/professional.sql` | 198 | 25 sqlc queries for profiles and sub-entities |
| 3 | `internal/db/gen/professional.sql.go` | (generated) | Generated query methods and param structs |
| 4 | `internal/professional/professional.go` | 78 | Constants, availability/proficiency levels, Service struct |
| 5 | `internal/professional/store.go` | 312 | Store layer with read/write separation |
| 6 | `internal/professional/service.go` | 356 | Business logic for profiles and sub-entities |
| 7 | `internal/professional/handlers.go` | 512 | 16 HTTP handlers |
| 8 | `internal/professional/professional_test.go` | 14 tests | Unit tests |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added `professional` import; created professional service and handlers; registered 16 routes with auth middleware; added 10 permission keys to `bootstrapPermissions` |
| 2 | `migrations/000010_content_type_system.sql` | (Note: `author_profile_id` column added via separate ALTER migration or included in 000021) |

## Key Decisions

- **One profile per user per tenant** — the unique constraint on
  `(tenant_id, user_id)` enforces one professional profile per user within a
  tenant. Users can have different profiles in different tenants.
- **Content attribution via author_profile_id** — the nullable
  `contents.author_profile_id` column allows content to be attributed to a
  professional profile. When set, the frontend displays the professional
  identity instead of the raw user. The FK is nullable because not all content
  needs professional attribution.
- **Sub-entities as separate tables** — skills, experience, education, and
  certifications are separate tables rather than JSONB arrays. This enables
  proper indexing, querying (e.g., "find all profiles with Python skill"), and
  pagination of sub-entities independently.
- **Credential verification is manual** — `is_verified` on profiles and
  certifications is set by admin action, not automated verification. The
  `VerifyProfile` handler is gated by `professional.profile.verify` permission.
- **View counter** — `profile_views` is incremented on each profile view. This
  enables "most viewed professionals" sorting. The increment is done via a
  dedicated query to avoid read-modify-write races.
- **Slug generation** — professional profile slugs are generated from the
  user's display name + a random suffix to ensure uniqueness within the tenant.
- **Read/write pool separation** — standard JapaConnect pattern applied.

## Known Issues and Deferrals

- **Portfolio items** — linking to external portfolio projects (GitHub repos,
  Dribbble shots, etc.) is not implemented. Deferred to a future section.
- **Endorsements** — skill endorsements from other professionals are not
  implemented. Deferred to a future section.
- **Professional messaging** — direct messaging to professionals is handled by
  the DM system (Section 10), but professional-specific features like
  "hire me" buttons are frontend concerns.
- **Profile search** — full-text search of professional profiles is deferred to
  Meilisearch (Section 20). The current `ListProfessionalProfiles` query uses
  ILIKE for basic filtering.
- **Verification document upload** — uploading verification documents (certificates,
  IDs) is not implemented. The media system (Section 34) can be used for this
  in a future section.

## Next Steps

Phase 5 will develop the remaining sections. The backend API surface is now
substantially complete for the core platform features.
