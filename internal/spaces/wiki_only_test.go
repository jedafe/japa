package spaces

// §80 Wiki-Only Space Types — pure unit coverage for the forced permission
// axes and the cross-package sentinel marker, plus DB-backed coverage for
// IsWikiOnlyType, axis coercion, and post denial.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

func TestWikiOnlyPermAxes(t *testing.T) {
	a := WikiOnlyPermAxes()
	if a.Visibility != VisibilityPublic ||
		a.Posting != PermAdmins || a.Replying != PermAdmins || a.Commenting != PermAdmins ||
		a.Joining != JoiningInvite {
		t.Fatalf("unexpected wiki-only axes: %+v", a)
	}
}

func TestErrWikiOnlySpace_Marker(t *testing.T) {
	var m interface{ WikiOnlySpace() bool }
	if !errors.As(ErrWikiOnlySpace, &m) {
		t.Fatal("ErrWikiOnlySpace must satisfy the WikiOnlySpace marker interface")
	}
}

// TestWikiOnlyType_Gating is an integration test over a real database: it
// creates a space type, flips wiki_only on it, and asserts that
//   - IsWikiOnlyType reports true (and false for a normal/invalid type),
//   - CreateSpaceWithOwner coerces permissive request axes to the
//     wiki-only axes,
//   - UpdateSpace coerces permissive axes on the stored space,
//   - CanPostInSpace denies even though posting was requested "open".
//
// The space/type/user rows are cleaned up on test teardown.
func TestWikiOnlyType_Gating(t *testing.T) {
	svc, database := linkTestService(t)
	ctx := t.Context()
	// t.Context() is cancelled before Cleanup funcs run — teardown
	// deletes need a live context or they silently no-op.
	bg := t.Context()
	q := gen.New(database.Writer)

	st, err := q.CreateSpaceType(ctx, gen.CreateSpaceTypeParams{
		Name:     "WikiOnly Test Type",
		Slug:     "wiki-only-test",
		IsSystem: false,
	})
	if err != nil {
		t.Fatalf("create space type: %v", err)
	}

	t.Cleanup(func() {
		_, _ = database.Writer.Exec(bg, `DELETE FROM space_types WHERE id = $1`, st.ID)
	})

	wikiOnly := true

	st, err = q.UpdateSpaceType(ctx, gen.UpdateSpaceTypeParams{
		Name:     st.Name,
		WikiOnly: &wikiOnly,
		ID:       st.ID,
	})
	if err != nil {
		t.Fatalf("set wiki_only: %v", err)
	}

	if !st.WikiOnly {
		t.Fatal("space type should be wiki_only after update")
	}

	if !svc.IsWikiOnlyType(ctx, st.ID) {
		t.Fatal("IsWikiOnlyType must return true for a wiki-only type")
	}

	if svc.IsWikiOnlyType(ctx, pgtype.UUID{}) {
		t.Fatal("IsWikiOnlyType must return false for an invalid type id")
	}

	// A creator user is required by CreateSpaceWithOwner.
	hash := "x"

	creatorID, err := q.CreateUser(ctx, gen.CreateUserParams{
		Email:        "wiki-only-test@example.invalid",
		Username:     "wiki-only-test",
		PasswordHash: &hash,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = database.Writer.Exec(bg, `DELETE FROM users WHERE id = $1`, creatorID)
	})

	space, err := svc.CreateSpaceWithOwner(ctx, gen.CreateSpaceParams{
		SpaceTypeID:    st.ID,
		Name:           "Wiki Only Space",
		Slug:           "wiki-only-space-test",
		Level:          "custom",
		Visibility:     VisibilityPublic,
		PostingPerm:    PermOpen, // deliberately permissive — must be coerced
		ReplyingPerm:   PermOpen,
		CommentingPerm: PermOpen,
		JoiningPerm:    JoiningOpen,
		Status:         "active",
	}, creatorID, nil)
	if err != nil {
		t.Fatalf("create space: %v", err)
	}

	t.Cleanup(func() {
		_, _ = database.Writer.Exec(bg, `DELETE FROM spaces WHERE id = $1`, space.ID)
	})

	if space.PostingPerm != PermAdmins || space.ReplyingPerm != PermAdmins ||
		space.CommentingPerm != PermAdmins || space.JoiningPerm != JoiningInvite {
		t.Fatalf("axes not coerced: posting=%s replying=%s commenting=%s joining=%s",
			space.PostingPerm, space.ReplyingPerm, space.CommentingPerm, space.JoiningPerm)
	}

	full, err := svc.GetSpace(ctx, space.ID)
	if err != nil {
		t.Fatalf("get space: %v", err)
	}

	if svc.CanPostInSpace(ctx, full, creatorID, nil, nil) {
		t.Fatal("CanPostInSpace must deny posts to a wiki-only space")
	}

	if svc.CanCommentInSpace(ctx, full, creatorID, nil, nil) {
		t.Fatal("CanCommentInSpace must deny comments on a wiki-only space")
	}

	if svc.CanReplyInSpace(ctx, full, creatorID, nil, nil) {
		t.Fatal("CanReplyInSpace must deny replies on a wiki-only space")
	}

	// UpdateSpace must also coerce permissive axes on a wiki-only space.
	updated, err := svc.UpdateSpace(ctx, gen.UpdateSpaceParams{
		ID:             space.ID,
		Name:           space.Name,
		Visibility:     VisibilityPublic,
		PostingPerm:    PermOpen,
		ReplyingPerm:   PermOpen,
		CommentingPerm: PermOpen,
		JoiningPerm:    JoiningOpen,
		Status:         "active",
	})
	if err != nil {
		t.Fatalf("update space: %v", err)
	}

	if updated.PostingPerm != PermAdmins || updated.JoiningPerm != JoiningInvite {
		t.Fatalf("update did not coerce axes: posting=%s joining=%s",
			updated.PostingPerm, updated.JoiningPerm)
	}
}

