# Persona Geo Spaces Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give AI personas a geo space like real user profiles — `space_members` rows for the geo chain plus `users.current_geo_space_id` — assignable at provisioning, replaceable via PATCH, and pickable in the admin UI.

**Architecture:** A `geo_space` field (space UUID or slug) on `PersonaInput`/`UpdatePersonaInput` resolves through the existing `GetSpaceBySlug` query (which already accepts either form and excludes soft-deleted rows). The persona's backing bot user joins the space + its geo-level ancestors (`ListSpaceAncestors` + `AddSpaceMember`) and `UpdateUserGeoSpace` sets `current_geo_space_id` — mirroring onboarding's `autoJoinGeoSpaces`. Updates use replace semantics via a new `RemoveUserGeoMembershipsExcept` query. All writes ride the existing all-or-nothing provisioning/update transactions.

**Tech Stack:** Go 1.x + pgx/v5 + sqlc (`make sqlc-gen`), chi, SvelteKit 2 + Svelte 5 runes + bun.

## Global Constraints

- **JEOS Rule 2:** never hand-edit `internal/db/gen/*` — edit `internal/db/queries/*.sql`, then run `make sqlc-gen`.
- **JEOS Rule 5:** error codes are `snake_case`; validation failures wrap `errpkg.ErrInvalid` → HTTP 400 `validation_error`.
- **JEOS Rule 17:** no AI/agent attribution in commits, comments, or docs.
- **JEOS Rule 18:** frontend tooling uses `bun`/`bunx` only (never npm/npx/pnpm/yarn).
- Commits go to `main`, conventional-commit style (`feat(personas): …`), matching `git log` history.
- Spec: `docs/superpowers/specs/2026-11-02-persona-geo-spaces-design.md`. JEOS: `.jeos/sections/68-ai-personas.md` § "Geo Space Assignment" (already committed).
- Geo level gate is `level ∈ {country, state, city}` — mirrors `users.Store.isValidGeoSpace` (`internal/users/store.go:133`).

---

### Task 1: sqlc — geo joins on persona reads + RemoveUserGeoMembershipsExcept

**Files:**
- Modify: `internal/db/queries/personas.sql` (GetAIPersonaByID ~line 36, ListAIPersonas ~line 69; append new query at end)
- Regenerates: `internal/db/gen/personas.sql.go` (never hand-edit — Rule 2)

**Interfaces:**
- Produces: `gen.GetAIPersonaByIDRow` and `gen.ListAIPersonasRow` gain `GeoSpaceID pgtype.UUID`, `GeoSpaceName *string`, `GeoSpaceSlug *string`, `GeoSpaceLevel *string`. `gen.RemoveUserGeoMembershipsExcept(ctx, gen.RemoveUserGeoMembershipsExceptParams{UserID pgtype.UUID, KeepIds []pgtype.UUID}) error`. Task 2/3 consume these.

- [ ] **Step 1: Edit `GetAIPersonaByID` in `internal/db/queries/personas.sql`**

Add geo columns to the SELECT list and the join:

```sql
-- name: GetAIPersonaByID :one
-- Returns the persona with its bot user byline (from users + user_profiles).
SELECT
 ap.id,
 ap.user_id,
 ap.profile_id,
 ap.disclosure_mode,
 ap.autonomy_mode,
 ap.voice_id,
 ap.voice_prompt AS voice_prompt_override,
 COALESCE(ap.voice_prompt, pv.voice_prompt, '') AS voice_prompt,
 ap.activity_policy,
 ap.status,
 ap.created_by,
 ap.created_at,
 ap.updated_at,
 u.username AS backing_username,
 u.email AS backing_email,
 COALESCE(up.display_name, u.username) AS profile_name,
 u.username AS profile_slug,
 up.bio AS profile_tagline,
 pv.name AS voice_name,
 pv.slug AS voice_slug,
 gs.id AS geo_space_id,
 gs.name AS geo_space_name,
 gs.slug AS geo_space_slug,
 gs.level AS geo_space_level
FROM ai_personas ap
JOIN users u ON u.id = ap.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
LEFT JOIN persona_voices pv ON pv.id = ap.voice_id
LEFT JOIN spaces gs ON gs.id = u.current_geo_space_id AND gs.deleted_at IS NULL
WHERE ap.id = @id;
```

- [ ] **Step 2: Edit `ListAIPersonas` — same columns + join**

```sql
 pv.name AS voice_name,
 stats.total_actions AS total_actions,
 stats.last_action_at AS last_action_at,
 gs.id AS geo_space_id,
 gs.name AS geo_space_name,
 gs.slug AS geo_space_slug,
 gs.level AS geo_space_level
FROM ai_personas ap
JOIN users u ON u.id = ap.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
LEFT JOIN persona_voices pv ON pv.id = ap.voice_id
LEFT JOIN spaces gs ON gs.id = u.current_geo_space_id AND gs.deleted_at IS NULL
LEFT JOIN LATERAL ( ...unchanged...
```

