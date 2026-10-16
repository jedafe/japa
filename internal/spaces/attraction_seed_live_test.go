package spaces

// Ref: .jeos/sections/80-destination-wiki.md — Wiki-Only Space Types &
// Attractions amendment. Live-DB smoke test for the full seed path:
// stubbed SPARQL → candidate parse → P131 multi-parent resolution →
// space create + junction links → dedup on re-run. Gated on
// JC_SEED_LIVE=1 alongside JC_DATABASE_URL so it never writes to a dev
// database accidentally — all rows it creates are deleted on teardown.

import (
	"strconv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

func TestSeedAttractions_LiveDB(t *testing.T) {
	if os.Getenv("JC_SEED_LIVE") != "1" {
		t.Skip("set JC_SEED_LIVE=1 (with JC_DATABASE_URL) to run the live seed test")
	}

	svc, database := linkTestService(t)
	ctx := t.Context()
	// t.Context() is cancelled before Cleanup funcs run — cleanup deletes
	// need a live context or they silently no-op.
	bg := t.Context()
	q := gen.New(database.Writer)
	a := NewProcessorAdapter(svc)

	countryType, err := q.GetSpaceTypeBySlug(ctx, "country")
	if err != nil {
		t.Fatalf("country type: %v", err)
	}

	stateType, err := q.GetSpaceTypeBySlug(ctx, "state")
	if err != nil {
		t.Fatalf("state type: %v", err)
	}

	cityType, err := q.GetSpaceTypeBySlug(ctx, "city")
	if err != nil {
		t.Fatalf("city type: %v", err)
	}

	attractionType, err := q.GetSpaceTypeBySlug(ctx, "attraction")
	if err != nil {
		t.Fatalf("attraction type: %v", err)
	}

	uniType, err := q.GetSpaceTypeBySlug(ctx, "university")
	if err != nil {
		t.Fatalf("university type: %v", err)
	}

	hash := "x"

	creatorID, err := q.CreateUser(ctx, gen.CreateUserParams{
		Email:        fmt.Sprintf("seed-live-%d@example.invalid", time.Now().UnixNano()),
		Username:     fmt.Sprintf("seed-live-%d", time.Now().UnixNano()),
		PasswordHash: &hash,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = database.Writer.Exec(bg, `DELETE FROM users WHERE id = $1`, creatorID)
	})
	// The seeder creates spaces owned by systemUserID — a real user is
	// required (space_members.user_id is NOT NULL). Production wires the
	// seeded system account; the test uses its own creator.
	a.SetSystemUserID(creatorID)

	uniq := strconv.FormatInt(time.Now().UnixNano()%100000, 10)
	stamp := func(qid string) []byte {
		raw, _ := json.Marshal(map[string]string{"wikidata_id": qid + uniq})
		return raw
	}
	mkSpace := func(name, slug string, typeID pgtype.UUID, qid string) gen.CreateSpaceRow {
		t.Helper()

		sp, err := svc.CreateSpaceWithOwner(ctx, gen.CreateSpaceParams{
			SpaceTypeID: typeID,
			Name:        name,
			Slug:        slug + "-" + uniq,
			WikiContent: stamp(qid),
			Level:       "custom",
			Visibility:  VisibilityPublic,
			PostingPerm: PermAdmins,
			Status:      "active",
		}, creatorID, nil)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}

		t.Cleanup(func() {
			if _, err := database.Writer.Exec(bg, `DELETE FROM spaces WHERE id = $1`, sp.ID); err != nil {
				t.Logf("cleanup space %s: %v", sp.ID, err)
			}
		})

		return sp
	}

	// Stamped geo fixtures — what populateGeoSpace writes once it has run
	// for a space. The university carries a loc QID but must be filtered
	// out by the geo-type check.
	city := mkSpace("SeedLive City", "seedlive-city", cityType.ID, "QSL_CITY")
	state := mkSpace("SeedLive State", "seedlive-state", stateType.ID, "QSL_STATE")
	country := mkSpace("SeedLive Country", "seedlive-country", countryType.ID, "QSL_COUNTRY")
	mkSpace("SeedLive Uni", "seedlive-uni", uniType.ID, "QSL_UNI")

	// Stubbed Wikidata Query Service — two candidates; the first lists
	// city+state+country+uni P131s, the second country only. QIDs carry
	// the run's uniq suffix so stale fixture rows from earlier runs can
	// never cross-match.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/sparql-results+json")
		fmt.Fprintf(w, `{"results":{"bindings":[
			{"item":{"value":"http://www.wikidata.org/entity/QSL_ATT1%[1]s"},
			 "itemLabel":{"value":"SeedLive Falls"},
			 "itemDescription":{"value":"waterfall fixture"},
			 "loc":{"value":"http://www.wikidata.org/entity/QSL_CITY%[1]s"}},
			{"item":{"value":"http://www.wikidata.org/entity/QSL_ATT1%[1]s"},
			 "itemLabel":{"value":"SeedLive Falls"},
			 "itemDescription":{"value":"waterfall fixture"},
			 "loc":{"value":"http://www.wikidata.org/entity/QSL_STATE%[1]s"}},
			{"item":{"value":"http://www.wikidata.org/entity/QSL_ATT1%[1]s"},
			 "itemLabel":{"value":"SeedLive Falls"},
			 "itemDescription":{"value":"waterfall fixture"},
			 "loc":{"value":"http://www.wikidata.org/entity/QSL_COUNTRY%[1]s"}},
			{"item":{"value":"http://www.wikidata.org/entity/QSL_ATT1%[1]s"},
			 "itemLabel":{"value":"SeedLive Falls"},
			 "itemDescription":{"value":"waterfall fixture"},
			 "loc":{"value":"http://www.wikidata.org/entity/QSL_UNI%[1]s"}},
			{"item":{"value":"http://www.wikidata.org/entity/QSL_ATT2%[1]s"},
			 "itemLabel":{"value":"SeedLive Museum"},
			 "itemDescription":{"value":"museum fixture"},
			 "loc":{"value":"http://www.wikidata.org/entity/QSL_COUNTRY%[1]s"}}
		]}}`, uniq)
	}))
	defer srv.Close()

	old := wikidataSPARQLEndpoint
	wikidataSPARQLEndpoint = srv.URL

	defer func() { wikidataSPARQLEndpoint = old }()

	res, err := a.seedAttractions(ctx, city.ID, 10, false)
	if err != nil {
		t.Fatalf("seedAttractions: %v", err)
	}

	if res.Created != 2 || res.Failed != 0 {
		t.Fatalf("seed result: %+v", res)
	}

	if len(res.Slugs) != 2 {
		t.Fatalf("slugs: %+v", res.Slugs)
	}

	// Resolve the created spaces via the unique fixture parent's junction
	// — slug lookups could hit stale rows with the same name.
	children, err := svc.GetChildSpaces(ctx, city.ID)
	if err != nil {
		t.Fatalf("children of fixture city: %v", err)
	}

	var falls, museum gen.GetChildSpacesByParentIDRow

	for _, ch := range children {
		switch wikiContentWikidataID(ch.WikiContent) {
		case "QSL_ATT1" + uniq:
			falls = ch
		case "QSL_ATT2" + uniq:
			museum = ch
		}

		t.Cleanup(func() {
			if _, err := database.Writer.Exec(bg, `DELETE FROM spaces WHERE id = $1`, ch.ID); err != nil {
				t.Logf("cleanup child %s: %v", ch.ID, err)
			}
		})
	}

	if !falls.ID.Valid || !museum.ID.Valid {
		t.Fatalf("created spaces not found under city: %+v", children)
	}

	if falls.SpaceTypeID != attractionType.ID {
		t.Fatalf("falls space_type wrong: %s", falls.SpaceTypeID)
	}

	if falls.PostingPerm != PermAdmins || falls.JoiningPerm != JoiningInvite {
		t.Fatalf("wiki-only axes not coerced on seeded space: %s %s", falls.PostingPerm, falls.JoiningPerm)
	}

	parentsOf := func(id pgtype.UUID) map[pgtype.UUID]bool {
		t.Helper()

		parents, err := svc.GetParentSpaces(ctx, id)
		if err != nil {
			t.Fatalf("parents: %v", err)
		}

		have := map[pgtype.UUID]bool{}
		for _, p := range parents {
			have[p.ID] = true

			t.Logf("parent: %s (%s)", p.Name, p.ID)
		}

		return have
	}

	// Falls: trigger city + state + country via P131; the stamped
	// university must be filtered out.
	fp := parentsOf(falls.ID)
	if !fp[city.ID] || !fp[state.ID] || !fp[country.ID] {
		t.Fatalf("falls parents missing geo links: %+v", fp)
	}

	if len(fp) != 3 {
		t.Fatalf("falls parent count %d, want 3 (uni filtered)", len(fp))
	}

	// Museum: trigger city + country via P131.
	mp := parentsOf(museum.ID)
	if !mp[city.ID] || !mp[country.ID] || len(mp) != 2 {
		t.Fatalf("museum parents wrong: %+v", mp)
	}

	// Idempotent re-run: both candidates dedup via the wiki_content stamp.
	res2, err := a.seedAttractions(ctx, city.ID, 10, false)
	if err != nil {
		t.Fatalf("re-seed: %v", err)
	}

	if res2.Created != 0 || res2.SkippedDuplicate != 2 {
		t.Fatalf("re-seed result: %+v", res2)
	}
}

