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