- [ ] **Step 3: Append the replace-semantics delete to `internal/db/queries/personas.sql`**

```sql
-- =====================================================================
-- Persona geo replace (§68 Geo Space Assignment): drops the user's
-- memberships in geo-level spaces (country/state/city) that are not in the
-- keep list. An empty keep list clears all geo memberships (PATCH
-- geo_space:"" clear path). space_members.member_count self-corrects via
-- the trigger from migration 000967.
-- =====================================================================
-- name: RemoveUserGeoMembershipsExcept :exec
DELETE FROM space_members sm
USING spaces s
WHERE sm.space_id = s.id
  AND sm.user_id = @user_id
  AND s.level IN ('country','state','city')
  AND s.id <> ALL(@keep_ids::uuid[]);
```

- [ ] **Step 4: Regenerate sqlc and build**

```bash
make sqlc-gen && go build ./internal/db/gen ./internal/personas
```

Expected: build fails inside `internal/personas` is OK only if unrelated — `db/gen` must build clean. If `personas` package fails because `personaDetailFromRow` doesn't scan new fields yet, that's fine — it compiles regardless (new struct fields are optional). Verify `GeoSpaceID`/`KeepIds` names in generated code:

```bash
grep -n "GeoSpaceID\|KeepIds" internal/db/gen/personas.sql.go | head
```

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries/personas.sql internal/db/gen/personas.sql.go
git commit -m "feat(personas): geo space joins on persona reads + geo membership replace query"
```

---

### Task 2: Service — geo helpers, input field, DTO fields, provision wiring

**Files:**
- Create: `internal/personas/geo.go`
- Modify: `internal/personas/service.go` (PersonaInput ~line 61, PersonaDetail ~line 85, PersonaListItem ~line 112, ProvisionPersonas validation ~line 211 and tx loop ~line 305, mappers ~lines 958/985)

**Interfaces:**
- Consumes: `gen.GetSpaceBySlug` (accepts slug OR UUID, excludes `deleted_at`), `gen.ListSpaceAncestors`, `gen.AddSpaceMember`, `gen.UpdateUserGeoSpace`, `gen.RemoveUserGeoMembershipsExcept` (Task 1).
- Produces: `PersonaInput.GeoSpace string`, `PersonaDetail{GeoSpaceID, GeoSpaceName, GeoSpaceSlug, GeoSpaceLevel}`, `PersonaListItem` same four fields; `s.resolveGeoSpace`, `geoChain`, `s.applyGeoSpace` — Task 3 uses all of them.

- [ ] **Step 1: Create `internal/personas/geo.go`**

```go
// Package personas — geo.go implements §68 Geo Space Assignment: resolving a
// geo space ref (UUID or slug), expanding it to its geo ancestor chain, and
// joining a persona's backing user to that chain so the persona profile
// carries a geo space like a real profile (§05, ADR-064a).
// // Ref: japaconnect-engineering/sections/68-ai-personas.md (Geo Space Assignment)
package personas

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// isGeoSpaceLevel mirrors users.Store.isValidGeoSpace — only the geo
// hierarchy levels can anchor a persona's location.
func isGeoSpaceLevel(level string) bool {
	switch level {
	case "country", "state", "city":
		return true
	default:
		return false
	}
}

// resolveGeoSpace resolves a space ref (UUID or slug — GetSpaceBySlug accepts
// both and already excludes soft-deleted rows) to an active geo space id.
// Failures wrap ErrInvalid so callers surface 400 validation_error.
func (s *Service) resolveGeoSpace(ctx context.Context, q *gen.Queries, ref string) (pgtype.UUID, error) {
	space, err := q.GetSpaceBySlug(ctx, ref)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, fmt.Errorf("geo_space %q: space not found: %w", ref, errpkg.ErrInvalid)
		}
		return pgtype.UUID{}, fmt.Errorf("geo_space %q: lookup: %w", ref, err)
	}

	if space.Status != StatusActive {
		return pgtype.UUID{}, fmt.Errorf("geo_space %q: space is not active: %w", ref, errpkg.ErrInvalid)
	}

	if !isGeoSpaceLevel(space.Level) {
		return pgtype.UUID{}, fmt.Errorf("geo_space %q: not a geo space (country/state/city): %w", ref, errpkg.ErrInvalid)
	}

	return space.ID, nil
}

