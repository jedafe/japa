package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/customfields"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// aggregateRollupValues — the pure aggregation core of computeRollup (§80).
// Every reviewer-flagged panic/NaN path is exercised here with fabricated
// value rows; no database needed.
// ---------------------------------------------------------------------------

func numericVal(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		t.Fatalf("scan numeric %q: %v", s, err)
	}
	return n
}

func valueRow(fieldID, entityID pgtype.UUID, fieldType string, num pgtype.Numeric, jsn []byte) gen.ListCustomFieldValuesWithDefsForEntitiesRow {
	return gen.ListCustomFieldValuesWithDefsForEntitiesRow{
		FieldID:     fieldID,
		EntityID:    entityID,
		ValueNumber: num,
		ValueJson:   jsn,
		FieldType:   fieldType,
	}
}

func rollupDefs(t *testing.T, defs map[string]rollupFieldDef) (map[pgtype.UUID]rollupFieldDef, map[string]rollupFieldDef, []string, map[string]pgtype.UUID) {
	t.Helper()
	byID := map[pgtype.UUID]rollupFieldDef{}
	byKey := map[string]rollupFieldDef{}
	order := []string{}
	ids := map[string]pgtype.UUID{}
	for name, def := range defs {
		id := pgtype.UUID{Valid: true, Bytes: [16]byte{byte(len(ids) + 1)}}
		ids[name] = id
		byID[id] = def
		if _, seen := byKey[def.key]; !seen {
			byKey[def.key] = def
			order = append(order, def.key)
		}
	}
	return byID, byKey, order, ids
}

func allMetrics() map[string]bool {
	return map[string]bool{"min": true, "avg": true, "max": true}
}

func fieldByKey(t *testing.T, fields []WikiRollupFieldDTO, key string) WikiRollupFieldDTO {
	t.Helper()
	for _, f := range fields {
		if f.Key == key {
			return f
		}
	}
	t.Fatalf("field %q missing from rollup output (%d fields)", key, len(fields))
	return WikiRollupFieldDTO{}
}

func TestAggregateRollupValues_Scalars(t *testing.T) {
	entityA, entityB := pgtype.UUID{Valid: true, Bytes: [16]byte{0xAA}}, pgtype.UUID{Valid: true, Bytes: [16]byte{0xBB}}
	byID, byKey, order, ids := rollupDefs(t, map[string]rollupFieldDef{
		"rent": {key: "rent", label: "Rent", typ: customfields.FieldTypeNumber},
	})

	vals := []gen.ListCustomFieldValuesWithDefsForEntitiesRow{
		valueRow(ids["rent"], entityA, customfields.FieldTypeNumber, numericVal(t, "1000"), nil),
		valueRow(ids["rent"], entityB, customfields.FieldTypeNumber, numericVal(t, "2000"), nil),
	}

	fields, rows := aggregateRollupValues(vals, byID, byKey, order, allMetrics())
	f := fieldByKey(t, fields, "rent")
	if f.Samples != 2 || *f.Metrics.Min != 1000 || *f.Metrics.Avg != 1500 || *f.Metrics.Max != 2000 {
		t.Fatalf("scalar aggregate wrong: %+v", f)
	}
	if len(rows) != 2 || rows[entityA]["rent"] == nil {
		t.Fatalf("rowsBySpace missing values: %#v", rows)
	}
}

func TestAggregateRollupValues_NullScalar(t *testing.T) {
	entityA := pgtype.UUID{Valid: true, Bytes: [16]byte{0xAA}}
	byID, byKey, order, ids := rollupDefs(t, map[string]rollupFieldDef{
		"rent": {key: "rent", label: "Rent", typ: customfields.FieldTypeNumber},
	})

	// Cleared value: row exists but value_number is NULL — must not panic
	// and must not count as a sample.
	vals := []gen.ListCustomFieldValuesWithDefsForEntitiesRow{
		valueRow(ids["rent"], entityA, customfields.FieldTypeNumber, pgtype.Numeric{}, nil),
	}

	fields, rows := aggregateRollupValues(vals, byID, byKey, order, allMetrics())
	if len(fields) != 0 {
		t.Fatalf("NULL-only field should be skipped, got %+v", fields)
	}
	if len(rows[entityA]) != 1 {
		t.Fatalf("row value should still surface in children: %#v", rows)
	}
}

func TestAggregateRollupValues_PriceRangePartials(t *testing.T) {
	entityA := pgtype.UUID{Valid: true, Bytes: [16]byte{0xAA}}
	entityB := pgtype.UUID{Valid: true, Bytes: [16]byte{0xBB}}
	entityC := pgtype.UUID{Valid: true, Bytes: [16]byte{0xCC}}
	entityD := pgtype.UUID{Valid: true, Bytes: [16]byte{0xDD}}

	cases := []struct {
		name     string
		payloads []string
		wantMin  *float64
		wantAvg  *float64
		wantMax  *float64
		wantN    int
	}{
		{"max only", []string{`{"max":10}`}, nil, nil, f64ptr(10), 1},
		{"min and max, no avg", []string{`{"min":1,"max":2}`}, f64ptr(1), nil, f64ptr(2), 1},
		{"avg only", []string{`{"avg":5}`}, f64ptr(5), f64ptr(5), f64ptr(5), 1},
		{"full triple", []string{`{"min":1,"avg":2,"max":3}`}, f64ptr(1), f64ptr(2), f64ptr(3), 1},
		{"mixed partials merge", []string{`{"max":10}`, `{"min":2,"avg":4}`}, f64ptr(2), f64ptr(4), f64ptr(10), 2},
		{"malformed object ignored", []string{`{"foo":1}`}, nil, nil, nil, 0},
		{"non-object ignored", []string{`[1,2]`}, nil, nil, nil, 0},
		{"null members ignored", []string{`{"min":null,"avg":7}`}, f64ptr(7), f64ptr(7), f64ptr(7), 1},
	}

	byID, byKey, order, ids := rollupDefs(t, map[string]rollupFieldDef{
		"price": {key: "price", label: "Price", typ: customfields.FieldTypePriceRange},
	})
	entities := []pgtype.UUID{entityA, entityB, entityC, entityD}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vals := make([]gen.ListCustomFieldValuesWithDefsForEntitiesRow, 0, len(tc.payloads))
			for i, p := range tc.payloads {
				vals = append(vals, valueRow(ids["price"], entities[i], customfields.FieldTypePriceRange, pgtype.Numeric{}, []byte(p)))
			}

			fields, _ := aggregateRollupValues(vals, byID, byKey, order, allMetrics())

			if tc.wantN == 0 {
				if len(fields) != 0 {
					t.Fatalf("expected no field output, got %+v", fields)
				}
				return
			}

			f := fieldByKey(t, fields, "price")
			if f.Samples != tc.wantN {
				t.Fatalf("samples: got %d want %d", f.Samples, tc.wantN)
			}
			checkPtr(t, "min", f.Metrics.Min, tc.wantMin)
			checkPtr(t, "avg", f.Metrics.Avg, tc.wantAvg)
			checkPtr(t, "max", f.Metrics.Max, tc.wantMax)
		})
	}
}

func checkPtr(t *testing.T, name string, got, want *float64) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("%s: got %v want nil", name, *got)
		}
		return
	}
	if got == nil || *got != *want {
		t.Fatalf("%s: got %v want %v", name, got, *want)
	}
}

func TestAggregateRollupValues_NonFiniteAndMarshal(t *testing.T) {
	entityA := pgtype.UUID{Valid: true, Bytes: [16]byte{0xAA}}
	entityB := pgtype.UUID{Valid: true, Bytes: [16]byte{0xBB}}
	byID, byKey, order, ids := rollupDefs(t, map[string]rollupFieldDef{
		"score": {key: "score", label: "Score", typ: customfields.FieldTypeNumber},
	})

	// pg numeric accepts 'NaN' — a stored NaN must be dropped rather than
	// poison avgOf or fail json.Marshal on the page response.
	vals := []gen.ListCustomFieldValuesWithDefsForEntitiesRow{
		valueRow(ids["score"], entityA, customfields.FieldTypeNumber, pgtype.Numeric{NaN: true, Valid: true}, nil),
		valueRow(ids["score"], entityB, customfields.FieldTypeNumber, numericVal(t, "4"), nil),
	}

	fields, _ := aggregateRollupValues(vals, byID, byKey, order, allMetrics())
	f := fieldByKey(t, fields, "score")
	if f.Samples != 1 {
		t.Fatalf("NaN row counted as a sample: %+v", f)
	}
	for name, p := range map[string]*float64{"min": f.Metrics.Min, "avg": f.Metrics.Avg, "max": f.Metrics.Max} {
		if p != nil && math.IsNaN(*p) {
			t.Fatalf("%s emitted NaN", name)
		}
	}
	if _, err := json.Marshal(fields); err != nil {
		t.Fatalf("rollup output failed to marshal: %v", err)
	}
}