// TestSeedAttractions_LiveUpstream runs the seeder against the real
// Wikidata Query Service — the same code path the wiki:seed_attractions
// worker executes — creating real attraction spaces under a stamped geo
// parent, then removing them on teardown. Gated on JC_SEED_UPSTREAM=1
// plus JC_DATABASE_URL; the parent space must already carry a
// wikidata_id stamp (populateGeoSpace does this) so the parent lookup
// does not depend on the rate-limited wbsearchentities endpoint.
func TestSeedAttractions_LiveUpstream(t *testing.T) {
	if os.Getenv("JC_SEED_UPSTREAM") != "1" {
		t.Skip("set JC_SEED_UPSTREAM=1 (with JC_DATABASE_URL) to run the live upstream seed test")
	}

	svc, database := linkTestService(t)
	ctx := t.Context()
	bg := t.Context()
	q := gen.New(database.Writer)
	a := NewProcessorAdapter(svc)

	hash := "x"

	creatorID, err := q.CreateUser(ctx, gen.CreateUserParams{
		Email:        "seedup-test@example.invalid",
		Username:     "seedup-test",
		PasswordHash: &hash,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = database.Writer.Exec(bg, `DELETE FROM users WHERE id = $1`, creatorID)
	})
	a.SetSystemUserID(creatorID)

	parent, err := svc.GetSpaceBySlug(ctx, "abeokuta")
	if err != nil {
		t.Skipf("abeokuta space missing: %v", err)
	}

	if stamp := wikiContentWikidataID(parent.WikiContent); stamp == "" {
		t.Skip("abeokuta is not wikidata-stamped yet")
	}

	res, err := a.seedAttractions(ctx, parent.ID, 5, false)
	if err != nil {
		t.Fatalf("seedAttractions: %v", err)
	}

	t.Logf("seed result: %+v slugs=%v", res, res.Slugs)

	if res.Created == 0 {
		t.Fatalf("no attractions created: %+v", res)
	}

	children, err := svc.GetChildSpaces(ctx, parent.ID)
	if err != nil {
		t.Fatalf("children: %v", err)
	}

	attractionType, _ := q.GetSpaceTypeBySlug(ctx, "attraction")
	seen := 0

	for _, ch := range children {
		if ch.SpaceTypeID != attractionType.ID {
			continue
		}

		seen++

		t.Cleanup(func() {
			if _, err := database.Writer.Exec(bg, `DELETE FROM spaces WHERE id = $1`, ch.ID); err != nil {
				t.Logf("cleanup %s: %v", ch.ID, err)
			}
		})

		if wikiContentWikidataID(ch.WikiContent) == "" {
			t.Errorf("child %s missing wikidata_id stamp", ch.Slug)
		}

		if ch.PostingPerm != PermAdmins || ch.JoiningPerm != JoiningInvite {
			t.Errorf("child %s axes not coerced: %s %s", ch.Slug, ch.PostingPerm, ch.JoiningPerm)
		}

		t.Logf("created attraction %s qid=%s", ch.Slug, wikiContentWikidataID(ch.WikiContent))
	}

	if seen != res.Created {
		t.Fatalf("created=%d but %d attraction children under parent", res.Created, seen)
	}
}