// geoChain returns spaceID plus the ids of its geo-level ancestors
// (ListSpaceAncestors walks parent_id ∪ space_parent_spaces, ≤3 hops —
// the geo chain never exceeds city → state → country).
func geoChain(ctx context.Context, q *gen.Queries, spaceID pgtype.UUID) ([]pgtype.UUID, error) {
	chain := []pgtype.UUID{spaceID}

	ancestors, err := q.ListSpaceAncestors(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("list geo ancestors: %w", err)
	}

	for _, anc := range ancestors {
		if isGeoSpaceLevel(anc.Level) {
			chain = append(chain, anc.ID)
		}
	}

	return chain, nil
}

// applyGeoSpace joins userID to every space in the chain (idempotent via
// AddSpaceMember's ON CONFLICT) and sets users.current_geo_space_id to the
// chain head — the deepest level the admin selected. Mirrors onboarding's
// autoJoinGeoSpaces (§09 space auto-join) minus the form-field resolution.
func (s *Service) applyGeoSpace(ctx context.Context, q *gen.Queries, userID pgtype.UUID, chain []pgtype.UUID) error {
	for _, spaceID := range chain {
		if _, err := q.AddSpaceMember(ctx, gen.AddSpaceMemberParams{
			SpaceID: spaceID,
			UserID:  userID,
		}); err != nil {
			return fmt.Errorf("join geo space %s: %w", spaceID.String(), err)
		}
	}

	head := chain[0]
	if err := q.UpdateUserGeoSpace(ctx, gen.UpdateUserGeoSpaceParams{
		CurrentGeoSpaceID: head,
		ID:                userID,
	}); err != nil {
		return fmt.Errorf("set current geo space: %w", err)
	}

	return nil
}

// replaceGeoSpace applies PATCH replace semantics: memberships in geo-level
// spaces outside the new chain are deleted, then the new chain is joined and
// current_geo_space_id updated. An empty ref clears geo assignment entirely —
// all geo memberships are dropped and current_geo_space_id is set NULL.
func (s *Service) replaceGeoSpace(ctx context.Context, q *gen.Queries, userID pgtype.UUID, ref string) error {
	var chain []pgtype.UUID

	if ref != "" {
		spaceID, err := s.resolveGeoSpace(ctx, q, ref)
		if err != nil {
			return err
		}

		chain, err = geoChain(ctx, q, spaceID)
		if err != nil {
			return err
		}
	}

	keep := chain
	if keep == nil {
		keep = []pgtype.UUID{}
	}

	if err := q.RemoveUserGeoMembershipsExcept(ctx, gen.RemoveUserGeoMembershipsExceptParams{
		UserID:  userID,
		KeepIds: keep,
	}); err != nil {
		return fmt.Errorf("remove stale geo memberships: %w", err)
	}

	if chain == nil {
		// Clear path — NULL current geo space, no memberships to add.
		return q.UpdateUserGeoSpace(ctx, gen.UpdateUserGeoSpaceParams{
			CurrentGeoSpaceID: pgtype.UUID{},
			ID:                userID,
		})
	}

	return s.applyGeoSpace(ctx, q, userID, chain)
}
```

- [ ] **Step 2: Add `GeoSpace` to `PersonaInput` in `service.go`**

```go
// PersonaInput is a single persona in a mass-provisioning request.
type PersonaInput struct {
	Name           string         `json:"name"`
	Slug           string         `json:"slug"`
	Tagline        string         `json:"tagline"`
	VoiceID        string         `json:"voice_id"`
	VoicePrompt    string         `json:"voice_prompt"`
	DisclosureMode string         `json:"disclosure_mode"`
	AutonomyMode   string         `json:"autonomy_mode"`
	ActivityPolicy ActivityPolicy `json:"activity_policy"`
	TypeID         string         `json:"type_id"` // optional; defaults to "contributor"/"columnist" lookup
	GeoSpace       string         `json:"geo_space"` // optional space UUID or slug (§68 Geo Space Assignment)
}
```

- [ ] **Step 3: Add geo fields to `PersonaDetail` and `PersonaListItem`**

In both structs, after `ProfileTagline`/`RecentActivity` grouping, add:

```go
	GeoSpaceID    string  `json:"geo_space_id"`
	GeoSpaceName  *string `json:"geo_space_name,omitempty"`
	GeoSpaceSlug  *string `json:"geo_space_slug,omitempty"`
	GeoSpaceLevel *string `json:"geo_space_level,omitempty"`
```

- [ ] **Step 4: Resolve geo refs pre-tx in `ProvisionPersonas`**

After the existing slug pre-validation block (after `rows.Close()` / `taken` check, before `pool := s.store.DB().Writer`), add:

```go
	// Resolve geo_space refs up front so an unknown slug / non-geo space
	// fails fast as validation_error instead of a mid-tx
	// provisioning_failed (mirrors the slug pre-check above).
	resolvedGeo := make([]pgtype.UUID, len(inputs))
	for i, in := range inputs {
		if in.GeoSpace == "" {
			continue
		}

		spaceID, err := s.resolveGeoSpace(ctx, gen.New(s.store.DB().Reader), in.GeoSpace)
		if err != nil {
			return nil, fmt.Errorf("personas: input %d: %w", i, err)
		}

		resolvedGeo[i] = spaceID
	}
