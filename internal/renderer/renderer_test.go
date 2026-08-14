package renderer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/net/html"
)

func TestCacheKey(t *testing.T) {
	t.Parallel()

	// Use a valid relative path so New succeeds; the cacheKey logic is
	// independent of the script path content.
	r, err := New("tiptap-ssr.ts", nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	var tenantID pgtype.UUID
	if err := tenantID.Scan("11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatal(err)
	}

	var contentID pgtype.UUID
	if err := contentID.Scan("550e8400-e29b-41d4-a716-446655440000"); err != nil {
		t.Fatal(err)
	}

	updatedAt := pgtype.Timestamptz{Time: time.Unix(1234567890, 0), Valid: true}

	key := r.cacheKey(tenantID, contentID, updatedAt)

	// Key should start with the jc:render:{tenant_id}: prefix
	if len(key) < 10 {
		t.Fatalf("cache key too short: %s", key)
	}

	if key[:11] != "jc:render:1" {
		t.Fatalf("cache key prefix wrong: %s", key)
	}

	// Same inputs must produce the same key (deterministic)
	key2 := r.cacheKey(tenantID, contentID, updatedAt)
	if key != key2 {
		t.Fatalf("cache key not deterministic: %s != %s", key, key2)
	}

	// Different timestamp must produce a different key
	updatedAt2 := pgtype.Timestamptz{Time: time.Unix(1234567891, 0), Valid: true}

	key3 := r.cacheKey(tenantID, contentID, updatedAt2)
	if key == key3 {
		t.Fatalf("different timestamps produced same key: %s", key)
	}

	// Different tenant must produce a different key (§4.2 tenant scoping)
	var tenantID2 pgtype.UUID
	if err := tenantID2.Scan("22222222-2222-2222-2222-222222222222"); err != nil {
		t.Fatal(err)
	}
	key4 := r.cacheKey(tenantID2, contentID, updatedAt)
	if key == key4 {
		t.Fatalf("different tenants produced same key: %s", key)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	// Valid relative .ts path inside the default allowlist root.
	r, err := New("tiptap-ssr.ts", nil)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if r == nil {
		t.Fatal("New returned nil")
	}

	if r.cache != nil {
		t.Fatal("cache should be nil")
	}

	if r.cacheTTL != 24*time.Hour {
		t.Fatalf("cacheTTL = %v, want 24h", r.cacheTTL)
	}

	if r.timeout != defaultTimeout {
		t.Fatalf("timeout = %v, want %v", r.timeout, defaultTimeout)
	}

	if r.sem == nil {
		t.Fatal("sem should be initialized")
	}

	if cap(r.sem) != defaultMaxConcurrent {
		t.Fatalf("sem capacity = %d, want %d", cap(r.sem), defaultMaxConcurrent)
	}
}

func TestNew_RejectsInvalidScriptPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{"absolute path", "/etc/passwd"},
		{"parent traversal", "../escape.ts"},
		{"nested parent traversal", "sub/../../escape.ts"},
		{"non-js extension", "tiptap-ssr.txt"},
		{"empty", ""},
		{"executable", "tiptap-ssr.sh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(tt.path, nil); err == nil {
				t.Errorf("New(%q) expected error, got nil", tt.path)
			}
		})
	}
}

// --- validateRenderInput tests ---

func TestValidateRenderInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid doc",
			input:   `{"type":"doc","content":[{"type":"paragraph"}]}`,
			wantErr: false,
		},
		{
			name:    "empty input",
			input:   ``,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			input:   `{not json}`,
			wantErr: true,
		},
		{
			name:    "wrong root type",
			input:   `{"type":"paragraph","content":[]}`,
			wantErr: true,
		},
		{
			name:    "missing content field",
			input:   `{"type":"doc"}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateRenderInput([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRenderInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRenderInput_SizeLimit(t *testing.T) {
	t.Parallel()

	// Construct a valid doc that exceeds MaxRenderInputSize.
	over := make([]byte, MaxRenderInputSize+1)
	for i := range over {
		over[i] = ' '
	}
	wrapped := `{"type":"doc","content":[{"type":"paragraph","text":"` + string(over) + `"}]}`
	if err := validateRenderInput([]byte(wrapped)); err == nil {
		t.Error("expected size limit error, got nil")
	}
}

// --- buildCouponWidgetHTML tests ---

func TestBuildCouponWidgetHTML_Expired(t *testing.T) {
	t.Parallel()
	c := &CouponRenderResult{IsExpired: true, Status: "active"}
	html := buildCouponWidgetHTML(c, "full")
	if !strings.Contains(html, "coupon-expired") {
		t.Fatalf("expected expired widget, got: %s", html)
	}
}

func TestBuildCouponWidgetHTML_Suspended(t *testing.T) {
	t.Parallel()
	c := &CouponRenderResult{Status: "suspended"}
	html := buildCouponWidgetHTML(c, "full")
	if !strings.Contains(html, "coupon-expired") {
		t.Fatalf("expected expired widget for suspended status, got: %s", html)
	}
}

func TestBuildCouponWidgetHTML_Active(t *testing.T) {
	t.Parallel()
	c := &CouponRenderResult{
		Title:         "20% off Remitly",
		Code:          "JAPA20",
		Status:        "active",
		DiscountType:  strPtr("percent"),
		DiscountValue: strPtr("20"),
		ReferralURL:   strPtr("https://remitly.com/?ref=japa20"),
	}
	html := buildCouponWidgetHTML(c, "full")
	if !strings.Contains(html, "coupon-card") {
		t.Fatalf("expected coupon-card, got: %s", html)
	}
	if !strings.Contains(html, "JAPA20") {
		t.Fatalf("expected code in widget, got: %s", html)
	}
	if !strings.Contains(html, `href="https://remitly.com/?ref=japa20"`) {
		t.Fatalf("expected safe referral URL in href, got: %s", html)
	}
}

// TestBuildCouponWidgetHTML_JavaScriptURIRejected verifies that a
// javascript: URI in referral_url is NOT injected into the href attribute
// (BE-HIGH-001 — stored XSS via coupon widget CTA).
func TestBuildCouponWidgetHTML_JavaScriptURIRejected(t *testing.T) {
	t.Parallel()
	c := &CouponRenderResult{
		Title:       "Malicious coupon",
		Code:        "EVIL",
		Status:      "active",
		ReferralURL: strPtr("javascript:alert(document.cookie)"),
	}
	html := buildCouponWidgetHTML(c, "full")
	if strings.Contains(html, "javascript:") {
		t.Fatalf("javascript: URI leaked into widget HTML: %s", html)
	}
	if strings.Contains(html, "coupon-cta") {
		t.Fatalf("CTA link should be omitted for unsafe URL, got: %s", html)
	}
}

func TestBuildCouponWidgetHTML_DataURIRejected(t *testing.T) {
	t.Parallel()
	c := &CouponRenderResult{
		Title:       "Data URI coupon",
		Code:        "DATA",
		Status:      "active",
		ReferralURL: strPtr("data:text/html,<script>alert(1)</script>"),
	}
	html := buildCouponWidgetHTML(c, "full")
	if strings.Contains(html, "data:") {
		t.Fatalf("data: URI leaked into widget HTML: %s", html)
	}
}

func TestBuildCouponWidgetHTML_CompactMode(t *testing.T) {
	t.Parallel()
	c := &CouponRenderResult{
		Title:         "Compact coupon",
		Code:          "CMP",
		Status:        "active",
		DiscountType:  strPtr("percent"),
		DiscountValue: strPtr("50"),
		ProviderName:  strPtr("TestProvider"),
		ExpiresAt:     strPtr("2026-12-31T23:59:59Z"),
	}
	html := buildCouponWidgetHTML(c, "compact")
	if !strings.Contains(html, "coupon-card-compact") {
		t.Fatalf("expected compact class, got: %s", html)
	}
	// Compact mode should NOT include discount/provider/expiry details.
	if strings.Contains(html, "coupon-discount") {
		t.Fatalf("compact mode should not include discount, got: %s", html)
	}
	if strings.Contains(html, "coupon-provider") {
		t.Fatalf("compact mode should not include provider, got: %s", html)
	}
	if strings.Contains(html, "coupon-expires") {
		t.Fatalf("compact mode should not include expiry, got: %s", html)
	}
}

func TestBuildCouponWidgetHTML_FullMode(t *testing.T) {
	t.Parallel()
	c := &CouponRenderResult{
		Title:         "Full coupon",
		Code:          "FULL",
		Status:        "active",
		DiscountType:  strPtr("percent"),
		DiscountValue: strPtr("50"),
		ProviderName:  strPtr("TestProvider"),
	}
	html := buildCouponWidgetHTML(c, "full")
	if strings.Contains(html, "coupon-card-compact") {
		t.Fatalf("full mode should not have compact class, got: %s", html)
	}
	if !strings.Contains(html, "coupon-discount") {
		t.Fatalf("full mode should include discount, got: %s", html)
	}
	if !strings.Contains(html, "coupon-provider") {
		t.Fatalf("full mode should include provider, got: %s", html)
	}
}

// --- isSafeURL tests ---

func TestIsSafeURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		url  string
		want bool
	}{
		{"https://example.com", true},
		{"http://example.com", true},
		{"/relative/path", true},
		{"#anchor", true},
		{"?query=1", true},
		{"", true},
		{"javascript:alert(1)", false},
		{"JavaScript:alert(1)", false},
		{"data:text/html,<script>", false},
		{"vbscript:msgbox(1)", false},
		{"javascript://example.com/%0aalert(1)", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			t.Parallel()
			if got := isSafeURL(tt.url); got != tt.want {
				t.Errorf("isSafeURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

// --- isExternalLink tests ---

func TestIsExternalLink(t *testing.T) {
	t.Parallel()
	tests := []struct {
		href string
		want bool
	}{
		{"https://example.com", true},
		{"http://example.com", true},
		{"/relative/path", false},
		{"javascript:alert(1)", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.href, func(t *testing.T) {
			t.Parallel()
			if got := isExternalLink(tt.href); got != tt.want {
				t.Errorf("isExternalLink(%q) = %v, want %v", tt.href, got, tt.want)
			}
		})
	}
}

// --- resolveCouponEmbedsAndAffiliateLinks tests ---

func TestResolveCouponEmbedsAndAffiliateLinks_NilResolvers(t *testing.T) {
	t.Parallel()
	input := `<p>hello world</p>`
	out := resolveCouponEmbedsAndAffiliateLinks(context.Background(), pgtype.UUID{}, input, nil, nil)
	if out != input {
		t.Fatalf("expected unchanged input when resolvers are nil, got: %s", out)
	}
}

func TestResolveCouponEmbedsAndAffiliateLinks_CouponEmbed(t *testing.T) {
	t.Parallel()
	mock := &mockCouponResolver{
		result: &CouponRenderResult{
			Title:       "Test Coupon",
			Code:        "TEST123",
			Status:      "active",
			ReferralURL: strPtr("https://example.com/deal"),
		},
	}
	input := `<div data-type="coupon-embed" data-coupon-id="550e8400-e29b-41d4-a716-446655440000"></div>`
	out := resolveCouponEmbedsAndAffiliateLinks(context.Background(), pgtype.UUID{}, input, mock, nil)
	if !strings.Contains(out, "Test Coupon") {
		t.Fatalf("expected coupon title in output, got: %s", out)
	}
	if !strings.Contains(out, "TEST123") {
		t.Fatalf("expected coupon code in output, got: %s", out)
	}
	if !strings.Contains(out, "coupon-embed-widget") {
		t.Fatalf("expected coupon-embed-widget class, got: %s", out)
	}
}

func TestResolveCouponEmbedsAndAffiliateLinks_AffiliateLink(t *testing.T) {
	t.Parallel()
	mock := &mockAffiliateResolver{
		result: &AffiliateLinkRenderResult{
			ResolvedURL: "https://booking.com/hotel?ref=japa20",
		},
	}
	input := `<a href="https://booking.com/hotel" data-affiliate-provider-id="550e8400-e29b-41d4-a716-446655440000">Book hotel</a>`
	out := resolveCouponEmbedsAndAffiliateLinks(context.Background(), pgtype.UUID{}, input, nil, mock)
	if !strings.Contains(out, "https://booking.com/hotel?ref=japa20") {
		t.Fatalf("expected rewritten href with tracking params, got: %s", out)
	}
}

func TestResolveCouponEmbedsAndAffiliateLinks_AffiliateLinkNoProviderID(t *testing.T) {
	t.Parallel()
	mock := &mockAffiliateResolver{
		result: &AffiliateLinkRenderResult{
			ResolvedURL: "https://example.com/should-not-appear",
		},
	}
	input := `<a href="https://booking.com/hotel">Book hotel</a>`
	out := resolveCouponEmbedsAndAffiliateLinks(context.Background(), pgtype.UUID{}, input, nil, mock)
	// Link without data-affiliate-provider-id should be left unchanged.
	if strings.Contains(out, "should-not-appear") {
		t.Fatalf("link without provider_id should not be rewritten, got: %s", out)
	}
	if !strings.Contains(out, `href="https://booking.com/hotel"`) {
		t.Fatalf("original href should be preserved, got: %s", out)
	}
}

func TestResolveCouponEmbedsAndAffiliateLinks_DisplayMode(t *testing.T) {
	t.Parallel()
	mock := &mockCouponResolver{
		result: &CouponRenderResult{
			Title:         "Compact Test",
			Code:          "CMP",
			Status:        "active",
			DiscountType:  strPtr("percent"),
			DiscountValue: strPtr("50"),
			ProviderName:  strPtr("Provider"),
		},
	}
	input := `<div data-type="coupon-embed" data-coupon-id="550e8400-e29b-41d4-a716-446655440000" data-display-mode="compact"></div>`
	out := resolveCouponEmbedsAndAffiliateLinks(context.Background(), pgtype.UUID{}, input, mock, nil)
	if !strings.Contains(out, "coupon-card-compact") {
		t.Fatalf("expected compact class in output, got: %s", out)
	}
	// Compact mode should not include discount details.
	if strings.Contains(out, "coupon-discount") {
		t.Fatalf("compact mode should not include discount, got: %s", out)
	}
}

// --- getAttr / filterAttrs tests ---

func TestGetAttr(t *testing.T) {
	t.Parallel()
	n := &html.Node{Type: html.ElementNode, Data: "div"}
	n.Attr = []html.Attribute{
		{Key: "class", Val: "test"},
		{Key: "data-id", Val: "abc"},
	}
	if got := getAttr(n, "data-id"); got != "abc" {
		t.Fatalf("getAttr(data-id) = %q, want %q", got, "abc")
	}
	if got := getAttr(n, "missing"); got != "" {
		t.Fatalf("getAttr(missing) = %q, want empty", got)
	}
}

// --- mock resolvers ---

type mockCouponResolver struct {
	result *CouponRenderResult
	err    error
}

func (m *mockCouponResolver) ResolveCouponForRender(ctx context.Context, tenantID, id pgtype.UUID) (*CouponRenderResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

type mockAffiliateResolver struct {
	result *AffiliateLinkRenderResult
	err    error
}

func (m *mockAffiliateResolver) ResolveAffiliateLinkForRender(ctx context.Context, tenantID, providerID pgtype.UUID, originalURL string) (*AffiliateLinkRenderResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string { return &s }