// ---------------------------------------------------------------------------
// §80 attraction auto-population helpers
// ---------------------------------------------------------------------------

func TestClassifyAttractionKind(t *testing.T) {
	cases := []struct {
		name string
		ent  *wikidataUniversity
		want string
	}{
		{"p31 museum", &wikidataUniversity{InstanceOf: []string{"Q33506"}}, "museum"},
		{"p31 waterfall", &wikidataUniversity{InstanceOf: []string{"Q34038"}}, "waterfall"},
		{"p31 national park", &wikidataUniversity{InstanceOf: []string{"Q46169"}}, "national-park"},
		{"p31 church→other", &wikidataUniversity{InstanceOf: []string{"Q16970"}}, "other"},
		{"desc mosque→other", &wikidataUniversity{Description: "grand mosque in Kano"}, "other"},
		{"desc cathedral→other", &wikidataUniversity{Description: "cathedral in Lagos"}, "other"},
		{"desc monolith", &wikidataUniversity{Description: "Inselberg and monolith in Abeokuta"}, "rock-formation"},
		{"desc falls", &wikidataUniversity{Description: "waterfall on the Zambezi"}, "waterfall"},
		{"desc mountain", &wikidataUniversity{Description: "Highest mountain in Africa"}, "mountain"},
		{"unknown→other", &wikidataUniversity{Description: "a place"}, "other"},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		if got := classifyAttractionKind(tc.ent); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestMarshalWikiContentWithQID(t *testing.T) {
	s := &wikipediaSummary{Title: "Victoria Falls", Extract: "a waterfall"}

	raw, err := marshalWikiContentWithQID(s, "Q38324")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if m["wikidata_id"] != "Q38324" || m["title"] != "Victoria Falls" {
		t.Fatalf("merged map wrong: %v", m)
	}

	// No QID → plain summary JSON, no key injected.
	raw, err = marshalWikiContentWithQID(s, "")
	if err != nil {
		t.Fatalf("marshal no qid: %v", err)
	}

	var m2 map[string]any
	if err := json.Unmarshal(raw, &m2); err != nil {
		t.Fatalf("unmarshal no qid: %v", err)
	}

	if _, ok := m2["wikidata_id"]; ok {
		t.Fatal("wikidata_id must not appear when no QID is known")
	}
}

func TestAttractionSeededQID_WikiContentFallback(t *testing.T) {
	a := &ProcessorAdapter{} // no customFields — exercises the wiki_content arm

	var space gen.GetSpaceByIDRow

	space.WikiContent = []byte(`{"wikidata_id":"Q38324","title":"Victoria Falls"}`)
	if got := a.attractionSeededQID(t.Context(), &space); got != "Q38324" {
		t.Fatalf("got %q want Q38324", got)
	}

	space.WikiContent = []byte(`{"title":"no qid"}`)
	if got := a.attractionSeededQID(t.Context(), &space); got != "" {
		t.Fatalf("got %q want empty", got)
	}

	space.WikiContent = nil
	if got := a.attractionSeededQID(t.Context(), &space); got != "" {
		t.Fatalf("nil wiki_content: got %q want empty", got)
	}
}

func TestClampAttractionSeedLimit(t *testing.T) {
	if got := clampAttractionSeedLimit(0); got != attractionSeedDefaultLimit {
		t.Fatalf("0 → %d, want %d", got, attractionSeedDefaultLimit)
	}

	if got := clampAttractionSeedLimit(9999); got != attractionSeedMaxLimit {
		t.Fatalf("9999 → %d, want %d", got, attractionSeedMaxLimit)
	}

	if got := clampAttractionSeedLimit(42); got != 42 {
		t.Fatalf("42 → %d", got)
	}
}

func TestFetchAttractionCandidates_ParsesBindings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") != "json" {
			t.Errorf("missing format=json")
		}

		w.Header().Set("Content-Type", "application/sparql-results+json")
		fmt.Fprint(w, `{"results":{"bindings":[
			{"item":{"value":"http://www.wikidata.org/entity/Q38324"},
			 "itemLabel":{"value":"Victoria Falls"},
			 "itemDescription":{"value":"waterfall on the Zambezi"},
			 "loc":{"value":"http://www.wikidata.org/entity/Q1111"}},
			{"item":{"value":"http://www.wikidata.org/entity/Q38324"},
			 "itemLabel":{"value":"Victoria Falls"},
			 "itemDescription":{"value":"waterfall on the Zambezi"},
			 "loc":{"value":"http://www.wikidata.org/entity/Q2222"}},
			{"item":{"value":"http://www.wikidata.org/entity/Q38324"},
			 "itemLabel":{"value":"Victoria Falls"},
			 "itemDescription":{"value":"dup row"},
			 "loc":{"value":"http://www.wikidata.org/entity/Q1111"}},
			{"item":{"value":"http://www.wikidata.org/entity/Q99999"},
			 "itemLabel":{"value":"Q99999"},
			 "itemDescription":{"value":"unlabeled — label service fallback"}},
			{"item":{"value":"http://www.wikidata.org/entity/Q13911"},
			 "itemLabel":{"value":"Olumo Rock"},
			 "itemDescription":{"value":"rock formation in Abeokuta"}},
			{"item":{"value":"http://www.wikidata.org/entity/Q77777"},
			 "itemLabel":{"value":"Grand Mosque of Testville"},
			 "itemDescription":{"value":"mosque and tourist attraction"}}
		]}}`)
	}))
	defer srv.Close()

	old := wikidataSPARQLEndpoint
	wikidataSPARQLEndpoint = srv.URL

	defer func() { wikidataSPARQLEndpoint = old }()

	got, err := fetchAttractionCandidates(t.Context(), "Q1000", 50)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d candidates, want 2 (dedup + unlabeled + denylist drop): %+v", len(got), got)
	}

	if got[0].QID != "Q38324" || got[0].Name != "Victoria Falls" {
		t.Fatalf("candidate[0] wrong: %+v", got[0])
	}
	// §80 border attractions — the item's three bindings carry two
	// distinct P131 locations; both fold into LocatedIn, deduped.
	if !slices.Equal(got[0].LocatedIn, []string{"Q1111", "Q2222"}) {
		t.Fatalf("LocatedIn wrong: %+v", got[0].LocatedIn)
	}

	if len(got[1].LocatedIn) != 0 {
		t.Fatalf("no-loc candidate must have empty LocatedIn: %+v", got[1].LocatedIn)
	}
}