```

Then inside the tx loop, after the profile byline upsert (step 2, after the `UpsertUserProfile` block), add:

```go
		// 2b. Geo space assignment (§68): join the backing user to the geo
		// chain and set current_geo_space_id so the persona profile shows
		// Spaces + the primary badge like a real profile.
		if resolvedGeo[i].Valid {
			chain, err := geoChain(ctx, q, resolvedGeo[i])
			if err != nil {
				results[i] = ProvisionResult{Input: in, Error: err.Error()}
				return results, fmt.Errorf("personas: input %d: geo chain: %w", i, err)
			}

			if err := s.applyGeoSpace(ctx, q, user.ID, chain); err != nil {
				results[i] = ProvisionResult{Input: in, Error: err.Error()}
				return results, fmt.Errorf("personas: input %d: apply geo space: %w", i, err)
			}
		}
```

- [ ] **Step 5: Map the new fields in `personaDetailFromRow` and `personaListItemFromRow`**

Add to both struct literals:

```go
		GeoSpaceID:    uuidOrEmpty(r.GeoSpaceID),
		GeoSpaceName:  r.GeoSpaceName,
		GeoSpaceSlug:  r.GeoSpaceSlug,
		GeoSpaceLevel: r.GeoSpaceLevel,
```

- [ ] **Step 6: Build + vet**

```bash
go build ./internal/personas && go vet ./internal/personas
```

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add internal/personas/geo.go internal/personas/service.go
git commit -m "feat(personas): geo space resolution, chain join, and provision-time geo_space"
```

---

### Task 3: UpdatePersona replace semantics + PATCH handler field

**Files:**
- Modify: `internal/personas/service.go` (UpdatePersonaInput ~line 562, changed-fields map ~line 591, identityTouched ~line 682, tx body ~line 793)
- Modify: `internal/personas/handlers.go` (update body struct ~line 274, input mapping ~line 291)

**Interfaces:**
- Consumes: `s.replaceGeoSpace` (Task 2), `detail.UserID` from `GetAIPersonaByIDRow`.
- Produces: `UpdatePersonaInput.GeoSpace *string` — nil = unchanged, `""` = clear, value = set/replace. PATCH body field `geo_space`.

- [ ] **Step 1: Add `GeoSpace` to `UpdatePersonaInput`**

```go
	// Identity fields — land on the backing bot account, not ai_personas:
	Name     *string
	Tagline  *string
	Username *string
	// GeoSpace replaces the backing user's geo membership chain (§68 Geo
	// Space Assignment). Nil = unchanged, "" = clear, value = set/replace.
	GeoSpace *string
```

- [ ] **Step 2: Add `geo_space` to the audit changed-fields map**

```go
		"username":        in.Username != nil,
		"geo_space":       in.GeoSpace != nil,
```

- [ ] **Step 3: Route `geo_space` through the identity tx path**

Change:

```go
	identityTouched := in.Name != nil || in.Tagline != nil || in.Username != nil
```

to:

```go
	identityTouched := in.Name != nil || in.Tagline != nil || in.Username != nil || in.GeoSpace != nil
```

- [ ] **Step 4: Apply the geo replace inside the tx (after the byline block, before `tx.Commit`)**

```go
	if in.GeoSpace != nil {
		if err := s.replaceGeoSpace(ctx, q, detail.UserID, *in.GeoSpace); err != nil {
			return PersonaDetail{}, fmt.Errorf("personas: replace geo space: %w", err)
		}
	}
```

(`resolveGeoSpace` failures already wrap `errpkg.ErrInvalid`, so `errors.Is` in `handleServiceError` yields 400.)

- [ ] **Step 5: Add `geo_space` to the PATCH body + input mapping in `handlers.go`**

```go
	var body struct {
		// ...existing fields...
		Username       *string         `json:"username"`
		GeoSpace       *string         `json:"geo_space"`
	}
```

```go
	in := UpdatePersonaInput{
		// ...existing fields...
		Username:       body.Username,
		GeoSpace:       body.GeoSpace,
	}
```

- [ ] **Step 6: Build + vet + commit**

```bash
go build ./internal/personas && go vet ./internal/personas
git add internal/personas/service.go internal/personas/handlers.go
git commit -m "feat(personas): PATCH geo_space with replace semantics"
```

---

### Task 4: Backend tests

**Files:**
- Create: `internal/personas/geo_test.go`