func TestAggregateRollupValues_MixedTypeKey(t *testing.T) {
	entityA := pgtype.UUID{Valid: true, Bytes: [16]byte{0xAA}}
	entityB := pgtype.UUID{Valid: true, Bytes: [16]byte{0xBB}}
	byID, byKey, order, ids := rollupDefs(t, map[string]rollupFieldDef{
		"price_num":   {key: "price", label: "Price (number)", typ: customfields.FieldTypeNumber},
		"price_range": {key: "price", label: "Price (range)", typ: customfields.FieldTypePriceRange},
	})

	vals := []gen.ListCustomFieldValuesWithDefsForEntitiesRow{
		valueRow(ids["price_num"], entityA, customfields.FieldTypeNumber, numericVal(t, "100"), nil),
		valueRow(ids["price_range"], entityB, customfields.FieldTypePriceRange, pgtype.Numeric{}, []byte(`{"min":50,"max":200}`)),
	}

	// map iteration order varies — run both orders; the first-seen def
	// (insertion order into defsByKey) always owns the emitted metadata.
	fields, _ := aggregateRollupValues(vals, byID, byKey, order, allMetrics())
	f := fieldByKey(t, fields, "price")
	if f.Samples < 1 {
		t.Fatalf("expected contributions under shared key, got %+v", f)
	}
}

// ---------------------------------------------------------------------------
// parseRollupConfig / validateRollupInput
// ---------------------------------------------------------------------------

func TestParseRollupConfig(t *testing.T) {
	if parseRollupConfig(nil) != nil {
		t.Fatal("nil config should parse to nil")
	}
	if parseRollupConfig([]byte(`{not json`)) != nil {
		t.Fatal("malformed config should parse to nil")
	}
	if parseRollupConfig([]byte(`{"enabled":false,"metrics":["min"]}`)) != nil {
		t.Fatal("disabled config should parse to nil")
	}

	cfg := parseRollupConfig([]byte(`{"enabled":true,"metrics":["min","max"],"levels":["city"],"section_limit":8}`))
	if cfg == nil || !cfg.Enabled || cfg.SectionLimit != 8 || len(cfg.Levels) != 1 {
		t.Fatalf("valid config misparsed: %+v", cfg)
	}
}

func TestParseRollupConfig_DirectoryMode(t *testing.T) {
	cfg := parseRollupConfig([]byte(`{"enabled":true,"mode":"directory","levels":["state"]}`))
	if cfg == nil || cfg.Mode != "directory" || len(cfg.Levels) != 1 {
		t.Fatalf("directory config misparsed: %+v", cfg)
	}
	// Aggregate stays the default mode when absent.
	cfg = parseRollupConfig([]byte(`{"enabled":true,"metrics":["min"]}`))
	if cfg == nil || cfg.Mode != "" {
		t.Fatalf("absent mode should parse as empty, got %+v", cfg)
	}
}

func TestValidateRollupInput(t *testing.T) {
	if _, err := validateRollupInput(json.RawMessage(`{"enabled":true,"section_limit":-1}`)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("negative section_limit: want ErrInvalidInput, got %v", err)
	}
	if _, err := validateRollupInput(json.RawMessage(`{"enabled":true,"metrics":["p99"]}`)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown metric: want ErrInvalidInput, got %v", err)
	}
	out, err := validateRollupInput(json.RawMessage(`{"enabled":true,"metrics":["avg"],"section_limit":8}`))
	if err != nil || out == nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	out, err = validateRollupInput(json.RawMessage(`null`))
	if err != nil || out != nil {
		t.Fatalf("null should clear config: out=%s err=%v", out, err)
	}

	if _, err := validateRollupInput(json.RawMessage(`{"enabled":true,"mode":"sidebar"}`)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown mode: want ErrInvalidInput, got %v", err)
	}
	if _, err := validateRollupInput(json.RawMessage(`{"enabled":true,"mode":"directory","levels":["state","university"]}`)); err != nil {
		t.Fatalf("directory config rejected: %v", err)
	}
}

// ---------------------------------------------------------------------------
// normalizePriceRange (§80 Itemized Pricing)
// ---------------------------------------------------------------------------

func TestNormalizePriceRange(t *testing.T) {
	ok := func(s string) map[string]float64 {
		t.Helper()
		out, err := normalizePriceRange(json.RawMessage(s), "price")
		if err != nil {
			t.Fatalf("normalizePriceRange(%s): %v", s, err)
		}
		var m map[string]float64
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatalf("normalized output not a number map: %s", s)
		}
		return m
	}
	bad := func(s string) {
		t.Helper()
		if _, err := normalizePriceRange(json.RawMessage(s), "price"); err == nil {
			t.Fatalf("normalizePriceRange(%s) should reject", s)
		}
	}

	if m := ok(`{"avg":2.29,"min":1.79,"max":2.99}`); len(m) != 3 || m["avg"] != 2.29 {
		t.Fatalf("full triple wrong: %v", m)
	}
	if m := ok(`{"min":1,"max":2}`); len(m) != 2 {
		t.Fatalf("partial {min,max} wrong: %v", m)
	}
	if m := ok(`{"max":10}`); len(m) != 1 || m["max"] != 10 {
		t.Fatalf("max-only wrong: %v", m)
	}
	if m := ok(`{"avg":5}`); len(m) != 1 {
		t.Fatalf("avg-only wrong: %v", m)
	}
	if m := ok(`{"min":1,"currency":"CAD"}`); len(m) != 1 || m["min"] != 1 {
		t.Fatalf("stray keys must be dropped: %v", m)
	}

	bad(`{}`)                    // no recognized key
	bad(`{"foo":1}`)             // unknown-only object
	bad(`{"min":"cheap"}`)       // non-numeric bound
	bad(`{"min":3,"max":1}`)     // min > max
	bad(`5`)                     // not an object
	bad(`{"min":null}`)          // explicit nulls count as absent
	bad(`{"avg":null,"x":true}`) // null + stray key
}

// ---------------------------------------------------------------------------
// fieldSections — §80 section-grouped schema ordering
// ---------------------------------------------------------------------------

func TestFieldSections(t *testing.T) {
	fld := func(key, section string) EntityFieldDTO {
		return EntityFieldDTO{Key: key, Section: section}
	}

	if got := fieldSections(nil); got != nil {
		t.Fatalf("nil input should yield nil, got %+v", got)
	}
	if got := fieldSections([]EntityFieldDTO{fld("a", ""), fld("b", "")}); got != nil {
		t.Fatalf("single implicit section should yield nil, got %+v", got)
	}
	if got := fieldSections([]EntityFieldDTO{fld("a", "food"), fld("b", "food")}); got != nil {
		t.Fatalf("single named section should yield nil, got %+v", got)
	}

	// First-seen order preserved; "" bucket promoted to front.
	got := fieldSections([]EntityFieldDTO{
		fld("rent", "housing"), fld("note", ""), fld("milk", "food"), fld("bread", "food"),
	})
	if len(got) != 3 || got[0].Key != "" || got[1].Key != "housing" || got[2].Key != "food" {
		t.Fatalf("section order wrong: %+v", got)
	}
	if len(got[2].Fields) != 2 || got[2].Fields[0].Key != "milk" {
		t.Fatalf("section members wrong: %+v", got[2].Fields)
	}
}

// ---------------------------------------------------------------------------
// validateFieldDef — section token gate (§80)
// ---------------------------------------------------------------------------

