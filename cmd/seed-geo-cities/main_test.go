package main

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func uid(s string) pgtype.UUID {
	u := uuid.NewSHA1(uuid.NameSpaceOID, []byte(s))
	return pgtype.UUID{Bytes: u, Valid: true}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Frankfurt (Oder)":        "frankfurt-oder",
		"Zürich (Kreis 10)":       "zurich-kreis-10",
		"São Paulo":               "sao-paulo",
		"Al Bayda'":               "al-bayda",
		"'s-Hertogenbosch":        "s-hertogenbosch",
		"Machida-shi":             "machida-shi",
		"Cox’s Bāzār":             "cox-s-bazar",
		"City - trailing - dash-": "city-trailing-dash",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAsciiFoldNonDecomposing(t *testing.T) {
	// Letters with no NFD decomposition must still fold to ASCII so GeoNames
	// ASCII spellings match seeded names ("Balikesir" vs "Balıkesir").
	cases := map[string]string{
		"Balıkesir":  "balikesir",
		"Đồng Nai":   "dong nai",
		"Østfold":    "ostfold",
		"Łódź":       "lodz",
		"Kırklareli": "kirklareli",
		"Şanlıurfa":  "sanliurfa",
		"Diyarbakır": "diyarbakir",
		"Međimurje":  "medimurje",
		"İzmir":      "izmir",
	}
	for in, want := range cases {
		if got := normName(in); got != want {
			t.Errorf("normName(%q) = %q, want %q", in, got, want)
		}
	}
}

// fixture for a country whose seeded states use ISO-style geo_codes that
// numerically collide with GeoNames admin1 codes (GeoNames PT.09 = Faro,
// but the seeded state with geo_code '09' is Guarda).
type stateFx struct {
	byCode    map[pgtype.UUID]map[string]pgtype.UUID
	byNorm    map[pgtype.UUID]map[string]pgtype.UUID
	bySqueeze map[pgtype.UUID]map[string]pgtype.UUID
	tokens    map[pgtype.UUID]map[pgtype.UUID]map[string]struct{}
	byID      map[pgtype.UUID]geoRow
}

// countryKey is the short string passed to uid() for the country (e.g. "PT"),
// so state ids are uid("PT/Faro") and assertions can reference them directly.
func newStateFx(countryID pgtype.UUID, countryKey string, states map[string]string /* geoCode -> name */) stateFx {
	fx := stateFx{
		byCode:    map[pgtype.UUID]map[string]pgtype.UUID{countryID: {}},
		byNorm:    map[pgtype.UUID]map[string]pgtype.UUID{countryID: {}},
		bySqueeze: map[pgtype.UUID]map[string]pgtype.UUID{countryID: {}},
		tokens:    map[pgtype.UUID]map[pgtype.UUID]map[string]struct{}{countryID: {}},
		byID:      map[pgtype.UUID]geoRow{},
	}

	for code, name := range states {
		id := uid(countryKey + "/" + name)
		norm := normName(name)
		fx.byCode[countryID][code] = id
		fx.byNorm[countryID][norm] = id
		fx.bySqueeze[countryID][norm] = id
		fx.tokens[countryID][id] = tokenSet(norm)
		fx.byID[id] = geoRow{id: id, name: name, geoCode: code}
	}

	return fx
}

func (fx stateFx) resolve(c cityRow, countryID pgtype.UUID, admin1Name map[string]string) (pgtype.UUID, bool) {
	return resolveState(c, countryID, admin1Name, fx.byCode, fx.byNorm, fx.bySqueeze, fx.tokens, fx.byID)
}

func TestResolveStateNameBeatsNumericCode(t *testing.T) {
	pt := uid("PT")
	fx := newStateFx(pt, "PT", map[string]string{"09": "Guarda", "08": "Faro"})
	admin1 := map[string]string{"PT.09": "Faro"}

	// GeoNames PT.09 is Faro; seeded geo '09' is Guarda — name must win.
	got, ok := fx.resolve(cityRow{name: "Lagos", country: "PT", admin1: "09"}, pt, admin1)
	if !ok || got != uid("PT/Faro") {
		t.Fatalf("expected Faro, got %v ok=%v", got, ok)
	}
}

func TestResolveStateNumericCodeCorroborated(t *testing.T) {
	ke := uid("KE")
	fx := newStateFx(ke, "KE", map[string]string{"30": "Nairobi County", "05": "Elgeyo-Marakwet"})
	admin1 := map[string]string{"KE.30": "Nairobi Area"} // tokens don't match exactly

	// 'Nairobi Area' token-subset matches 'Nairobi County' — name path wins.
	got, ok := fx.resolve(cityRow{name: "Nairobi", country: "KE", admin1: "30"}, ke, admin1)
	if !ok || got != uid("KE/Nairobi County") {
		t.Fatalf("expected Nairobi County via name, got %v ok=%v", got, ok)
	}

	// When no admin1 name exists, a bare numeric code IS trusted (nothing to
	// corroborate against) — falls back to the code match.
	got, ok = fx.resolve(cityRow{name: "Iten", country: "KE", admin1: "05"}, ke, map[string]string{})
	if !ok || got != uid("KE/Elgeyo-Marakwet") {
		t.Fatalf("expected Elgeyo-Marakwet via code fallback, got %v ok=%v", got, ok)
	}
}

func TestResolveStateUncorroboratedNumericCodeIgnored(t *testing.T) {
	jp := uid("JP")
	fx := newStateFx(jp, "JP", map[string]string{"23": "Aichi", "12": "Chiba"})
	// GeoNames JP.01 = Aichi but seeded '01' would be Hokkaidō — here '12'
	// is the code collision; admin1 name 'Chiba' matches state 'Chiba'.
	admin1 := map[string]string{"JP.12": "Chiba"}

	got, ok := fx.resolve(cityRow{name: "Chiba", country: "JP", admin1: "12"}, jp, admin1)
	if !ok || got != uid("JP/Chiba") {
		t.Fatalf("expected Chiba, got %v ok=%v", got, ok)
	}

	// Numeric code disagrees with name AND name resolves elsewhere -> name wins.
	fx2 := newStateFx(jp, "JP", map[string]string{"23": "Aichi", "40": "Fukuoka"})
	admin1b := map[string]string{"JP.40": "Tokyo"}
	fx2.byNorm[jp]["tokyo"] = uid("JP/Tokyo")
	fx2.bySqueeze[jp]["tokyo"] = uid("JP/Tokyo")
	fx2.tokens[jp][uid("JP/Tokyo")] = tokenSet("tokyo")
	fx2.byID[uid("JP/Tokyo")] = geoRow{id: uid("JP/Tokyo"), name: "Tokyo", geoCode: "13"}

	got, ok = fx2.resolve(cityRow{name: "Tokyo", country: "JP", admin1: "40"}, jp, admin1b)
	if !ok || got != uid("JP/Tokyo") {
		t.Fatalf("expected Tokyo (name-first), got %v ok=%v", got, ok)
	}
}

func TestResolveStateLetteredCodeTrusted(t *testing.T) {
	gb := uid("GB")
	fx := newStateFx(gb, "GB", map[string]string{"eng": "England"})

	got, ok := fx.resolve(cityRow{name: "London", country: "GB", admin1: "ENG"}, gb, map[string]string{})
	if !ok || got != uid("GB/England") {
		t.Fatalf("expected England via lettered code, got %v ok=%v", got, ok)
	}
}

func TestMatchStateNameAmbiguousSubset(t *testing.T) {
	id1, id2 := uid("a"), uid("b")
	tokens := map[pgtype.UUID]map[string]struct{}{
		id1: tokenSet("north"),
		id2: tokenSet("south"),
	}
	byNorm := map[string]pgtype.UUID{"north": id1, "south": id2}
	bySq := map[string]pgtype.UUID{"north": id1, "south": id2}

	// Exact normalized match.
	if got, ok := matchStateName("North", byNorm, bySq, tokens); !ok || got != id1 {
		t.Fatalf("exact match failed: %v %v", got, ok)
	}
	// Space-collapsed.
	byNorm["north west"] = id1

	bySq["northwest"] = id1
	if got, ok := matchStateName("North-Western X", map[string]pgtype.UUID{"northwestern x": id1}, bySq, tokens); !ok || got != id1 {
		t.Fatalf("squeeze match failed: %v %v", got, ok)
	}
	// Ambiguous token-subset: 'north' ⊆ both? 'north' ⊆ {north} only -> unique.
	tok2 := map[pgtype.UUID]map[string]struct{}{id1: {"north": {}}, id2: {"north": {}, "east": {}}}
	byNorm2 := map[string]pgtype.UUID{} // force token path

	if _, ok := matchStateName("north", byNorm2, map[string]pgtype.UUID{}, tok2); ok {
		t.Fatal("expected ambiguous token-subset to resolve to nothing")
	}
}

func TestUniqueSlugDeterministic(t *testing.T) {
	taken := map[string]struct{}{"london": {}, "london-gb": {}}
	if got := uniqueSlug("london", "GB", "ENG", taken); got != "london-gb-eng" {
		t.Fatalf("got %q", got)
	}

	taken["london-gb-eng"] = struct{}{}
	if got := uniqueSlug("london", "GB", "ENG", taken); got != "london-gb-eng-2" {
		t.Fatalf("got %q", got)
	}

	if got := uniqueSlug("", "GB", "ENG", taken); got != "" {
		t.Fatalf("empty base should yield empty slug, got %q", got)
	}

	if got := uniqueSlug("fresh", "GB", "ENG", taken); got != "fresh" {
		t.Fatalf("got %q", got)
	}
}

func TestPinAdmin1(t *testing.T) {
	rows := []cityRow{
		{name: "Arai", country: "JP", admin1: "29"},
		{name: "Arai", country: "JP", admin1: "15"},
	}
	if got := pinAdmin1(rows, "arai-jp-29", "JP"); len(got) != 1 || got[0].admin1 != "29" {
		t.Fatalf("pin failed: %+v", got)
	}

	if got := pinAdmin1(rows, "arai-jp-29-2", "JP"); len(got) != 1 || got[0].admin1 != "29" {
		t.Fatalf("numbered pin failed: %+v", got)
	}

	if got := pinAdmin1(rows, "arai", "JP"); got != nil {
		t.Fatalf("base slug should not pin: %+v", got)
	}

	if got := pinAdmin1(rows[:1], "arai-jp-29", "JP"); got != nil {
		t.Fatalf("single-variant should not pin: %+v", got)
	}
}