// TestAttractionP131ParentLinking exercises the multi-P131 parent
// resolution end-to-end against a real DB: stamped geo spaces resolve via
// SpaceIDsByWikidataIDs, linkAttractionParents adds every match, skips
// non-geo types and unmatched QIDs, and re-runs stay idempotent.
func TestAttractionP131ParentLinking(t *testing.T) {
	svc, database := linkTestService(t)
	ctx := t.Context()
	// t.Context() is cancelled before Cleanup funcs run — teardown
	// deletes need a live context or they silently no-op.
	bg := t.Context()
	q := gen.New(database.Writer)
	a := NewProcessorAdapter(svc)

	countryType, err := q.GetSpaceTypeBySlug(ctx, "country")
	if err != nil {
		t.Fatalf("country type: %v", err)
	}

	attractionType, err := q.GetSpaceTypeBySlug(ctx, "attraction")
	if err != nil {
		t.Fatalf("attraction type: %v", err)
	}

	hash := "x"

	creatorID, err := q.CreateUser(ctx, gen.CreateUserParams{
		Email:        "p131-test@example.invalid",
		Username:     "p131-test",
		PasswordHash: &hash,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = database.Writer.Exec(bg, `DELETE FROM users WHERE id = $1`, creatorID)
	})

	mkSpace := func(name, slug string, typeID pgtype.UUID, qid string, parents []pgtype.UUID) gen.CreateSpaceRow {
		t.Helper()

		var wc []byte
		if qid != "" {
			wc, _ = json.Marshal(map[string]string{"wikidata_id": qid})
		}

		sp, err := svc.CreateSpaceWithOwner(ctx, gen.CreateSpaceParams{
			SpaceTypeID: typeID,
			Name:        name,
			Slug:        slug,
			WikiContent: wc,
			Level:       "custom",
			Visibility:  VisibilityPublic,
			PostingPerm: PermAdmins,
			Status:      "active",
		}, creatorID, parents)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}

		t.Cleanup(func() {
			_, _ = database.Writer.Exec(bg, `DELETE FROM spaces WHERE id = $1`, sp.ID)
		})

		return sp
	}

	// Unique QIDs/slugs per run — rows left by an earlier failed run must
	// never collide with this one's fixtures.
	uniq := strconv.FormatInt(time.Now().UnixNano()%100000, 10)
	zambia := mkSpace("P131 Zambia", "p131-zambia-"+uniq, countryType.ID, "QTP111"+uniq, nil)
	zimbabwe := mkSpace("P131 Zimbabwe", "p131-zimbabwe-"+uniq, countryType.ID, "QTP222"+uniq, nil)

	// Batch resolution: matches both stamps, skips the unknown QID.
	matches, err := q.SpaceIDsByWikidataIDs(ctx, []string{"QTP111" + uniq, "QTP222" + uniq, "QTP_MISSING"})
	if err != nil {
		t.Fatalf("SpaceIDsByWikidataIDs: %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2: %+v", len(matches), matches)
	}

	for _, m := range matches {
		if m.TypeSlug != "country" || m.Qid == "" {
			t.Fatalf("bad match row: %+v", m)
		}
	}

	falls := mkSpace("P131 Falls", "p131-falls-"+uniq, attractionType.ID, "QTP333"+uniq,
		[]pgtype.UUID{zambia.ID})

	full, err := svc.GetSpace(ctx, falls.ID)
	if err != nil {
		t.Fatalf("get attraction: %v", err)
	}

	ent := &wikidataUniversity{LocatedIn: []string{"QTP111" + uniq, "QTP222" + uniq, "QTP_MISSING"}}
	a.linkAttractionParents(ctx, &full, ent)
	a.linkAttractionParents(ctx, &full, ent) // idempotent re-run

	parents, err := svc.GetParentSpaces(ctx, falls.ID)
	if err != nil {
		t.Fatalf("parents: %v", err)
	}

	have := map[pgtype.UUID]bool{}
	for _, p := range parents {
		have[p.ID] = true
	}

	if !have[zambia.ID] || !have[zimbabwe.ID] {
		t.Fatalf("want both geo parents linked, got %+v", parents)
	}

	if len(parents) != 2 {
		t.Fatalf("re-run duplicated a parent link: %+v", parents)
	}
}