func TestValidateFieldDef_SectionToken(t *testing.T) {
	good := FieldDefInput{Key: "milk_1l", Label: "Milk 1L", FieldType: customfields.FieldTypePriceRange, Section: "food_groceries"}
	if err := validateFieldDef(&good, "field", false); err != nil {
		t.Fatalf("valid section rejected: %v", err)
	}

	for _, section := range []string{"Food", "has space", "with-dash", "semi;colon", "x!"} {
		f := FieldDefInput{Key: "k", Label: "L", FieldType: customfields.FieldTypeNumber, Section: section}
		if err := validateFieldDef(&f, "field", false); err == nil {
			t.Fatalf("invalid section %q accepted", section)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration: siblings exclude soft-deleted pages; page-template lookup is
// category-scoped; template delete guards (system + prose pages).
// ---------------------------------------------------------------------------

// hierarchyFixture seeds a wiki type (fresh space_type + wiki_type_configs
// row), two categories, and a field group per category template.
type hierarchyFixture struct {
	typeID   pgtype.UUID
	catAID   pgtype.UUID // category 'cost_of_living'
	catBID   pgtype.UUID // category 'other'
	groupIDs []pgtype.UUID
}

func seedHierarchyFixture(t *testing.T, ctx context.Context, database *db.DB) hierarchyFixture {
	t.Helper()
	uniq := strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:12]

	mk := func(q string, args ...any) pgtype.UUID { return mustIDF(t, ctx, database, q, args...) }

	fx := hierarchyFixture{}
	fx.typeID = mk(`INSERT INTO space_types (id,name,slug) VALUES (gen_random_uuid(),$1,$1) RETURNING id`, "wop-hier-"+uniq)
	mk(`INSERT INTO wiki_type_configs (space_type_id,enabled,label) VALUES ($1,true,$2) RETURNING space_type_id`,
		fx.typeID, "Wop Hierarchy "+uniq)

	mkGroup := func(suffix string) pgtype.UUID {
		return mk(`INSERT INTO custom_field_groups (name,slug,is_system) VALUES ($1,$2,false) RETURNING id`,
			"Wop Group "+uniq+suffix, "wop_hier_"+uniq+suffix)
	}
	gA, gB := mkGroup("_a"), mkGroup("_b")
	fx.groupIDs = []pgtype.UUID{gA, gB}

	// Category slugs use the dash form on purpose — the resolution query
	// must bind both the raw category token ('cost_of_living') and the
	// dash/underscore-normalized slug ('cost-of-living').
	fx.catAID = mk(`INSERT INTO wiki_type_categories (space_type_id,slug,category,label,field_group_id)
		VALUES ($1,'cost-of-living','cost_of_living','Cost of Living',$2) RETURNING id`, fx.typeID, gA)
	fx.catBID = mk(`INSERT INTO wiki_type_categories (space_type_id,slug,category,label,field_group_id)
		VALUES ($1,'other','other','Other',$2) RETURNING id`, fx.typeID, gB)

	t.Cleanup(func() {
		cctx := context.Background()
		// categories cascade with the type config row; groups must outlive
		// nothing else — delete them explicitly (ON DELETE RESTRICT).
		database.Writer.Exec(cctx, `DELETE FROM wiki_type_configs WHERE space_type_id = $1`, fx.typeID)
		database.Writer.Exec(cctx, `DELETE FROM space_types WHERE id = $1`, fx.typeID)
		for _, g := range fx.groupIDs {
			database.Writer.Exec(cctx, `DELETE FROM custom_field_groups WHERE id = $1`, g)
		}
	})

	return fx
}

// TestListPageSiblings_ExcludesSoftDeleted — a tombstoned page must not
// appear in Prev/Next (review fix: deleted_at IS NULL).
func TestListPageSiblings_ExcludesSoftDeleted(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	fx := seedOpsFixture(t, ctx, database)

	cat := "wopsib_" + strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:8]
	mk := func(slug string) pgtype.UUID {
		return mustIDF(t, ctx, database,
			`INSERT INTO wiki_pages (id,space_id,slug,title,category,created_by) VALUES (gen_random_uuid(),$1,$2,$3,$4,$5) RETURNING id`,
			fx.spaceID, slug, slug, cat, fx.userID)
	}
	live1, dead, live2 := mk("sib-a"), mk("sib-b"), mk("sib-c")
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(),
			`DELETE FROM wiki_pages WHERE space_id = $1 AND category = $2`, fx.spaceID, cat)
	})

	if err := store.DeleteWikiPage(ctx, dead); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	siblings, err := store.ListPageSiblings(ctx, fx.spaceID, cat)
	if err != nil {
		t.Fatalf("ListPageSiblings: %v", err)
	}
	if len(siblings) != 2 {
		t.Fatalf("siblings: got %d want 2: %+v", len(siblings), siblings)
	}
	for _, s := range siblings {
		if s.Slug == "sib-b" {
			t.Fatal("soft-deleted page leaked into siblings")
		}
	}
	_ = live1
	_ = live2
}

// TestGetPageTemplateForSpace_CategoryScoped — same template slug on two
// categories must resolve per (category, slug), not space-wide.
func TestGetPageTemplateForSpace_CategoryScoped(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)

	// The store takes a space id (resolution walks space → space_type_id
	// → category row), so seed a space of the fixture type.
	spaceID := mustIDF(t, ctx, database,
		`INSERT INTO spaces (id,space_type_id,name,slug,level,visibility,status)
		 VALUES (gen_random_uuid(),$1,$2,$2,'custom','public','active') RETURNING id`,
		hf.typeID, "wop-tmpl-"+strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:12])
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM spaces WHERE id = $1`, spaceID)
	})

	tmplA, err := store.CreatePageTemplate(ctx, hf.catAID, "prices", "Prices in {name}", hf.groupIDs[0], 0, false)
	if err != nil {
		t.Fatalf("create template A: %v", err)
	}
	tmplB, err := store.CreatePageTemplate(ctx, hf.catBID, "prices", "Prices alt", hf.groupIDs[1], 0, false)
	if err != nil {
		t.Fatalf("create template B: %v", err)
	}
	t.Cleanup(func() {
		cctx := context.Background()
		database.Writer.Exec(cctx, `DELETE FROM wiki_type_page_templates WHERE id IN ($1,$2)`, tmplA.ID, tmplB.ID)
	})

	gotA, err := store.GetPageTemplateForSpace(ctx, spaceID, "cost_of_living", "prices")
	if err != nil {
		t.Fatalf("resolve (cost_of_living, prices): %v", err)
	}
	if gotA.ID != tmplA.ID || gotA.Label != "Prices in {name}" {
		t.Fatalf("wrong template bound for cost_of_living: %+v", gotA)
	}

	gotB, err := store.GetPageTemplateForSpace(ctx, spaceID, "other", "prices")
	if err != nil {
		t.Fatalf("resolve (other, prices): %v", err)
	}
	if gotB.ID != tmplB.ID {
		t.Fatalf("category scoping failed — resolved wrong category's template: %+v", gotB)
	}

	// The dash-form token binds through the normalized slug arm
	// (c.slug 'cost-of-living' → 'cost_of_living' ≠ input, but the
	// category token 'cost_of_living' also matches — both predicates
	// cover the two spellings a page may carry).
	gotNorm, err := store.GetPageTemplateForSpace(ctx, spaceID, "cost-of-living", "prices")
	if err != nil || gotNorm.ID != tmplA.ID {
		t.Fatalf("normalized slug token: got %+v err %v", gotNorm, err)
	}

	if _, err := store.GetPageTemplateForSpace(ctx, spaceID, "nonexistent_category", "prices"); !errors.Is(err, ErrWikiPageTemplateNotFound) {
		t.Fatalf("unknown category: want ErrWikiPageTemplateNotFound, got %v", err)
	}
}

// TestDeleteWikiPageTemplate_Guards — system templates are protected and a
// template bound to a page carrying approved prose cannot be deleted.
func TestDeleteWikiPageTemplate_Guards(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	fx := seedOpsFixture(t, ctx, database)

	svc := NewService(store, nil)

	sys, err := store.CreatePageTemplate(ctx, hf.catAID, "system-t", "System", hf.groupIDs[0], 0, true)
	if err != nil {
		t.Fatalf("create system template: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_type_page_templates WHERE id = $1`, sys.ID)
	})

	if err := svc.DeleteWikiPageTemplate(ctx, sys.ID.String()); !errors.Is(err, ErrWikiPageTemplateProtected) {
		t.Fatalf("system template: want ErrWikiPageTemplateProtected, got %v", err)
	}

	// Bound page with an approved content revision blocks deletion.
	prose, err := store.CreatePageTemplate(ctx, hf.catAID, "prose-t", "Prose", hf.groupIDs[0], 0, false)
	if err != nil {
		t.Fatalf("create prose template: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_type_page_templates WHERE id = $1`, prose.ID)
	})

	pageID := mustIDF(t, ctx, database,
		`INSERT INTO wiki_pages (id,space_id,slug,title,category,created_by,page_template_id)
		 VALUES (gen_random_uuid(),$1,$2,'Bound','cost_of_living',$3,$4) RETURNING id`,
		fx.spaceID, "wop-bound-"+strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:8], fx.userID, prose.ID)
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_pages WHERE id = $1`, pageID)
	})

	revID, err := store.SubmitRevision(ctx, pageID, fx.userID, "content",
		[]byte(`{"type":"doc","content":[{"type":"paragraph"}]}`), nil, "prose", nil, pgtype.UUID{})
	if err != nil {
		t.Fatalf("submit revision: %v", err)
	}
	if _, err := store.ApproveRevisionByID(ctx, revID, fx.userID, ""); err != nil {
		t.Fatalf("approve revision: %v", err)
	}

	if err := svc.DeleteWikiPageTemplate(ctx, prose.ID.String()); !errors.Is(err, ErrWikiPageTemplateInUse) {
		t.Fatalf("prose-bound template: want ErrWikiPageTemplateInUse, got %v", err)
	}

	// A clean template deletes normally.
	free, err := store.CreatePageTemplate(ctx, hf.catBID, "free-t", "Free", hf.groupIDs[1], 0, false)
	if err != nil {
		t.Fatalf("create free template: %v", err)
	}
	if err := svc.DeleteWikiPageTemplate(ctx, free.ID.String()); err != nil {
		t.Fatalf("clean delete: %v", err)
	}
}

// ---------------------------------------------------------------------------
// §80 review-fix coverage: lazy template pages, materialization, the rollup
// template-group union, and the delete guard's field-values arm.
// ---------------------------------------------------------------------------

// allowAllPerms grants every RBAC node — tests that need seeded fields to
// stay visible through the fail-closed can_view path wire it via
// SetPermissionChecker.
type allowAllPerms struct{}

func (allowAllPerms) HasPermission(context.Context, pgtype.UUID, string) bool { return true }

// seedSpaceOfType inserts a space bound to the fixture's space type and
// registers cleanup.
func seedSpaceOfType(t *testing.T, ctx context.Context, database *db.DB, typeID pgtype.UUID, namePrefix string) (pgtype.UUID, string) {
	t.Helper()

	slug := "wop-" + namePrefix + "-" + strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:10]
	spaceID := mustIDF(t, ctx, database,
		`INSERT INTO spaces (id,space_type_id,name,slug,level,visibility,status)
		 VALUES (gen_random_uuid(),$1,$2,$3,'custom','public','active') RETURNING id`,
		typeID, namePrefix+" "+slug, slug)
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM spaces WHERE id = $1`, spaceID)
	})

	return spaceID, slug
}

