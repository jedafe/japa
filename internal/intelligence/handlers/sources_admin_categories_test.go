package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jedafe/japa/internal/db/gen"
)

// --- admin source create: categories[] normalization + dual-write ----------

// validSourceCreateBody returns a body that passes every create validation;
// each test mutates one field to exercise a single rule.
func validSourceCreateBody() handleSourceCreateBody {
	return handleSourceCreateBody{
		Name:        "Admin Test Source",
		SourceURL:   "https://example.test/feed",
		Category:    "news",
		TrustLevel:  3,
		CrawlMethod: "rss",
	}
}

// TestBuildCreateSourceParams_Categories covers the intel-channels
// multi-category payload on the admin create path: the array is normalized
// like Propose (trim/lowercase/dedupe, order preserved) and dual-writes the
// legacy scalar category when category is empty.
func TestBuildCreateSourceParams_Categories(t *testing.T) {
	t.Parallel()

	t.Run("normalized and persisted", func(t *testing.T) {
		t.Parallel()

		body := validSourceCreateBody()
		body.Categories = []string{" News ", "", "JOB", "news", "  "}

		params, ok := buildCreateSourceParams(httptest.NewRecorder(), body)
		if !ok {
			t.Fatal("buildCreateSourceParams = false, want true")
		}

		want := []string{"news", "job"}
		if len(params.Categories) != len(want) {
			t.Fatalf("categories = %v, want %v", params.Categories, want)
		}

		for i := range want {
			if params.Categories[i] != want[i] {
				t.Errorf("categories[%d] = %q, want %q", i, params.Categories[i], want[i])
			}
		}

		if params.Category != "news" {
			t.Errorf("category = %q, want news (explicit category kept)", params.Category)
		}
	})

	t.Run("categories-only dual-writes category", func(t *testing.T) {
		t.Parallel()

		body := validSourceCreateBody()
		body.Category = ""
		body.Categories = []string{" Visa ", "news"}

		params, ok := buildCreateSourceParams(httptest.NewRecorder(), body)
		if !ok {
			t.Fatal("buildCreateSourceParams = false, want true (categories satisfy the category requirement)")
		}

		if params.Category != "visa" {
			t.Errorf("category = %q, want visa (dual-write of categories[0])", params.Category)
		}

		if len(params.Categories) != 2 || params.Categories[0] != "visa" {
			t.Errorf("categories = %v, want [visa news]", params.Categories)
		}
	})

	t.Run("explicit category wins over categories[0]", func(t *testing.T) {
		t.Parallel()

		body := validSourceCreateBody()
		body.Category = "scholarship"
		body.Categories = []string{"news", "job"}

		params, ok := buildCreateSourceParams(httptest.NewRecorder(), body)
		if !ok {
			t.Fatal("buildCreateSourceParams = false, want true")
		}

		if params.Category != "scholarship" {
			t.Errorf("category = %q, want scholarship", params.Category)
		}
	})

	t.Run("absent categories leaves params nil for the SQL fallback", func(t *testing.T) {
		t.Parallel()

		body := validSourceCreateBody()

		params, ok := buildCreateSourceParams(httptest.NewRecorder(), body)
		if !ok {
			t.Fatal("buildCreateSourceParams = false, want true")
		}

		if params.Categories != nil {
			t.Errorf("categories = %v, want nil (INSERT derives ARRAY[category])", params.Categories)
		}
	})

	t.Run("empty categories array falls back like absent", func(t *testing.T) {
		t.Parallel()

		body := validSourceCreateBody()
		body.Categories = []string{}

		params, ok := buildCreateSourceParams(httptest.NewRecorder(), body)
		if !ok {
			t.Fatal("buildCreateSourceParams = false, want true")
		}

		if params.Categories != nil {
			t.Errorf("categories = %v, want nil (ARRAY[category] fallback)", params.Categories)
		}
	})
}

