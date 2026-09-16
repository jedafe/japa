package visitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// defaultMarketSets returns the ADR-064 default origin/destination market sets
// (matching the migration 000182 seed fallbacks).
func defaultMarketSets() (origin, dest map[string]bool) {
	origin = csvSet("NG,GH,ZA,KE,EG")
	dest = csvSet("GB,CA,US,DE,AU,AE")

	return origin, dest
}

// csvSet mirrors Resolver.marketSet's parsing for test fixtures.
func csvSet(csv string) map[string]bool {
	out := make(map[string]bool)
	for _, c := range splitCSV(csv) {
		out[c] = true
	}

	return out
}

func splitCSV(csv string) []string {
	var out []string

	cur := ""

	for _, ch := range csv {
		if ch == ',' {
			if cur != "" {
				out = append(out, normalizeCountry(cur))
			}

			cur = ""

			continue
		}

		cur += string(ch)
	}

	if cur != "" {
		out = append(out, normalizeCountry(cur))
	}

	return out
}

func TestResolveArchetype(t *testing.T) {
	t.Parallel()

	origin, dest := defaultMarketSets()

	tests := []struct {
		name           string
		current        string
		stage          string
		destination    string
		wantArchetype  string
		wantCountry    string
		wantConfidence float32
	}{
		// origin_visitor: current is an origin market, not settled.
		{
			name:    "origin market no destination",
			current: "NG", stage: "planning", destination: "",
			wantArchetype: ArchetypeOriginVisitor, wantCountry: "NG", wantConfidence: 0.8,
		},
		{
			name:    "origin market with destination",
			current: "NG", stage: "planning", destination: "GB",
			wantArchetype: ArchetypeOriginVisitor, wantCountry: "GB", wantConfidence: 0.8,
		},
		{
			name:    "origin market but settled becomes diaspora",
			current: "NG", stage: "settled", destination: "",
			wantArchetype: ArchetypeDiaspora, wantCountry: "NG", wantConfidence: 0.85,
		},
		// diaspora: current is a destination market.
		{
			name:    "destination market not settled",
			current: "GB", stage: "newly_japa", destination: "",
			wantArchetype: ArchetypeDiaspora, wantCountry: "GB", wantConfidence: 0.85,
		},
		{
			name:    "destination market and settled (highest confidence)",
			current: "US", stage: "settled", destination: "",
			wantArchetype: ArchetypeDiaspora, wantCountry: "US", wantConfidence: 0.95,
		},
		{
			name:    "settled in non-destination country still diaspora",
			current: "BR", stage: "settled", destination: "",
			wantArchetype: ArchetypeDiaspora, wantCountry: "BR", wantConfidence: 0.85,
		},
		// global_traveler: neither pattern.
		{
			name:    "non-market non-settled",
			current: "BR", stage: "planning", destination: "",
			wantArchetype: ArchetypeGlobalTraveler, wantCountry: "BR", wantConfidence: 0.5,
		},
		{
			name:    "no current country (no GeoIP)",
			current: "", stage: "", destination: "",
			wantArchetype: ArchetypeGlobalTraveler, wantCountry: "", wantConfidence: 0.5,
		},
		// Guest in origin market with no stage → origin_visitor.
		{
			name:    "guest in origin market",
			current: "KE", stage: "", destination: "",
			wantArchetype: ArchetypeOriginVisitor, wantCountry: "KE", wantConfidence: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotArch, gotCountry, gotConf := resolveArchetype(
				tt.current, tt.stage, tt.destination, origin, dest,
			)
			if gotArch != tt.wantArchetype {
				t.Errorf("archetype = %q, want %q", gotArch, tt.wantArchetype)
			}

			if gotCountry != tt.wantCountry {
				t.Errorf("contentCountry = %q, want %q", gotCountry, tt.wantCountry)
			}

			if gotConf != tt.wantConfidence {
				t.Errorf("confidence = %v, want %v", gotConf, tt.wantConfidence)
			}
		})
	}
}