// TestGetWikiPage_LazyTemplatePage — a declared (category, slug) page
// template renders without a wiki_pages row: lazy flag, substituted
// {name} title, bound group ids, and template discovery links. An unknown
// slug still returns pgx.ErrNoRows (real 404).
func TestGetWikiPage_LazyTemplatePage(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)

	spaceID, spaceSlug := seedSpaceOfType(t, ctx, database, hf.typeID, "lazy")

	tmpl, err := store.CreatePageTemplate(ctx, hf.catAID, "prices", "Prices in {name}", hf.groupIDs[0], 3, false)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_type_page_templates WHERE id = $1`, tmpl.ID)
	})

	svc := NewService(store, nil)
	svc.SetPermissionChecker(allowAllPerms{})
	svc.SetCustomFields(customfields.NewService(database))

	page, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "prices")
	if err != nil {
		t.Fatalf("lazy template page: %v", err)
	}
	if !page.Lazy {
		t.Fatal("want lazy=true on an unmaterialized template page")
	}
	// {name} must expand to the space's display name — not its level
	// ('custom' for seeded spaces). Loose prefix checks once let
	// "Prices in custom" pass when GetSpaceBySlug returned the level.
	wantName := "lazy " + spaceSlug
	if page.Title != "Prices in "+wantName || page.SpaceName != wantName {
		t.Fatalf("display name not substituted: title=%q space_name=%q want_name=%q",
			page.Title, page.SpaceName, wantName)
	}
	if page.Category != "cost_of_living" {
		t.Fatalf("category: got %q want cost_of_living", page.Category)
	}
	if page.PageTemplateID != tmpl.ID.String() {
		t.Fatalf("page_template_id: got %q want %s", page.PageTemplateID, tmpl.ID.String())
	}
	if page.FieldGroupID != hf.groupIDs[0].String() {
		t.Fatalf("field_group_id: got %q want %s", page.FieldGroupID, hf.groupIDs[0].String())
	}
	if page.Position != 3 {
		t.Fatalf("position: got %d want 3", page.Position)
	}
	if len(page.PageTemplates) == 0 {
		t.Fatal("page_templates discovery list empty")
	}
	for _, pt := range page.PageTemplates {
		if strings.Contains(pt.Label, "{name}") {
			t.Fatalf("literal {name} leaked in template label %q", pt.Label)
		}
	}

	// No wiki_pages row may exist for the lazy render.
	var n int
	if err := database.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM wiki_pages WHERE space_id = $1 AND slug = 'prices'`, spaceID).Scan(&n); err != nil {
		t.Fatalf("count pages: %v", err)
	}
	if n != 0 {
		t.Fatal("lazy render must not persist a wiki_pages row")
	}

	if _, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "no-such-template"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown slug: want pgx.ErrNoRows, got %v", err)
	}
}