**Interfaces:**
- Consumes: `isGeoSpaceLevel` (Task 2), `Service.ProvisionPersonas`/`UpdatePersona` (Tasks 2-3), `JC_DATABASE_URL` integration pattern from `internal/spaces/link_geo_test.go:13`.

- [ ] **Step 1: Write the tests**

```go
// Package personas — geo_test.go covers the geo-level gate and the
// provision/update geo assignment flows. The service-level test is a live
// integration test skipped unless JC_DATABASE_URL is set (same pattern as
// internal/spaces/link_geo_test.go).
package personas

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// TestIsGeoSpaceLevel verifies only the geo hierarchy levels pass the gate —
// mirrors users.Store.isValidGeoSpace (custom/university/global rejected).
func TestIsGeoSpaceLevel(t *testing.T) {
	for _, lvl := range []string{"country", "state", "city"} {
		if !isGeoSpaceLevel(lvl) {
			t.Errorf("level %q should be geo", lvl)
		}
	}
	for _, lvl := range []string{"custom", "university", "global", "", "COUNTRY"} {
		if isGeoSpaceLevel(lvl) {
			t.Errorf("level %q should not be geo", lvl)
		}
	}
}

// TestPersonaGeoSpace_Live provisions a persona with geo_space and asserts
// the backing user joined the geo chain and got current_geo_space_id; then
// PATCHes a different geo space and asserts replace semantics; then clears.
// Skipped unless JC_DATABASE_URL is set.
func TestPersonaGeoSpace_Live(t *testing.T) {
	dsn := os.Getenv("JC_DATABASE_URL")
	if dsn == "" {
		t.Skip("JC_DATABASE_URL not set; skipping integration test")
	}

	database, err := db.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Close()
		database.Reader.Close()
	})

	ctx := t.Context()

	svc := NewService(NewStore(database), nil, nil)

	var adminID pgtype.UUID
	if err := database.Reader.QueryRow(ctx,
		`SELECT id FROM users WHERE is_bot = false LIMIT 1`).Scan(&adminID); err != nil {
		t.Skipf("no admin user for created_by: %v", err)
	}

	// Pick two distinct seeded country spaces dynamically — hardcoded slugs
	// would be brittle against seed drift.
	var spaces []struct {
		ID   pgtype.UUID
		Slug string
	}
	rows, err := database.Reader.Query(ctx,
		`SELECT id, slug FROM spaces WHERE level = 'country' AND status = 'active'
		 AND deleted_at IS NULL ORDER BY name LIMIT 2`)
	if err != nil {
		t.Fatalf("list country spaces: %v", err)
	}
	for rows.Next() {
		var s struct {
			ID   pgtype.UUID
			Slug string
		}
		if err := rows.Scan(&s.ID, &s.Slug); err != nil {
			rows.Close()
			t.Fatalf("scan country: %v", err)
		}
		spaces = append(spaces, s)
	}
	rows.Close()
	if len(spaces) < 2 {
		t.Skip("fewer than 2 seeded country spaces")
	}
	country, other := spaces[0], spaces[1]

	slug := "geo-test-persona"
	// Clean up leftovers from a previous run (ai_personas cascades on
	// users delete; space_members / user_profiles cleaned explicitly).
	cleanup := func() {
		_, _ = database.Writer.Exec(ctx, `DELETE FROM space_members WHERE user_id IN (SELECT id FROM users WHERE username = $1)`, slug)
		_, _ = database.Writer.Exec(ctx, `DELETE FROM user_profiles WHERE user_id IN (SELECT id FROM users WHERE username = $1)`, slug)
		_, _ = database.Writer.Exec(ctx, `DELETE FROM users WHERE username = $1`, slug)
	}
	cleanup()
	t.Cleanup(cleanup)

	results, err := svc.ProvisionPersonas(ctx, adminID, []PersonaInput{{
		Name:           "Geo Test Persona",
		Slug:           slug,
		DisclosureMode: DisclosureModeUndisclosed,
		AutonomyMode:   AutonomyModeQueued,
		GeoSpace:       country.Slug,
	}})
	if err != nil {
		t.Fatalf("provision with geo_space: %v", err)
	}
	if results[0].ID == "" {
		t.Fatalf("provision failed: %s", results[0].Error)
	}

	// Backing user: current_geo_space_id set to the selected space.
	var personaID pgtype.UUID
	var userID pgtype.UUID
	var currentGeo pgtype.UUID
	if err := database.Reader.QueryRow(ctx,
		`SELECT ap.id, u.id, u.current_geo_space_id FROM ai_personas ap
		 JOIN users u ON u.id = ap.user_id WHERE u.username = $1`, slug).
		Scan(&personaID, &userID, &currentGeo); err != nil {
		t.Fatalf("fetch persona: %v", err)
	}
	if currentGeo != country.ID {
		t.Fatalf("current_geo_space_id = %v, want %v", currentGeo, country.ID)
	}

	// Membership exists in the selected space.
	var memberCount int
	if err := database.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM space_members WHERE space_id = $1 AND user_id = $2`,
		country.ID, userID).Scan(&memberCount); err != nil {
		t.Fatalf("count membership: %v", err)
	}
	if memberCount != 1 {
		t.Fatalf("geo membership count = %d, want 1", memberCount)
	}

	// PATCH replace: geo_space -> other country. The old geo membership
	// must be removed (replace semantics).
	newGeo := other.Slug
	if _, err := svc.UpdatePersona(ctx, personaID, UpdatePersonaInput{GeoSpace: &newGeo}, adminID); err != nil {
		t.Fatalf("update geo_space: %v", err)
	}
	if err := database.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM space_members sm JOIN spaces s ON s.id = sm.space_id
		 WHERE sm.user_id = $1 AND s.level IN ('country','state','city')`,
		userID).Scan(&memberCount); err != nil {
		t.Fatalf("count geo memberships: %v", err)
	}
	if memberCount != 1 {
		t.Fatalf("geo memberships after replace = %d, want 1", memberCount)
	}
	var sameGeo bool
	if err := database.Reader.QueryRow(ctx,
		`SELECT current_geo_space_id = $1 FROM users WHERE id = $2`,
		other.ID, userID).Scan(&sameGeo); err != nil {
		t.Fatalf("check current geo: %v", err)
	}
	if !sameGeo {
		t.Fatal("current_geo_space_id was not updated to the replacement space")
	}

	// PATCH clear: geo_space "" removes all geo memberships + NULLs current.
	empty := ""
	if _, err := svc.UpdatePersona(ctx, personaID, UpdatePersonaInput{GeoSpace: &empty}, adminID); err != nil {
		t.Fatalf("clear geo_space: %v", err)
	}
	if err := database.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM space_members sm JOIN spaces s ON s.id = sm.space_id
		 WHERE sm.user_id = $1 AND s.level IN ('country','state','city')`,
		userID).Scan(&memberCount); err != nil {
		t.Fatalf("count after clear: %v", err)
	}
	if memberCount != 0 {
		t.Fatalf("geo memberships after clear = %d, want 0", memberCount)
	}
	var geoNull bool
	if err := database.Reader.QueryRow(ctx,
		`SELECT current_geo_space_id IS NULL FROM users WHERE id = $1`,
		userID).Scan(&geoNull); err != nil {
		t.Fatalf("check cleared geo: %v", err)
	}
	if !geoNull {
		t.Fatal("current_geo_space_id was not cleared")
	}

	// Detail exposes the geo fields (geo_space_id "" after clear).
	detail, err := svc.GetPersona(ctx, personaID)
	if err != nil {
		t.Fatalf("get persona: %v", err)
	}
	if detail.GeoSpaceID != "" {
		t.Fatalf("geo_space_id = %q, want empty after clear", detail.GeoSpaceID)
	}
}
```

- [ ] **Step 2: Run unit test**

```bash
go test ./internal/personas/ -run TestIsGeoSpaceLevel -v
```

Expected: PASS.

- [ ] **Step 3: Run integration test if DB available**

```bash
go test ./internal/personas/ -run TestPersonaGeoSpace_Live -v
```

Expected: SKIP (no JC_DATABASE_URL) or PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/personas/geo_test.go
git commit -m "test(personas): geo space level gate + provision/update geo flows"
```

