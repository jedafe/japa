package visitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/jedafe/japa/internal/geoip"
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

	originMkts, destMkts := defaultMarketSets()

	tests := []struct {
		name           string
		current        string
		stage          string
		destination    string
		origin         string
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
		// Declared origin market lifts a visitor located in a non-market
		// country to origin_visitor (e.g. a Nigerian in transit in Brazil).
		{
			name:    "declared origin market, current non-market",
			current: "BR", stage: "planning", destination: "", origin: "NG",
			wantArchetype: ArchetypeOriginVisitor, wantCountry: "NG", wantConfidence: 0.85,
		},
		{
			name:    "declared origin market with destination",
			current: "BR", stage: "planning", destination: "CA", origin: "NG",
			wantArchetype: ArchetypeOriginVisitor, wantCountry: "CA", wantConfidence: 0.85,
		},
		// Destination-market location and settled status still win over a
		// declared origin — someone who already relocated is diaspora.
		{
			name:    "declared origin market but in destination market",
			current: "GB", stage: "newly_japa", destination: "", origin: "NG",
			wantArchetype: ArchetypeDiaspora, wantCountry: "GB", wantConfidence: 0.85,
		},
		{
			name:    "declared origin market but settled",
			current: "BR", stage: "settled", destination: "", origin: "NG",
			wantArchetype: ArchetypeDiaspora, wantCountry: "BR", wantConfidence: 0.85,
		},
		// No GeoIP at all — declared origin alone still classifies.
		{
			name:    "declared origin market, no current country",
			current: "", stage: "", destination: "", origin: "GH",
			wantArchetype: ArchetypeOriginVisitor, wantCountry: "GH", wantConfidence: 0.85,
		},
		// In an origin market while declaring a different origin market —
		// the explicit declaration wins for ContentCountry.
		{
			name:    "declared origin overrides current origin market",
			current: "GH", stage: "planning", destination: "", origin: "NG",
			wantArchetype: ArchetypeOriginVisitor, wantCountry: "NG", wantConfidence: 0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotArch, gotCountry, gotConf := resolveArchetype(
				tt.current, tt.stage, tt.destination, tt.origin, originMkts, destMkts,
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

	b, err := json.Marshal(cachedVisitorContext{
		Fingerprint: res.geoConfigFingerprint(t.Context()),
		VC:          vc,
	})
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

func TestUnmarshalCacheRejectsBareVisitorContext(t *testing.T) {
	t.Parallel()

	// Entries written before the fingerprint-in-payload format are bare
	// VisitorContext JSON — they decode to an empty fingerprint and must be
	// treated as misses (self-heals via re-resolution).
	res := NewResolver(nil, nil, nil)
	b, err := json.Marshal(VisitorContext{ContentCountry: "NG", Source: "geoip"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if _, ok := res.unmarshalCache(t.Context(), string(b)); ok {
		t.Fatal("unmarshalCache accepted bare VisitorContext; expected ok=false")
	}
}

func TestUnmarshalCacheRejectsStaleFingerprint(t *testing.T) {
	t.Parallel()

	// An entry resolved under a different geo config must not be served —
	// a fingerprint mismatch is a miss, not a stale serve.
	res := NewResolver(nil, nil, nil)
	b, err := json.Marshal(cachedVisitorContext{
		Fingerprint: "otherfp",
		VC:          VisitorContext{ContentCountry: "GB", Source: "geoip"},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if _, ok := res.unmarshalCache(t.Context(), string(b)); ok {
		t.Fatal("unmarshalCache accepted stale fingerprint; expected ok=false")
	}
}

func TestUnmarshalCacheEmptyCountryDefaultsToNG(t *testing.T) {
	t.Parallel()

	// A cached JSON entry with an empty ContentCountry (edge case) must
	// fall back to the configured default (NG when no settings/platform).
	res := NewResolver(nil, nil, nil)
	b, _ := json.Marshal(cachedVisitorContext{
		Fingerprint: res.geoConfigFingerprint(t.Context()),
		VC:          VisitorContext{Source: "archetype"},
	})

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

func TestExplicitGeoParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		url    string
		wantCC string
		wantOK bool
	}{
		{"/feed?geo=all", "", true},
		{"/feed?geo=ALL", "", true},
		{"/feed?geo=All", "", true},
		{"/feed?geo=ng", "NG", true},
		{"/feed?geo=%20gb%20", "GB", true},
		{"/feed", "", false},
		{"/feed?geo=", "", false},
		{"/feed?geo=NGA", "", false},
		{"/feed?geo=12", "", false},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.url, nil)

		cc, ok := explicitGeoParam(req)
		if ok != tt.wantOK || cc != tt.wantCC {
			t.Errorf("explicitGeoParam(%q) = (%q, %v), want (%q, %v)",
				tt.url, cc, ok, tt.wantCC, tt.wantOK)
		}
	}
}

// withGeoIP injects a GeoContext the way geoip.Middleware would.
func withGeoIP(r *http.Request, gc geoip.GeoContext) *http.Request {
	ctx := context.WithValue(r.Context(), geoip.GeoContextKey, gc)
	return r.WithContext(ctx)
}

func TestResolve_GeoAllExplicitGlobal(t *testing.T) {
	t.Parallel()

	// ADR-064 §2: ?geo=all explicitly requests global ranking — an empty
	// ContentCountry (no geo filter), even when GeoIP resolved a country.
	res := NewResolver(nil, nil, nil)
	req := withGeoIP(httptest.NewRequest(http.MethodGet, "/feed?geo=all", nil),
		geoip.GeoContext{CountryCode: "GB", IsValid: true})

	vc := res.resolve(req)
	if vc.ContentCountry != "" {
		t.Errorf("ContentCountry = %q, want empty (global)", vc.ContentCountry)
	}

	if vc.Source != "query" {
		t.Errorf("Source = %q, want query", vc.Source)
	}
}

func TestResolve_GeoParamPinsCountry(t *testing.T) {
	t.Parallel()

	// ?geo=fr wins over GeoIP GB — the explicit param is priority 1.
	res := NewResolver(nil, nil, nil)
	req := withGeoIP(httptest.NewRequest(http.MethodGet, "/feed?geo=fr", nil),
		geoip.GeoContext{CountryCode: "GB", IsValid: true})

	vc := res.resolve(req)
	if vc.ContentCountry != "FR" {
		t.Errorf("ContentCountry = %q, want FR", vc.ContentCountry)
	}

	if vc.Source != "query" {
		t.Errorf("Source = %q, want query", vc.Source)
	}
}

func TestResolve_GeoParamInvalidFallsThrough(t *testing.T) {
	t.Parallel()

	// An invalid ?geo= value must not be treated as an override — normal
	// resolution proceeds (GeoIP GB → diaspora → ContentCountry GB).
	res := NewResolver(nil, nil, nil)
	req := withGeoIP(httptest.NewRequest(http.MethodGet, "/feed?geo=1A", nil),
		geoip.GeoContext{CountryCode: "GB", IsValid: true})

	vc := res.resolve(req)
	if vc.ContentCountry != "GB" {
		t.Errorf("ContentCountry = %q, want GB", vc.ContentCountry)
	}

	if vc.Source == "query" {
		t.Error("Source = query for an invalid ?geo= value; expected archetype/default")
	}
}

func TestAnonCacheKey(t *testing.T) {
	t.Parallel()

	res := NewResolver(nil, nil, nil)

	// Journey-cookie guests resolve under the token — never the IP key, so
	// journey fields cannot bleed to other guests on the same NAT address.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "jc_journey_token", Value: "tok123"})

	if k := res.anonCacheKey(req); !strings.HasSuffix(k, "visitor:ctx:journey:tok123") {
		t.Errorf("anonCacheKey(cookie) = %q, want ...visitor:ctx:journey:tok123", k)
	}

	// Param-only journeys are transient — no cache entry at all.
	req = httptest.NewRequest(http.MethodGet, "/?journey_origin=NG", nil)
	if k := res.anonCacheKey(req); k != "" {
		t.Errorf("anonCacheKey(param-only) = %q, want empty", k)
	}

	// Params are transient even for cookied guests: a param-overridden
	// context must neither persist under the token key nor be masked by the
	// cached entry — so it bypasses the cache entirely.
	req = httptest.NewRequest(http.MethodGet, "/?journey_origin=NG", nil)
	req.AddCookie(&http.Cookie{Name: "jc_journey_token", Value: "tok9"})

	if k := res.anonCacheKey(req); k != "" {
		t.Errorf("anonCacheKey(cookie+params) = %q, want empty (params bypass cache)", k)
	}

	// Plain anonymous guests fall back to the IP scope.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	if k := res.anonCacheKey(req); !strings.Contains(k, "visitor:ctx:ip:") {
		t.Errorf("anonCacheKey(plain) = %q, want ip scope", k)
	}
}