// TestSubmitRevision_MaterializesLazyTemplatePage — the first prose
// submission persists the row bound to the resolved template (template id,
// field group, category, position, substituted title).
func TestSubmitRevision_MaterializesLazyTemplatePage(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	fx := seedOpsFixture(t, ctx, database)

	spaceID, spaceSlug := seedSpaceOfType(t, ctx, database, hf.typeID, "lazy")

	tmpl, err := store.CreatePageTemplate(ctx, hf.catAID, "prices", "Prices in {name}", hf.groupIDs[0], 2, false)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	t.Cleanup(func() {
		cctx := context.Background()
		database.Writer.Exec(cctx, `DELETE FROM wiki_type_page_templates WHERE id = $1`, tmpl.ID)
		database.Writer.Exec(cctx, `DELETE FROM wiki_pages WHERE space_id = $1 AND slug = 'prices'`, spaceID)
	})

	svc := NewService(store, nil)
	revID, err := svc.SubmitRevision(ctx, fx.userID, spaceSlug, "prices", SubmitRevisionInput{
		Content: []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Milk is cheap."}]}]}`),
	})
	if err != nil {
		t.Fatalf("submit on lazy page: %v", err)
	}
	if revID == "" {
		t.Fatal("empty revision id")
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_revisions WHERE wiki_page_id IN (SELECT id FROM wiki_pages WHERE space_id = $1 AND slug = 'prices')`, spaceID)
	})

	var (
		tmplID   pgtype.UUID
		groupID  pgtype.UUID
		category string
		title    string
		position int32
	)
	if err := database.Reader.QueryRow(ctx,
		`SELECT page_template_id, field_group_id, category, title, position
		 FROM wiki_pages WHERE space_id = $1 AND slug = 'prices'`, spaceID).
		Scan(&tmplID, &groupID, &category, &title, &position); err != nil {
		t.Fatalf("materialized row: %v", err)
	}
	if tmplID != tmpl.ID {
		t.Fatalf("page_template_id: got %v want %v", tmplID, tmpl.ID)
	}
	if groupID != hf.groupIDs[0] {
		t.Fatalf("field_group_id: got %v want %v", groupID, hf.groupIDs[0])
	}
	if category != "cost_of_living" {
		t.Fatalf("category: got %q want cost_of_living", category)
	}
	if strings.Contains(title, "{name}") {
		t.Fatalf("materialized title still carries the token: %q", title)
	}
	if position != 2 {
		t.Fatalf("position: got %d want 2", position)
	}
}

// TestSubmitPageFieldOps_ValidatesBeforeMaterializing — §80 ordering fix:
// the page-slug-keyed ops path validates BEFORE resolving the lazy page, so
// a rejected submission never leaves an empty wiki_pages stub; a valid one
// still materializes it bound to the template.
func TestSubmitPageFieldOps_ValidatesBeforeMaterializing(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	fx := seedOpsFixture(t, ctx, database)
	uniq := strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:8]

	spaceID, spaceSlug := seedSpaceOfType(t, ctx, database, hf.typeID, "lazyops")

	tmpl, err := store.CreatePageTemplate(ctx, hf.catAID, "prices", "Prices in {name}", hf.groupIDs[0], 0, false)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	t.Cleanup(func() {
		cctx := context.Background()
		database.Writer.Exec(cctx, `DELETE FROM wiki_type_page_templates WHERE id = $1`, tmpl.ID)
		database.Writer.Exec(cctx, `DELETE FROM wiki_revisions WHERE wiki_page_id IN (SELECT id FROM wiki_pages WHERE space_id = $1 AND slug = 'prices')`, spaceID)
		database.Writer.Exec(cctx, `DELETE FROM wiki_pages WHERE space_id = $1 AND slug = 'prices'`, spaceID)
	})

	svc := NewService(store, nil)
	svc.SetPermissionChecker(allowAllPerms{})
	svc.SetCustomFields(customfields.NewService(database))

	countPages := func() int {
		var n int
		if err := database.Reader.QueryRow(ctx,
			`SELECT COUNT(*) FROM wiki_pages WHERE space_id = $1 AND slug = 'prices'`, spaceID).Scan(&n); err != nil {
			t.Fatalf("count pages: %v", err)
		}
		return n
	}

	// Rejected ops (unknown op kind) — validation must fire before the lazy
	// page resolves; no stub row may exist afterward.
	if _, _, err := svc.SubmitPageFieldOps(ctx, fx.userID, spaceSlug, "prices", nil,
		[]FieldOp{{Op: "bogus"}}, "", false); err == nil {
		t.Fatal("want validation error on unknown op")
	}
	if n := countPages(); n != 0 {
		t.Fatalf("rejected ops submission materialized the lazy page (%d rows)", n)
	}

	// Valid ops — the page materializes and the pending revision lands on it.
	revID, autoApproved, err := svc.SubmitPageFieldOps(ctx, fx.userID, spaceSlug, "prices", nil,
		[]FieldOp{{
			Op:          OpProposeGroup,
			EntityClass: "space",
			Group:       &GroupDefInput{Slug: "wop_lazyops_" + uniq, Name: "Lazy Ops " + uniq},
		}}, "", false)
	if err != nil {
		t.Fatalf("valid ops on lazy page: %v", err)
	}
	if revID == "" || autoApproved {
		t.Fatalf("want pending revision id, got rev=%q auto=%v", revID, autoApproved)
	}
	if n := countPages(); n != 1 {
		t.Fatalf("valid ops submission should materialize the page once, got %d rows", n)
	}
}

// TestComputeRollup_IncludesTemplateGroupFields — the flagship fix: a
// price_range field living only in the descendant type's page-template
// group (e.g. wiki_city_prices under cost_of_living) must appear in the
// parent page's rollup fields and rows.
func TestComputeRollup_IncludesTemplateGroupFields(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	uniq := strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:8]

	mk := func(q string, args ...any) pgtype.UUID { return mustIDF(t, ctx, database, q, args...) }

	// Descendant type with its own cost_of_living category. Its category
	// group is empty — the price field lives ONLY on the page-template
	// group, mirroring the seeded wiki_city_prices layout.
	childTypeID := mk(`INSERT INTO space_types (id,name,slug) VALUES (gen_random_uuid(),$1,$1) RETURNING id`, "wop-child-"+uniq)
	mk(`INSERT INTO wiki_type_configs (space_type_id,enabled,label) VALUES ($1,true,$2) RETURNING space_type_id`,
		childTypeID, "Wop Child "+uniq)
	childCatGroup := mk(`INSERT INTO custom_field_groups (name,slug,is_system) VALUES ($1,$2,false) RETURNING id`,
		"Wop Child Cat "+uniq, "wop_child_cat_"+uniq)
	tmplGroup := mk(`INSERT INTO custom_field_groups (name,slug,is_system) VALUES ($1,$2,false) RETURNING id`,
		"Wop Child Prices "+uniq, "wop_child_prices_"+uniq)
	childCat := mk(`INSERT INTO wiki_type_categories (space_type_id,slug,category,label,field_group_id)
		VALUES ($1,'cost-of-living','cost_of_living','Cost of Living',$2) RETURNING id`, childTypeID, childCatGroup)
	tmpl, err := store.CreatePageTemplate(ctx, childCat, "prices", "Prices in {name}", tmplGroup, 0, false)
	if err != nil {
		t.Fatalf("create child template: %v", err)
	}
	milkField := mk(`INSERT INTO custom_fields (group_id,key,label,field_type,section,position)
		VALUES ($1,'milk_1l','Milk 1L','price_range','groceries',0) RETURNING id`, tmplGroup)

	parentSpace, _ := seedSpaceOfType(t, ctx, database, hf.typeID, "parent")
	childSpace, _ := seedSpaceOfType(t, ctx, database, childTypeID, "child")
	if _, err := database.Writer.Exec(ctx,
		`UPDATE spaces SET parent_id = $1 WHERE id = $2`, parentSpace, childSpace); err != nil {
		t.Fatalf("link child space: %v", err)
	}
	mk(`INSERT INTO custom_field_values (field_id,entity_type,entity_id,value_json)
		VALUES ($1,'space',$2,'{"avg":2.29}') RETURNING id`, milkField, childSpace)

	t.Cleanup(func() {
		cctx := context.Background()
		database.Writer.Exec(cctx, `DELETE FROM wiki_type_page_templates WHERE id = $1`, tmpl.ID)
		database.Writer.Exec(cctx, `DELETE FROM wiki_type_configs WHERE space_type_id = $1`, childTypeID)
		database.Writer.Exec(cctx, `DELETE FROM space_types WHERE id = $1`, childTypeID)
		for _, g := range []pgtype.UUID{childCatGroup, tmplGroup} {
			database.Writer.Exec(cctx, `DELETE FROM custom_field_groups WHERE id = $1`, g)
		}
	})

	svc := NewService(store, nil)
	svc.SetPermissionChecker(allowAllPerms{})
	svc.SetCustomFields(customfields.NewService(database))

	cfg := &WikiRollupConfig{Enabled: true, Metrics: []string{"min", "avg", "max"}, Levels: []string{"wop-child-" + uniq}}
	rollup := svc.computeRollup(ctx, pgtype.UUID{}, parentSpace, hf.typeID, cfg, "cost_of_living")
	if rollup == nil {
		t.Fatal("rollup is nil — descendants or source fields unresolved")
	}

	f := fieldByKey(t, rollup.Fields, "milk_1l")
	if f.FieldType != customfields.FieldTypePriceRange {
		t.Fatalf("field_type: got %q want price_range", f.FieldType)
	}
	if f.Section != "groceries" {
		t.Fatalf("section: got %q want groceries", f.Section)
	}
	if f.Metrics.Avg == nil || *f.Metrics.Avg != 2.29 {
		t.Fatalf("avg metric: got %+v want 2.29", f.Metrics.Avg)
	}
	// {avg}-only record stands in for both bounds (per-record semantics).
	if f.Metrics.Min == nil || *f.Metrics.Min != 2.29 || f.Metrics.Max == nil || *f.Metrics.Max != 2.29 {
		t.Fatalf("avg-standin bounds: min %+v max %+v", f.Metrics.Min, f.Metrics.Max)
	}
	if len(rollup.Rows) != 1 || rollup.Rows[0].Values["milk_1l"] == nil {
		t.Fatalf("row set: %+v", rollup.Rows)
	}
}

// TestAggregateRollupValues_AvgStandinMixed — per-record standin: an
// {avg}-only record's avg counts toward min/max even when another record
// declares explicit bounds ({min:10} + {avg:2} → min=2, not 10).
func TestAggregateRollupValues_AvgStandinMixed(t *testing.T) {
	byID, byKey, order, ids := rollupDefs(t, map[string]rollupFieldDef{
		"price": {key: "price", label: "Price", typ: customfields.FieldTypePriceRange},
	})
	entA := pgtype.UUID{Valid: true, Bytes: [16]byte{1}}
	entB := pgtype.UUID{Valid: true, Bytes: [16]byte{2}}

	vals := []gen.ListCustomFieldValuesWithDefsForEntitiesRow{
		valueRow(ids["price"], entA, customfields.FieldTypePriceRange, pgtype.Numeric{}, []byte(`{"min":10}`)),
		valueRow(ids["price"], entB, customfields.FieldTypePriceRange, pgtype.Numeric{}, []byte(`{"avg":2}`)),
	}

	fields, _ := aggregateRollupValues(vals, byID, byKey, order, allMetrics())
	f := fieldByKey(t, fields, "price")
	if f.Metrics.Min == nil || *f.Metrics.Min != 2 {
		t.Fatalf("min: got %+v want 2 (avg stands in per record)", f.Metrics.Min)
	}
	if f.Metrics.Max == nil || *f.Metrics.Max != 2 {
		t.Fatalf("max: got %+v want 2 (avg stands in per record)", f.Metrics.Max)
	}
	if f.Metrics.Avg == nil || *f.Metrics.Avg != 2 {
		t.Fatalf("avg: got %+v want 2", f.Metrics.Avg)
	}
	if f.Samples != 2 {
		t.Fatalf("samples: got %d want 2", f.Samples)
	}
}

// ---------------------------------------------------------------------------
// Directory mode (§80 child-type directories)
// ---------------------------------------------------------------------------

// TestDirectoryChildRows_SortAndCap — the listing helper orders by depth
// then name (slug tie-break) and truncates at the limit, deterministically
// dropping the tail rather than an arbitrary subset.
func TestDirectoryChildRows_SortAndCap(t *testing.T) {
	mkID := func(b byte) pgtype.UUID {
		var u pgtype.UUID
		u.Valid = true
		u.Bytes[15] = b
		return u
	}
	strPtr := func(s string) *string { return &s }

	children := []WikiDirectoryChild{
		{ID: mkID(4), Slug: "b-city", Name: "B City", Level: "city", SpaceTypeSlug: "city", Depth: 2},
		{ID: mkID(1), Slug: "z-state", Name: "Z State", Level: "state", SpaceTypeSlug: "state", Depth: 1},
		{ID: mkID(2), Slug: "a-state", Name: "A State", Level: "state", SpaceTypeSlug: "state", Depth: 1, GeoCode: strPtr("LA"), PageCount: 3},
		{ID: mkID(3), Slug: "a-city", Name: "A City", Level: "city", SpaceTypeSlug: "city", Depth: 2},
	}

	rows := directoryChildRows(children, 3)
	if len(rows) != 3 {
		t.Fatalf("cap: got %d want 3", len(rows))
	}
	if rows[0].Slug != "a-state" || rows[1].Slug != "z-state" || rows[2].Slug != "a-city" {
		t.Fatalf("order: got %q %q %q want a-state, z-state, a-city",
			rows[0].Slug, rows[1].Slug, rows[2].Slug)
	}
	if rows[0].GeoCode == nil || *rows[0].GeoCode != "LA" || rows[0].PageCount != 3 {
		t.Fatalf("geo/page_count not carried: %+v", rows[0])
	}

	// limit <= 0 and over-length limits are no-ops on the slice bound.
	if got := directoryChildRows(children, 0); len(got) != len(children) {
		t.Fatalf("zero limit: got %d want %d", len(got), len(children))
	}
	if got := directoryChildRows(children, 99); len(got) != len(children) {
		t.Fatalf("over limit: got %d want %d", len(got), len(children))
	}
}

// TestComputeDirectory_ListsValuelessChildren — the directory lists every
// linked wiki-type descendant even with no field values or wiki_pages rows
// (the rollup would emit nothing here). Unlinked, wrong-type, and
// non-wiki-type children are excluded; page_count and geo_code ride along.
func TestComputeDirectory_ListsValuelessChildren(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	uniq := strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:8]

	mk := func(q string, args ...any) pgtype.UUID { return mustIDF(t, ctx, database, q, args...) }

	// A wiki-enabled child type, plus a non-wiki type that must never
	// appear even when linked.
	childTypeID := mk(`INSERT INTO space_types (id,name,slug) VALUES (gen_random_uuid(),$1,$1) RETURNING id`, "wop-child-"+uniq)
	mk(`INSERT INTO wiki_type_configs (space_type_id,enabled,label) VALUES ($1,true,$2) RETURNING space_type_id`,
		childTypeID, "Wop Child "+uniq)
	nonWikiTypeID := mk(`INSERT INTO space_types (id,name,slug) VALUES (gen_random_uuid(),$1,$1) RETURNING id`, "wop-plain-"+uniq)
	t.Cleanup(func() {
		cctx := context.Background()
		database.Writer.Exec(cctx, `DELETE FROM wiki_type_configs WHERE space_type_id = $1`, childTypeID)
		database.Writer.Exec(cctx, `DELETE FROM space_types WHERE id IN ($1,$2)`, childTypeID, nonWikiTypeID)
	})

	parentSpace, _ := seedSpaceOfType(t, ctx, database, hf.typeID, "dirp")
	childA, childASlug := seedSpaceOfType(t, ctx, database, childTypeID, "dira")
	childB, _ := seedSpaceOfType(t, ctx, database, childTypeID, "dirb")
	nonWiki, _ := seedSpaceOfType(t, ctx, database, nonWikiTypeID, "dirnw")
	stray, _ := seedSpaceOfType(t, ctx, database, hf.typeID, "dirstray") // same type, unlinked

	link := func(kid pgtype.UUID) {
		t.Helper()
		if _, err := database.Writer.Exec(ctx,
			`UPDATE spaces SET parent_id = $1 WHERE id = $2`, parentSpace, kid); err != nil {
			t.Fatalf("link %v: %v", kid, err)
		}
	}
	link(childA)
	link(nonWiki)
	// Junction-only link for childB — exercises the space_parent_spaces arm.
	mk(`INSERT INTO space_parent_spaces (parent_space_id,space_id) VALUES ($1,$2) RETURNING space_id`,
		parentSpace, childB)
	_ = stray

	// childA gets a geo_code and two pages (one active, one inactive) —
	// the directory card must count only active pages.
	if _, err := database.Writer.Exec(ctx, `UPDATE spaces SET geo_code = 'LA' WHERE id = $1`, childA); err != nil {
		t.Fatalf("geo_code: %v", err)
	}
	pg := func(slug string, active bool) {
		mk(`INSERT INTO wiki_pages (id,space_id,slug,title,category,is_active)
			VALUES (gen_random_uuid(),$1,$2,$2,'stub',$3) RETURNING id`, childA, slug, active)
	}
	pg("p1", true)
	pg("p2", false)
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_pages WHERE space_id = $1`, childA)
	})

	svc := NewService(store, nil)
	cfg := &WikiRollupConfig{Enabled: true, Mode: "directory", Levels: []string{"wop-child-" + uniq}}
	dir := svc.computeDirectory(ctx, parentSpace, hf.typeID, cfg)
	if dir == nil {
		t.Fatal("directory is nil")
	}
	if dir.Total != 2 || len(dir.Children) != 2 {
		t.Fatalf("children: got total=%d len=%d want 2/2: %+v", dir.Total, len(dir.Children), dir.Children)
	}
	// name order: dira < dirb.
	got := map[string]WikiDirectoryChildDTO{}
	for _, c := range dir.Children {
		got[c.Slug] = c
	}
	a, ok := got[childASlug]
	if !ok {
		t.Fatalf("childA missing: %+v", dir.Children)
	}
	if a.PageCount != 1 || a.GeoCode == nil || *a.GeoCode != "LA" {
		t.Fatalf("childA card: %+v", a)
	}
	if a.TypeSlug != "wop-child-"+uniq {
		t.Fatalf("childA type slug: %q", a.TypeSlug)
	}

	// A non-directory config still produces nil via this path's contract
	// (callers gate on mode, but the helper itself is defensive).
	if d := svc.computeDirectory(ctx, parentSpace, hf.typeID, &WikiRollupConfig{Enabled: true}); d != nil {
		t.Fatalf("aggregate config produced a directory: %+v", d)
	}
	// Unknown level slugs resolve to no types → nil.
	if d := svc.computeDirectory(ctx, parentSpace, hf.typeID,
		&WikiRollupConfig{Enabled: true, Mode: "directory", Levels: []string{"no-such-type"}}); d != nil {
		t.Fatalf("unresolvable levels produced a directory: %+v", d)
	}
}