---

### Task 5: GeoSpaceCascade component (country → state → city picker)

**Files:**
- Create: `apps/admin/src/lib/components/ui/GeoSpaceCascade.svelte`

**Interfaces:**
- Consumes: `geoApi.listCountries() / listStates(code) / listCities(stateId)` (already in `apps/admin/src/lib/api.ts:1099`) — the same public `/geo/*` endpoints the real user location picker uses.
- Produces: `bind:value` = deepest selected space UUID (city → state → country fallback); `''` when nothing picked. Task 6 consumes it.
- Review note: `SpaceAutocomplete` was rejected for this use — `/admin/spaces` ignores `level` server-side and is gated by `spaces.admin`, which a `persona.manage` holder may lack. `/geo/*` is public.

GeoSpace item shape (from `projectGeoSpace` in `internal/geoip/handlers.go`): `{ id, name, slug, level, geo_code?, ... }`. `listCountries` returns `GeoCountry[]` — check its type def near api.ts:1099 and reuse if compatible, else type locally.

- [ ] **Step 1: Create the component**

```svelte
<script lang="ts">
	// GeoSpaceCascade — cascading country → state → city picker backed by the
	// public /geo/* endpoints (same source as the member location picker,
	// ADR-064a). `value` binds the deepest selected space UUID; '' = unset.
	import { geoApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type GeoSpace = { id: string; name: string; slug: string; level: string; geo_code?: string | null };

	let {
		value = $bindable<string>(''),
		id = 'geo-cascade'
	}: {
		value?: string;
		id?: string;
	} = $props();

	let countries = $state<GeoSpace[]>([]);
	let states = $state<GeoSpace[]>([]);
	let cities = $state<GeoSpace[]>([]);
	let countryId = $state('');
	let stateId = $state('');
	let cityId = $state('');
	let loading = $state(false);

	$effect(() => {
		(async () => {
			loading = true;
			try {
				const res = await geoApi.listCountries();
				if (res.ok) {
					const data = res.data as GeoSpace[] | { data?: GeoSpace[] };
					countries = Array.isArray(data) ? data : (data.data ?? []);
				}
			} finally {
				loading = false;
			}
		})();
	});

	async function pickCountry(idVal: string) {
		countryId = idVal;
		stateId = '';
		cityId = '';
		cities = [];
		value = idVal;
		if (!idVal) {
			states = [];
			return;
		}
		const res = await geoApi.listStates(geoCodeOf(idVal));
		if (res.ok) {
			const data = res.data as GeoSpace[] | { data?: GeoSpace[] };
			states = Array.isArray(data) ? data : (data.data ?? []);
		}
	}

	// listStates keys off the country's ISO geo_code, not its id.
	function geoCodeOf(idVal: string): string {
		return countries.find((c) => c.id === idVal)?.geo_code ?? '';
	}

	async function pickState(idVal: string) {
		stateId = idVal;
		cityId = '';
		value = idVal || countryId;
		if (!idVal) {
			cities = [];
			return;
		}
		const res = await geoApi.listCities(idVal);
		if (res.ok) {
			const data = res.data as GeoSpace[] | { data?: GeoSpace[] };
			cities = Array.isArray(data) ? data : (data.data ?? []);
		}
	}

	function pickCity(idVal: string) {
		cityId = idVal;
		value = idVal || stateId || countryId;
	}

	const selectCls =
		'w-full rounded-md border px-3 py-1.5 text-sm';
	const selectStyle =
		'border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)';
</script>

<div class="grid gap-3 sm:grid-cols-3">
	<select id={`${id}-country`} class={selectCls} style={selectStyle} value={countryId} onchange={(e) => pickCountry((e.target as HTMLSelectElement).value)}>
		<option value="">{t('personas.geo_country', 'Country')}</option>
		{#each countries as c (c.id)}
			<option value={c.id}>{c.name}</option>
		{/each}
	</select>
	<select id={`${id}-state`} class={selectCls} style={selectStyle} value={stateId} disabled={states.length === 0} onchange={(e) => pickState((e.target as HTMLSelectElement).value)}>
		<option value="">{t('personas.geo_state', 'State / Region')}</option>
		{#each states as s (s.id)}
			<option value={s.id}>{s.name}</option>
		{/each}
	</select>
	<select id={`${id}-city`} class={selectCls} style={selectStyle} value={cityId} disabled={cities.length === 0} onchange={(e) => pickCity((e.target as HTMLSelectElement).value)}>
		<option value="">{t('personas.geo_city', 'City')}</option>
		{#each cities as c (c.id)}
			<option value={c.id}>{c.name}</option>
		{/each}
	</select>
</div>
{#if loading}
	<p class="mt-1 text-[10px]" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
{/if}
```