// TestBuildCreateSourceParams_SourceKind covers the 10-kind enum validation
// on the admin create path: unknown kinds are a 400, not a CHECK violation.
func TestBuildCreateSourceParams_SourceKind(t *testing.T) {
	t.Parallel()

	t.Run("rejects unknown kind", func(t *testing.T) {
		t.Parallel()

		body := validSourceCreateBody()
		kind := "robot"
		body.SourceKind = &kind

		rr := httptest.NewRecorder()
		if _, ok := buildCreateSourceParams(rr, body); ok {
			t.Fatal("buildCreateSourceParams = true, want false for unknown source_kind")
		}

		assertErrorCode(t, rr, http.StatusBadRequest, "invalid_source_kind")
	})

	t.Run("accepts a valid kind", func(t *testing.T) {
		t.Parallel()

		body := validSourceCreateBody()
		kind := "job_board"
		body.SourceKind = &kind

		params, ok := buildCreateSourceParams(httptest.NewRecorder(), body)
		if !ok {
			t.Fatal("buildCreateSourceParams = false, want true")
		}
		// SourceKind binds as *string (sqlc.narg → COALESCE 'publisher').
		sk, _ := params.SourceKind.(*string)
		if sk == nil || *sk != kind {
			t.Errorf("source_kind = %v, want %q", params.SourceKind, kind)
		}
	})

	t.Run("absent kind defaults to publisher", func(t *testing.T) {
		t.Parallel()

		params, ok := buildCreateSourceParams(httptest.NewRecorder(), validSourceCreateBody())
		if !ok {
			t.Fatal("buildCreateSourceParams = false, want true")
		}

		sk, _ := params.SourceKind.(*string)
		if sk == nil || *sk != "publisher" {
			t.Errorf("source_kind = %v, want publisher", params.SourceKind)
		}
	})
}

// --- live-DB: create/update persist categories -----------------------------