// TestGetWikiPage_LazyCategoryStubPage — a category's stub slug resolves
// without a wiki_pages row (§80: category pages are lazy like template
// pages); a directory-mode category carries the descendant list. A
// template slug colliding with a category stub keeps template precedence.
func TestGetWikiPage_LazyCategoryStubPage(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	uniq := strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:8]

	mk := func(q string, args ...any) pgtype.UUID { return mustIDF(t, ctx, database, q, args...) }
	childTypeID := mk(`INSERT INTO space_types (id,name,slug) VALUES (gen_random_uuid(),$1,$1) RETURNING id`, "wop-child-"+uniq)
	mk(`INSERT INTO wiki_type_configs (space_type_id,enabled,label) VALUES ($1,true,$2) RETURNING space_type_id`,
		childTypeID, "Wop Child "+uniq)
	t.Cleanup(func() {
		cctx := context.Background()
		database.Writer.Exec(cctx, `DELETE FROM wiki_type_configs WHERE space_type_id = $1`, childTypeID)
		database.Writer.Exec(cctx, `DELETE FROM space_types WHERE id = $1`, childTypeID)
	})

	parentSpace, spaceSlug := seedSpaceOfType(t, ctx, database, hf.typeID, "dirlazy")
	childSpace, childSlug := seedSpaceOfType(t, ctx, database, childTypeID, "dirchild")
	if _, err := database.Writer.Exec(ctx,
		`UPDATE spaces SET parent_id = $1 WHERE id = $2`, parentSpace, childSpace); err != nil {
		t.Fatalf("link child: %v", err)
	}
	_ = childSlug

	// catA ('cost-of-living') becomes a directory over the child type.
	if _, err := database.Writer.Exec(ctx,
		`UPDATE wiki_type_categories SET rollup = $1 WHERE id = $2`,
		[]byte(`{"enabled":true,"mode":"directory","levels":["wop-child-`+uniq+`"]}`), hf.catAID); err != nil {
		t.Fatalf("set directory rollup: %v", err)
	}

	svc := NewService(store, nil)
	svc.SetPermissionChecker(allowAllPerms{})
	svc.SetCustomFields(customfields.NewService(database))

	page, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "cost-of-living")
	if err != nil {
		t.Fatalf("lazy category page: %v", err)
	}
	if !page.Lazy || page.Directory == nil {
		t.Fatalf("want lazy directory page, got lazy=%v dir=%v", page.Lazy, page.Directory)
	}
	if page.Category != "cost_of_living" || page.CategoryLabel != "Cost of Living" {
		t.Fatalf("category identity: %+v", page.WikiPageDTO)
	}
	if len(page.Directory.Children) != 1 || page.Directory.Total != 1 {
		t.Fatalf("directory children: %+v", page.Directory)
	}
	var slug string
	if err := database.Reader.QueryRow(ctx, `SELECT slug FROM spaces WHERE id = $1`, childSpace).Scan(&slug); err != nil {
		t.Fatalf("child slug: %v", err)
	}
	if page.Directory.Children[0].Slug != slug {
		t.Fatalf("wrong child: %+v", page.Directory.Children[0])
	}

	// A plain (non-directory) category stub still resolves lazily — with
	// no directory payload — and unknown slugs still 404.
	plain, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "other")
	if err != nil || !plain.Lazy || plain.Directory != nil {
		t.Fatalf("plain lazy category: page=%+v err=%v", plain, err)
	}
	if _, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "no-such-slug"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown slug: want pgx.ErrNoRows, got %v", err)
	}

	// Template precedence: a page template named after a category stub
	// slug resolves as the template, not the category.
	tmpl, err := store.CreatePageTemplate(ctx, hf.catBID, "other", "Template {name}", hf.groupIDs[1], 0, false)
	if err != nil {
		t.Fatalf("create colliding template: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_type_page_templates WHERE id = $1`, tmpl.ID)
	})
	collide, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "other")
	if err != nil {
		t.Fatalf("colliding slug: %v", err)
	}
	if !strings.HasPrefix(collide.Title, "Template ") {
		t.Fatalf("template precedence broken — category stub won: title=%q", collide.Title)
	}

	// Token-form URL (category token vs canonical page slug): resolves the
	// canonical page — first the lazy stub, then the materialized row, so
	// submitted prose is never shadowed by an empty token-URL stub.
	stub, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "cost_of_living")
	if err != nil || !stub.Lazy || stub.Slug != "cost-of-living" {
		t.Fatalf("token URL stub: slug=%q lazy=%v err=%v", stub.Slug, stub.Lazy, err)
	}

	if _, err := store.CreatePage(ctx, parentSpace, pgtype.UUID{}, "cost-of-living", "Cost of Living", "cost_of_living"); err != nil {
		t.Fatalf("materialize category page: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Writer.Exec(context.WithoutCancel(t.Context()),
			`DELETE FROM wiki_pages WHERE space_id = $1`, parentSpace)
	})

	// The materialized category page carries the directory (its own page,
	// not a sibling's).
	mat, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "cost-of-living")
	if err != nil || mat.Lazy || mat.Directory == nil {
		t.Fatalf("materialized directory page: lazy=%v dir=%v err=%v", mat.Lazy, mat.Directory, err)
	}
	matViaToken, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "cost_of_living")
	if err != nil || matViaToken.Lazy || matViaToken.Slug != "cost-of-living" || matViaToken.Directory == nil {
		t.Fatalf("token URL after materialization: slug=%q lazy=%v dir=%v err=%v",
			matViaToken.Slug, matViaToken.Lazy, matViaToken.Directory, err)
	}

	if _, err := store.CreatePage(ctx, parentSpace, pgtype.UUID{}, "col-sibling", "Sibling", "cost_of_living"); err != nil {
		t.Fatalf("create sibling: %v", err)
	}
	sib, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "col-sibling")
	if err != nil || sib.Directory != nil {
		t.Fatalf("sibling must not carry directory: dir=%v err=%v", sib.Directory, err)
	}
}