- [ ] **Step 2: Typecheck + commit**

```bash
cd apps/admin && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | grep -i "geospacecascade" | head
git add apps/admin/src/lib/components/ui/GeoSpaceCascade.svelte
git commit -m "feat(admin): GeoSpaceCascade country/state/city picker via /geo endpoints"
```

---

### Task 6: Personas admin page — geo picker, column, i18n

**Files:**
- Modify: `apps/admin/src/routes/(app)/personas/+page.svelte`
- Modify: `apps/admin/src/lib/locales/en.json`, `apps/admin/src/lib/locales/fr.json` (add `personas.geo_space`, `personas.geo_space_hint`)

**Interfaces:**
- Consumes: `GeoSpaceCascade` `bind:value` (Task 5); backend `geo_space` field + `geo_space_*` response fields (Tasks 2-3).
- Produces: provision form sends `geo_space` (UUID); edit form sends `geo_space` only when changed (mirroring the name/tagline/username diff-send pattern); list shows a Geo column.

- [ ] **Step 1: Extend types and state**

```ts
	type Persona = {
		// ...existing fields...
		geo_space_id?: string;
		geo_space_name?: string | null;
		geo_space_slug?: string | null;
		geo_space_level?: string | null;
	};

	type PersonaInput = {
		// ...existing fields...
		geo_space?: string;
	};

	type UpdateInput = {
		// ...existing fields...
		geo_space?: string;
	};
```