// TestAdminSourceCategories_LiveDB exercises the multi-category surface end
// to end against a live Postgres (JC_TEST_DATABASE_URL): create with
// categories[] persists the array + dual-writes category; PATCH categories
// replaces; PATCH without the key leaves the column untouched; an empty
// array clears to '{}'.
func TestAdminSourceCategories_LiveDB(t *testing.T) {
	h, svc, _, cleanup := newLiveChannelHandlers(t)
	defer cleanup()

	ctx := t.Context()

	// --- create: categories[] + no category → dual-write categories[0] ---
	createBody := `{
		"name": "Categories Admin Test Source",
		"source_url": "https://categories-test.example.com/feed",
		"categories": ["News", " job ", "NEWS"],
		"trust_level": 3,
		"crawl_method": "rss"
	}`
	rr := httptest.NewRecorder()
	r := authedChannelRequest(t, http.MethodPost, "/", "", createBody, true)
	h.HandleCreateSource(rr, r)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, want 201 (body=%s)", rr.Code, rr.Body.String())
	}

	created := decodeChannelData(t, rr)

	sourceIDStr, _ := created["id"].(string)
	if sourceIDStr == "" {
		t.Fatalf("create: no id in response (body=%s)", rr.Body.String())
	}

	sourceID := mustUUID(t, sourceIDStr)

	defer func() { _ = svc.Delete(ctx, sourceID) }()

	if created["category"] != "news" {
		t.Errorf("create: category = %v, want news (dual-write of categories[0])", created["category"])
	}

	assertStringArray(t, created["categories"], []string{"news", "job"})

	src, err := svc.Get(ctx, sourceID)
	if err != nil {
		t.Fatalf("get created source: %v", err)
	}

	if len(src.Categories) != 2 || src.Categories[0] != "news" || src.Categories[1] != "job" {
		t.Errorf("persisted categories = %v, want [news job]", src.Categories)
	}

	if src.Category != "news" {
		t.Errorf("persisted category = %q, want news", src.Category)
	}

	// --- patch: categories present → replace + dual-write category ---
	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodPatch, "/", sourceIDStr, `{"categories": ["Visa", "travel_alert"]}`, true)
	h.HandleUpdateSource(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("patch categories: status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}

	patched := decodeChannelData(t, rr)
	assertStringArray(t, patched["categories"], []string{"visa", "travel_alert"})

	if patched["category"] != "visa" {
		t.Errorf("patch: category = %v, want visa (dual-write)", patched["category"])
	}

	// --- patch: categories key absent → column untouched ---
	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodPatch, "/", sourceIDStr, `{"name": "Renamed Source"}`, true)
	h.HandleUpdateSource(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("patch name: status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}

	src, err = svc.Get(ctx, sourceID)
	if err != nil {
		t.Fatalf("get after name patch: %v", err)
	}

	if len(src.Categories) != 2 || src.Categories[0] != "visa" || src.Categories[1] != "travel_alert" {
		t.Errorf("categories after unrelated patch = %v, want [visa travel_alert] (untouched)", src.Categories)
	}

	// --- patch: explicit category alongside categories wins ---
	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodPatch, "/", sourceIDStr, `{"categories": ["news"], "category": "scholarship"}`, true)
	h.HandleUpdateSource(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("patch categories+category: status = %d (body=%s)", rr.Code, rr.Body.String())
	}

	src, err = svc.Get(ctx, sourceID)
	if err != nil {
		t.Fatalf("get after category patch: %v", err)
	}

	if src.Category != "scholarship" {
		t.Errorf("category = %q, want scholarship (explicit key wins)", src.Category)
	}

	if len(src.Categories) != 1 || src.Categories[0] != "news" {
		t.Errorf("categories = %v, want [news]", src.Categories)
	}

	// --- patch: empty array is an explicit clear to '{}' ---
	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodPatch, "/", sourceIDStr, `{"categories": []}`, true)
	h.HandleUpdateSource(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("patch clear: status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}

	src, err = svc.Get(ctx, sourceID)
	if err != nil {
		t.Fatalf("get after clear: %v", err)
	}

	if len(src.Categories) != 0 {
		t.Errorf("categories after clear = %v, want []", src.Categories)
	}

	if src.Category != "scholarship" {
		t.Errorf("category after clear = %q, want scholarship (untouched)", src.Category)
	}

	// --- patch: invalid source_kind → 400 before any write ---
	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodPatch, "/", sourceIDStr, `{"source_kind": "robot"}`, true)
	h.HandleUpdateSource(rr, r)
	assertErrorCode(t, rr, http.StatusBadRequest, "invalid_source_kind")

	// --- patch: valid source_kind applies ---
	rr = httptest.NewRecorder()
	r = authedChannelRequest(t, http.MethodPatch, "/", sourceIDStr, `{"source_kind": "job_board"}`, true)
	h.HandleUpdateSource(rr, r)

	if rr.Code != http.StatusOK {
		t.Fatalf("patch source_kind: status = %d (body=%s)", rr.Code, rr.Body.String())
	}

	src, err = svc.Get(ctx, sourceID)
	if err != nil {
		t.Fatalf("get after kind patch: %v", err)
	}

	if src.SourceKind != "job_board" {
		t.Errorf("source_kind = %q, want job_board", src.SourceKind)
	}
}

// assertStringArray asserts v is a []any of strings equal to want.
func assertStringArray(t *testing.T, v any, want []string) {
	t.Helper()

	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("value = %v (%T), want []any", v, v)
	}

	if len(arr) != len(want) {
		t.Fatalf("array = %v, want %v", arr, want)
	}

	for i := range want {
		if arr[i] != want[i] {
			t.Errorf("array[%d] = %v, want %q", i, arr[i], want[i])
		}
	}
}

// Compile-time check that the dedicated update params keep the direct-
// assignment shape (regresses if the query shape changes).
var _ = gen.UpdateIntelSourceCategoriesParams{Categories: []string{}}