// TestSubmitRevision_MaterializesLazyCategoryPage — first prose submission
// on a lazy category stub persists a wiki_pages row bound to the
// category's token/group with NULL page_template_id.
func TestSubmitRevision_MaterializesLazyCategoryPage(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	fx := seedOpsFixture(t, ctx, database)

	spaceID, spaceSlug := seedSpaceOfType(t, ctx, database, hf.typeID, "lazycat")
	t.Cleanup(func() {
		cctx := context.Background()
		database.Writer.Exec(cctx, `DELETE FROM wiki_revisions WHERE wiki_page_id IN (SELECT id FROM wiki_pages WHERE space_id = $1 AND slug = 'other')`, spaceID)
		database.Writer.Exec(cctx, `DELETE FROM wiki_pages WHERE space_id = $1 AND slug = 'other'`, spaceID)
	})

	svc := NewService(store, nil)
	revID, err := svc.SubmitRevision(ctx, fx.userID, spaceSlug, "other", SubmitRevisionInput{
		Content: []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Notes."}]}]}`),
	})
	if err != nil {
		t.Fatalf("submit on lazy category: %v", err)
	}
	if revID == "" {
		t.Fatal("empty revision id")
	}

	var tmplID, groupID pgtype.UUID
	var category string
	if err := database.Reader.QueryRow(ctx,
		`SELECT page_template_id, field_group_id, category FROM wiki_pages WHERE space_id = $1 AND slug = 'other'`, spaceID).
		Scan(&tmplID, &groupID, &category); err != nil {
		t.Fatalf("materialized row: %v", err)
	}
	if tmplID.Valid {
		t.Fatalf("category-stub page must not bind a template, got %v", tmplID)
	}
	if groupID != hf.groupIDs[1] || category != "other" {
		t.Fatalf("binding: group=%v want %v, category=%q want other", groupID, hf.groupIDs[1], category)
	}
}

