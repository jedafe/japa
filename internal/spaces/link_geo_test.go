package spaces

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// linkTestService returns a Service backed by a real Postgres. Skipped unless
// JC_DATABASE_URL is set, so it never runs in CI without a database.
func linkTestService(t *testing.T) (*Service, *db.DB) {
	t.Helper()

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

	return NewService(database), database
}

// TestLinkSpaceToPrimarySpaces_GeoScoped asserts that a geo_code anchors
// parent resolution to that country's subtree: the country is always linked,
// and no parent may resolve to a different country. Regression test for the
// bug where unscoped keyword matching linked "Tbilisi Institute of Asia and
// Africa" to ~98 countries via continent tokens ("asia", "africa").
func TestLinkSpaceToPrimarySpaces_GeoScoped(t *testing.T) {
	svc, database := linkTestService(t)
	ctx := t.Context()
	q := gen.New(database.Reader)

	cases := []struct {
		name    string
		geoCode string
	}{
		{"Tbilisi Institute of Asia and Africa", "GE"},
		{"South-East Asia University", "TH"},
		{"North Island College", "CA"},
		{"Central European University", "HU"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			country, err := q.GetSpaceByGeoCode(ctx, &tc.geoCode)
			if err != nil || !country.ID.Valid {
				t.Skipf("no country space for geo_code %s; seed data missing", tc.geoCode)
			}

			primary, parentIDs, err := svc.LinkSpaceToPrimarySpaces(ctx, pgtype.UUID{}, tc.name, tc.geoCode)
			if err != nil {
				t.Fatalf("LinkSpaceToPrimarySpaces: %v", err)
			}
			if len(parentIDs) == 0 {
				t.Fatalf("expected at least the geo_code country as parent")
			}

			// Country itself must be in the set.
			foundCountry := false
			for _, pid := range parentIDs {
				if pid == country.ID {
					foundCountry = true
				}
			}
			if !foundCountry {
				t.Fatalf("geo_code country %s not in parent set", tc.geoCode)
			}

			// Every parent must resolve its country ancestor to the geo_code
			// country — no foreign-country edges.
			anc, err := q.GetSpaceCountryAncestorIDs(ctx, parentIDs)
			if err != nil {
				t.Fatalf("GetSpaceCountryAncestorIDs: %v", err)
			}
			bySpace := map[pgtype.UUID]pgtype.UUID{}
			for _, r := range anc {
				bySpace[r.SpaceID] = r.CountryID
			}
			for _, pid := range parentIDs {
				c, ok := bySpace[pid]
				if !ok {
					t.Fatalf("parent %v has no country ancestor (non-geo edge)", pid)
				}
				if c != country.ID {
					t.Fatalf("parent %v resolves to a different country than geo_code %s", pid, tc.geoCode)
				}
			}

			// Primary must also be in-country.
			if pc, ok := bySpace[primary]; !ok || pc != country.ID {
				t.Fatalf("primary parent %v not inside geo_code country", primary)
			}
		})
	}
}

// TestLinkSpaceToPrimarySpaces_NoGeoCode keeps the legacy unscoped fallback
// working for parentless custom spaces (§05): a name containing a country
// keyword must still link at least that country.
func TestLinkSpaceToPrimarySpaces_NoGeoCode(t *testing.T) {
	svc, _ := linkTestService(t)
	ctx := t.Context()

	primary, parentIDs, err := svc.LinkSpaceToPrimarySpaces(ctx, pgtype.UUID{}, "Nigeria Workers in Brazil", "")
	if err != nil {
		t.Fatalf("LinkSpaceToPrimarySpaces: %v", err)
	}
	if len(parentIDs) == 0 || !primary.Valid {
		t.Fatalf("expected fallback matcher to find primary parents")
	}
}