func TestNormalizeCountry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in, want string
	}{
		{"ng", "NG"},
		{"  gb  ", "GB"},
		{"NG", "NG"},
		{"", ""},
		{"NGA", ""}, // 3 letters invalid
		{"N", ""},   // 1 letter invalid
		{"n-g", ""}, // hyphen invalid
		{"12", ""},  // non-letters invalid
		{"!@", ""},  // punctuation invalid
		{"gb1", ""}, // 3 chars invalid
	}
	for _, tt := range tests {
		if got := normalizeCountry(tt.in); got != tt.want {
			t.Errorf("normalizeCountry(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveLanguage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		header string
		want   string
	}{
		{"", "en"},
		{"en-US,en;q=0.9", "en"},
		{"fr-FR", "fr"},
		{"de", "de"},
		{"  EN-GB ", "en"},
		{",,", "en"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if tt.header != "" {
			req.Header.Set("Accept-Language", tt.header)
		}

		if got := resolveLanguage(req); got != tt.want {
			t.Errorf("resolveLanguage(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestFromContextDefault(t *testing.T) {
	t.Parallel()

	// No VisitorContext in context → zero-value default.
	vc := FromContext(t.Context())
	if vc.ContentCountry != "NG" {
		t.Errorf("ContentCountry = %q, want NG", vc.ContentCountry)
	}

	if vc.Source != "default" {
		t.Errorf("Source = %q, want default", vc.Source)
	}
}

func TestFromContextSet(t *testing.T) {
	t.Parallel()

	want := VisitorContext{
		ContentCountry: "GB",
		Source:         "archetype",
		Archetype:      ArchetypeDiaspora,
		Confidence:     0.95,
	}
	ctx := context.WithValue(t.Context(), ContextKey{}, want)

	got := FromContext(ctx)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FromContext = %+v, want %+v", got, want)
	}
}

func TestCacheRoundTrip(t *testing.T) {
	t.Parallel()

	res := NewResolver(nil, nil, nil)
	vc := VisitorContext{
		ContentCountry:     "NG",
		Source:             "archetype",
		Archetype:          ArchetypeOriginVisitor,
		Confidence:         0.8,
		CurrentCountry:     "NG",
		DestinationCountry: "GB",
		JapaStage:          "planning",
	}

	b, err := json.Marshal(vc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got, ok := res.unmarshalCache(t.Context(), string(b))
	if !ok {
		t.Fatal("unmarshalCache returned ok=false for JSON entry")
	}

	if got.ContentCountry != vc.ContentCountry {
		t.Errorf("ContentCountry = %q, want %q", got.ContentCountry, vc.ContentCountry)
	}

	if got.Archetype != vc.Archetype {
		t.Errorf("Archetype = %q, want %q", got.Archetype, vc.Archetype)
	}

	if got.Confidence != vc.Confidence {
		t.Errorf("Confidence = %v, want %v", got.Confidence, vc.Confidence)
	}
}

func TestUnmarshalCacheRejectsOldFormat(t *testing.T) {
	t.Parallel()

	res := NewResolver(nil, nil, nil)
	// Old format was "NG|geoip" (not JSON). Must return ok=false so the
	// resolver re-resolves instead of trusting a stale shape.
	if _, ok := res.unmarshalCache(t.Context(), "NG|geoip"); ok {
		t.Fatal("unmarshalCache accepted old-format entry; expected ok=false")
	}

	if _, ok := res.unmarshalCache(t.Context(), ""); ok {
		t.Fatal("unmarshalCache accepted empty entry; expected ok=false")
	}
}

func TestUnmarshalCacheEmptyCountryDefaultsToNG(t *testing.T) {
	t.Parallel()

	// A cached JSON entry with an empty ContentCountry (edge case) must
	// fall back to the configured default (NG when no settings/platform).
	res := NewResolver(nil, nil, nil)
	b, _ := json.Marshal(VisitorContext{Source: "archetype"})

	got, ok := res.unmarshalCache(t.Context(), string(b))
	if !ok {
		t.Fatal("unmarshalCache returned ok=false for valid JSON")
	}

	if got.ContentCountry != "NG" {
		t.Errorf("ContentCountry = %q, want NG fallback", got.ContentCountry)
	}
}

func TestMarketSetParsing(t *testing.T) {
	t.Parallel()

	// Verify the default CSV fallbacks parse to the expected sets — this
	// guards against accidental whitespace/case drift in the seed values.
	origin, dest := defaultMarketSets()
	for _, c := range []string{"NG", "GH", "ZA", "KE", "EG"} {
		if !origin[c] {
			t.Errorf("origin market missing %q", c)
		}
	}

	for _, c := range []string{"GB", "CA", "US", "DE", "AU", "AE"} {
		if !dest[c] {
			t.Errorf("destination market missing %q", c)
		}
	}
	// A non-listed country must not be in either set.
	if origin["GB"] {
		t.Error("GB should not be an origin market")
	}

	if dest["NG"] {
		t.Error("NG should not be a destination market")
	}
}

func TestEnrichArchetypePreservesContentCountryPreference(t *testing.T) {
	t.Parallel()

	// ADR-064 §5: content_country_preference is an explicit user override and
	// must win over archetype-resolved ContentCountry.
	res := NewResolver(nil, nil, nil)
	vc := VisitorContext{
		ContentCountry:     "FR",
		Source:             "preference",
		CurrentCountry:     "NG",
		DestinationCountry: "",
		RoadmapSlugs:       []string{},
		WeakestDimensions:  []string{},
	}
	res.enrichArchetype(t.Context(), &vc, nil, "")

	if vc.ContentCountry != "FR" {
		t.Errorf("ContentCountry = %q, want FR (preference preserved)", vc.ContentCountry)
	}

	if vc.Source != "preference" {
		t.Errorf("Source = %q, want preference", vc.Source)
	}

	if vc.Archetype != ArchetypeOriginVisitor {
		t.Errorf("Archetype = %q, want origin_visitor", vc.Archetype)
	}
}