// TestDeleteWikiPageTemplate_ValuesGuard — a bound page whose space holds
// a stored value in the template's group (admin SetFieldValue writes carry
// no revision) must still block deletion (§80: "prose/values").
func TestDeleteWikiPageTemplate_ValuesGuard(t *testing.T) {
	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	fx := seedOpsFixture(t, ctx, database)

	fieldID := mustIDF(t, ctx, database,
		`INSERT INTO custom_fields (group_id,key,label,field_type,position)
		 VALUES ($1,$2,'Price','price_range',0) RETURNING id`,
		hf.groupIDs[0], "wopval_"+strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:8])

	tmpl, err := store.CreatePageTemplate(ctx, hf.catAID, "vals-t", "Vals", hf.groupIDs[0], 0, false)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_type_page_templates WHERE id = $1`, tmpl.ID)
	})

	pageID := mustIDF(t, ctx, database,
		`INSERT INTO wiki_pages (id,space_id,slug,title,category,created_by,page_template_id)
		 VALUES (gen_random_uuid(),$1,$2,'Vals Bound','cost_of_living',$3,$4) RETURNING id`,
		fx.spaceID, "wop-vals-"+strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:8], fx.userID, tmpl.ID)
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_pages WHERE id = $1`, pageID)
	})

	// No revision — a stored value in the template's bound group is enough.
	if _, err := database.Writer.Exec(ctx,
		`INSERT INTO custom_field_values (field_id,entity_type,entity_id,value_json)
		 VALUES ($1,'space',$2,'{"avg":3.50}')`, fieldID, fx.spaceID); err != nil {
		t.Fatalf("seed value: %v", err)
	}
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM custom_field_values WHERE field_id = $1`, fieldID)
	})

	svc := NewService(store, nil)
	if err := svc.DeleteWikiPageTemplate(ctx, tmpl.ID.String()); !errors.Is(err, ErrWikiPageTemplateInUse) {
		t.Fatalf("values-bound template: want ErrWikiPageTemplateInUse, got %v", err)
	}

	// A bound page with no content at all deletes fine.
	empty, err := store.CreatePageTemplate(ctx, hf.catBID, "empty-t", "Empty", hf.groupIDs[1], 0, false)
	if err != nil {
		t.Fatalf("create empty template: %v", err)
	}
	pageID2 := mustIDF(t, ctx, database,
		`INSERT INTO wiki_pages (id,space_id,slug,title,category,created_by,page_template_id)
		 VALUES (gen_random_uuid(),$1,$2,'Empty Bound','other',$3,$4) RETURNING id`,
		fx.spaceID, "wop-empty-"+strings.ReplaceAll(genUUID(t, ctx, database), "-", "")[:8], fx.userID, empty.ID)
	t.Cleanup(func() {
		database.Writer.Exec(context.Background(), `DELETE FROM wiki_pages WHERE id = $1`, pageID2)
	})
	if err := svc.DeleteWikiPageTemplate(ctx, empty.ID.String()); err != nil {
		t.Fatalf("contentless bound template should delete: %v", err)
	}
}

// ---------------------------------------------------------------------------
// §80 review round 2 fixes: junction-only parent links, lazy-render
// resurrection guard, and create-path slug/category canonicalization.
// ---------------------------------------------------------------------------

// TestListSpaceParentLinks_JunctionOnlyParent — a child whose
// spaces.parent_id is NULL but which carries a space_parent_spaces junction
// row must scan is_primary=false, not SQL NULL (was a pgx scan error that
// silently killed ancestors/ancestors_alt on ~21k spaces).
func TestListSpaceParentLinks_JunctionOnlyParent(t *testing.T) {
	t.Parallel()

	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)

	parentID, _ := seedSpaceOfType(t, ctx, database, hf.typeID, "jpar")
	childID, _ := seedSpaceOfType(t, ctx, database, hf.typeID, "jchild")

	// Junction edge only — the child's spaces.parent_id stays NULL.
	if _, err := database.Writer.Exec(ctx,
		`INSERT INTO space_parent_spaces (space_id, parent_space_id) VALUES ($1,$2)`,
		childID, parentID); err != nil {
		t.Fatalf("junction insert: %v", err)
	}

	t.Cleanup(func() {
		_, _ = database.Writer.Exec(context.WithoutCancel(t.Context()),
			`DELETE FROM space_parent_spaces WHERE space_id = $1`, childID)
	})

	links, err := store.ListSpaceParentLinks(ctx, childID)
	if err != nil {
		t.Fatalf("ListSpaceParentLinks must not scan-error on junction-only parents: %v", err)
	}

	if len(links) != 1 {
		t.Fatalf("links: got %d want 1: %+v", len(links), links)
	}

	if links[0].IsPrimary {
		t.Fatal("junction-only parent must report is_primary=false")
	}

	if links[0].ID != parentID {
		t.Fatalf("wrong parent: got %v want %v", links[0].ID, parentID)
	}
}

// TestListWikiDestinations_WithinJunctionDescendants — ?within= must reach
// junction-only descendants at every hop (space_parent_spaces edges past
// hop 1), matching the directory listing's edge set; the within target
// itself is never listed.
func TestListWikiDestinations_WithinJunctionDescendants(t *testing.T) {
	t.Parallel()

	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)

	parentID, parentSlug := seedSpaceOfType(t, ctx, database, hf.typeID, "wpar")
	childID, _ := seedSpaceOfType(t, ctx, database, hf.typeID, "wchild")
	grandID, _ := seedSpaceOfType(t, ctx, database, hf.typeID, "wgchild")

	// Junction-only edges — spaces.parent_id stays NULL on both links.
	if _, err := database.Writer.Exec(ctx,
		`INSERT INTO space_parent_spaces (space_id, parent_space_id)
		 VALUES ($1,$2),($3,$4)`, childID, parentID, grandID, childID); err != nil {
		t.Fatalf("junction insert: %v", err)
	}
	t.Cleanup(func() {
		_, _ = database.Writer.Exec(context.WithoutCancel(t.Context()),
			`DELETE FROM space_parent_spaces WHERE space_id IN ($1,$2)`, childID, grandID)
	})

	dests, err := store.ListWikiDestinations(ctx, "", "", "", "", parentSlug, false, "name", 0, 0)
	if err != nil {
		t.Fatalf("within destinations: %v", err)
	}

	got := make(map[pgtype.UUID]bool, len(dests))
	for _, d := range dests {
		got[d.SpaceID] = true
	}
	if !got[childID] || !got[grandID] {
		t.Fatalf("junction descendants missing: child=%v grandchild=%v", got[childID], got[grandID])
	}
	if got[parentID] {
		t.Fatal("within target must not appear in its own descendant list")
	}
}

// TestGetWikiPage_LazyRespectsTombstones — a template-matching slug whose
// wiki_pages row exists but is inactive or soft-deleted must 404, not
// resurrect as a lazy render.
func TestGetWikiPage_LazyRespectsTombstones(t *testing.T) {
	t.Parallel()

	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	fx := seedOpsFixture(t, ctx, database)

	spaceID, spaceSlug := seedSpaceOfType(t, ctx, database, hf.typeID, "tomb")

	tmpl, err := store.CreatePageTemplate(ctx, hf.catAID, "prices", "Prices in {name}", hf.groupIDs[0], 0, false)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	t.Cleanup(func() {
		cctx := context.WithoutCancel(t.Context())
		_, _ = database.Writer.Exec(cctx, `DELETE FROM wiki_type_page_templates WHERE id = $1`, tmpl.ID)
		_, _ = database.Writer.Exec(cctx, `DELETE FROM wiki_pages WHERE space_id = $1 AND slug = 'prices'`, spaceID)
	})

	svc := NewService(store, nil)
	svc.SetPermissionChecker(allowAllPerms{})
	svc.SetCustomFields(customfields.NewService(database))

	mk := func(isActive, deleted bool) {
		_, err := database.Writer.Exec(ctx,
			`INSERT INTO wiki_pages (id,space_id,slug,title,category,created_by,is_active,deleted_at)
			 VALUES (gen_random_uuid(),$1,'prices','Prices','cost_of_living',$2,$3,
			         CASE WHEN $4 THEN now() ELSE NULL END)`,
			spaceID, fx.userID, isActive, deleted)
		if err != nil {
			t.Fatalf("insert tombstone: %v", err)
		}
	}

	// Deactivated row → 404, no lazy render.
	mk(false, false)

	if _, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "prices"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("deactivated page: want pgx.ErrNoRows, got %v", err)
	}

	// Soft-deleted row → 404 too.
	if _, err := database.Writer.Exec(ctx,
		`DELETE FROM wiki_pages WHERE space_id = $1 AND slug = 'prices'`, spaceID); err != nil {
		t.Fatalf("clear deactivated row: %v", err)
	}

	mk(false, true)

	if _, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "prices"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("soft-deleted page: want pgx.ErrNoRows, got %v", err)
	}

	// And with the row gone entirely the lazy path still works.
	if _, err := database.Writer.Exec(ctx,
		`DELETE FROM wiki_pages WHERE space_id = $1 AND slug = 'prices'`, spaceID); err != nil {
		t.Fatalf("clear tombstone: %v", err)
	}

	page, err := svc.GetWikiPage(ctx, pgtype.UUID{}, spaceSlug, "prices")
	if err != nil || !page.Lazy {
		t.Fatalf("absent row should lazy-render: page=%+v err=%v", page, err)
	}
}

// TestCreatePageWithRevision_Canonicalizes — community creates must store
// the canonical category token and a validated slug so sibling bucketing
// and template binding don't fragment on input spelling.
func TestCreatePageWithRevision_Canonicalizes(t *testing.T) {
	t.Parallel()

	store, database := opsTestStore(t)
	ctx := t.Context()
	hf := seedHierarchyFixture(t, ctx, database)
	fx := seedOpsFixture(t, ctx, database)

	spaceID, spaceSlug := seedSpaceOfType(t, ctx, database, hf.typeID, "canon")

	tmpl, err := store.CreatePageTemplate(ctx, hf.catAID, "prices", "Prices in {name}", hf.groupIDs[0], 0, false)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	t.Cleanup(func() {
		cctx := context.WithoutCancel(t.Context())
		_, _ = database.Writer.Exec(cctx, `DELETE FROM wiki_type_page_templates WHERE id = $1`, tmpl.ID)
		_, _ = database.Writer.Exec(cctx, `DELETE FROM wiki_revisions WHERE submitted_by = $1`, fx.userID)
		_, _ = database.Writer.Exec(cctx, `DELETE FROM wiki_pages WHERE space_id = $1`, spaceID)
	})

	svc := NewService(store, nil)
	content := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}`)

	// Dash-form + uppercase category → canonical token; capitalized slug →
	// lowercased; the template binds.
	pageID, _, slug, err := svc.CreatePageWithRevision(ctx, fx.userID, CreateWikiPageInput{
		SpaceSlug: spaceSlug,
		Slug:      "Prices",
		Title:     "Prices",
		Category:  "Cost-of-Living",
		Content:   content,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if slug != "prices" {
		t.Fatalf("slug not normalized: %q", slug)
	}

	var (
		cat    string
		tmplID pgtype.UUID
	)

	if err := database.Reader.QueryRow(ctx,
		`SELECT category, page_template_id FROM wiki_pages WHERE id = $1`, pageID).Scan(&cat, &tmplID); err != nil {
		t.Fatalf("read stored page: %v", err)
	}

	if cat != "cost_of_living" {
		t.Fatalf("category not canonicalized: %q", cat)
	}

	if tmplID != tmpl.ID {
		t.Fatalf("template not bound: got %v want %v", tmplID, tmpl.ID)
	}

	// A dash-form category on a *different* slug lands in the same sibling
	// bucket as the canonical-token page.
	_, _, _, err = svc.CreatePageWithRevision(ctx, fx.userID, CreateWikiPageInput{
		SpaceSlug: spaceSlug,
		Slug:      "markets",
		Title:     "Markets",
		Category:  "cost-of-living",
		Content:   content,
	})
	if err != nil {
		t.Fatalf("create sibling: %v", err)
	}

	sibs, err := store.ListPageSiblings(ctx, spaceID, "cost_of_living")
	if err != nil {
		t.Fatalf("siblings: %v", err)
	}

	if len(sibs) != 2 {
		t.Fatalf("sibling bucket fragmented: got %d want 2: %+v", len(sibs), sibs)
	}

	// Slugs outside the canonical pattern are rejected.
	if _, _, _, err := svc.CreatePageWithRevision(ctx, fx.userID, CreateWikiPageInput{
		SpaceSlug: spaceSlug,
		Slug:      "Bad Slug!",
		Title:     "Bad",
		Category:  "cost_of_living",
		Content:   content,
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid slug: want ErrInvalidInput, got %v", err)
	}
}