Add import:

```ts
	import GeoSpaceCascade from '$lib/components/ui/GeoSpaceCascade.svelte';
```

`PersonaInput.geo_space` is bound directly to the cascade's `value` — no parallel selection state is needed (`bind:value` on an object property works in Svelte 5). For the edit form, keep a diff baseline:

```ts
	// Geo space for the edit cascade — the new selection diff-sends as
	// geo_space on save (mirrors the name/tagline/username pattern).
	let editGeoSpace = $state('');
```

- [ ] **Step 2: Send `geo_space` in `createPersonas` and reset**

`persona.geo_space` is already populated via the bind — no extra mapping needed in `createPersonas`. On success, `newPersonas = [newPersonaInput()]` already resets `geo_space` to `''` (add `geo_space: ''` to `newPersonaInput()`'s return object for a clean initial bind).

- [ ] **Step 3: Seed + diff-send in edit flow**

In both `openView` and `openEdit`, after building `editForm`:

```ts
		editGeoSpace = ''; // cascade starts unselected; current geo shows as text
```

In `submitEdit`, after the existing diff-send deletes:

```ts
			// Send geo_space only when the admin picked a new chain — the
			// cascade can't pre-seed ancestors, so untouched stays omitted.
			if (editGeoSpace !== '') body.geo_space = editGeoSpace;
```

- [ ] **Step 4: Picker in the create modal** (after the tagline `<div class="mt-2">`, before the disclosure/autonomy grid)

```svelte
					<div class="mt-2">
						<label for={`persona-geo-${idx}`} class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.geo_space', 'Geo Space')}</label>
						<GeoSpaceCascade id={`persona-geo-${idx}`} bind:value={persona.geo_space} />
					</div>
```

- [ ] **Step 5: Picker in the edit modal** (after the status block, before voice)

```svelte
			<div>
				<label for="edit-persona-geo" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('personas.geo_space', 'Geo Space')}</label>
				{#if editingPersona?.geo_space_name}
					<p class="mb-1 text-[10px]" style="color: var(--color-muted)">
						{t('personas.geo_current', 'Current')}: {editingPersona.geo_space_name}{editingPersona.geo_space_level ? ` · ${editingPersona.geo_space_level}` : ''}
					</p>
				{/if}
				<GeoSpaceCascade id="edit-persona-geo" bind:value={editGeoSpace} />
			</div>
```

- [ ] **Step 6: Geo column in `columns`** (insert before `total_actions`)

```ts
		{
			key: 'geo_space_name',
			label: t('personas.geo_space', 'Geo Space'),
			render: (value: string, row: Persona) =>
				`<span class="text-muted-foreground text-xs">${value ? `${value}${row.geo_space_level ? ` · ${row.geo_space_level}` : ''}` : '—'}</span>`
		},
```

- [ ] **Step 7: i18n keys**

en.json (alphabetical, after `personas.entity`):

```json
  "personas.geo_city": "City",
  "personas.geo_country": "Country",
  "personas.geo_current": "Current",
  "personas.geo_space": "Geo Space",
  "personas.geo_state": "State / Region",
```

fr.json — same keys, French values:

```json
  "personas.geo_city": "Ville",
  "personas.geo_country": "Pays",
  "personas.geo_current": "Actuel",
  "personas.geo_space": "Espace géographique",
  "personas.geo_state": "État / Région",
```

- [ ] **Step 9: Typecheck + build**

```bash
cd apps/admin && bun run check 2>&1 | tail -15
```

Expected: no new errors in `personas/+page.svelte` or `SpaceAutocomplete.svelte` (pre-existing errors elsewhere are out of scope).

- [ ] **Step 10: Commit**

```bash
git add "apps/admin/src/routes/(app)/personas/+page.svelte" apps/admin/src/lib/locales/en.json apps/admin/src/lib/locales/fr.json
git commit -m "feat(admin): geo space picker on persona create/edit + geo column"
```

---

## Self-Review Notes

- **Spec coverage:** provision geo_space (Task 2), PATCH set/replace/clear (Task 3), response fields (Tasks 1-2), admin UI picker + column (Tasks 5-6), tests (Task 4). Replace semantics via `RemoveUserGeoMembershipsExcept` (Task 1). All covered.
- **Type consistency:** `resolvedGeo []pgtype.UUID` — `pgtype` already imported in service.go. `KeepIds []pgtype.UUID` matches `uuid[]` param. `geo_space_id` emitted via `uuidOrEmpty` → `""` when unset, consistent with `voice_id`.
- **Edge cases:** `<> ALL('{}'::uuid[])` is TRUE for all rows → empty keep deletes all geo memberships (clear path verified in test). `AddSpaceMember` ON CONFLICT re-approves — re-setting the same geo space is idempotent.
