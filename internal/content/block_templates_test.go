package content

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Block Template Registry
// ---------------------------------------------------------------------------

func TestBlockTemplateCount(t *testing.T) {
	t.Parallel()

	if got := BlockTypeCount(); got != 11 {
		t.Errorf("BlockTypeCount() = %d, want 11", got)
	}
}

func TestGetBlockTemplatesReturnsAll(t *testing.T) {
	t.Parallel()

	templates := GetBlockTemplates()
	if len(templates) != 11 {
		t.Fatalf("GetBlockTemplates() returned %d templates, want 11", len(templates))
	}
}

func TestGetBlockTemplatesReturnsCopy(t *testing.T) {
	t.Parallel()

	a := GetBlockTemplates()
	b := GetBlockTemplates()
	// Mutating the returned slice must not affect subsequent calls.
	a[0].Type = "MUTATED"

	again := GetBlockTemplates()
	if again[0].Type == "MUTATED" {
		t.Error("GetBlockTemplates did not return a defensive copy")
	}

	_ = b
}

func TestBlockTemplatesRequiredFields(t *testing.T) {
	t.Parallel()

	templates := GetBlockTemplates()
	validCategories := map[string]bool{
		BlockCategoryLayout:      true,
		BlockCategoryContent:     true,
		BlockCategoryMedia:       true,
		BlockCategoryInteractive: true,
		BlockCategoryData:        true,
	}

	for i, bt := range templates {
		if bt.Type == "" {
			t.Errorf("template[%d].Type is empty", i)
		}

		if bt.Name == "" {
			t.Errorf("template[%d].Name is empty (type=%q)", i, bt.Type)
		}

		if bt.Category == "" {
			t.Errorf("template[%d].Category is empty (type=%q)", i, bt.Type)
		}

		if !validCategories[bt.Category] {
			t.Errorf("template[%d].Category = %q, not a valid category", i, bt.Category)
		}

		if bt.Icon == "" {
			t.Errorf("template[%d].Icon is empty (type=%q)", i, bt.Type)
		}

		if bt.Description == "" {
			t.Errorf("template[%d].Description is empty (type=%q)", i, bt.Type)
		}
	}
}

func TestBlockTemplatesDefaultPropsNotNil(t *testing.T) {
	t.Parallel()

	templates := GetBlockTemplates()
	for i, bt := range templates {
		if bt.DefaultProps == nil {
			t.Errorf("template[%d] (type=%q) has nil DefaultProps", i, bt.Type)
		}
	}
}

func TestGetBlockTemplateByType(t *testing.T) {
	t.Parallel()

	expected := []string{
		"hero", "rich_text", "image", "cta", "stats",
		"faq", "testimonial", "comparison", "form", "spacer",
		"dynamic_listing",
	}
	for _, typ := range expected {
		bt, ok := GetBlockTemplateByType(typ)
		if !ok {
			t.Errorf("GetBlockTemplateByType(%q) not found", typ)
			continue
		}

		if bt.Type != typ {
			t.Errorf("GetBlockTemplateByType(%q).Type = %q", typ, bt.Type)
		}
	}
	// Unknown type must return false.
	if _, ok := GetBlockTemplateByType("nonexistent"); ok {
		t.Error("GetBlockTemplateByType(nonexistent) = true, want false")
	}
}

func TestBlockTemplatesJSONSerializable(t *testing.T) {
	t.Parallel()

	templates := GetBlockTemplates()

	data, err := json.Marshal(templates)
	if err != nil {
		t.Fatalf("json.Marshal(templates) error: %v", err)
	}

	if len(data) == 0 {
		t.Error("json.Marshal produced empty output")
	}

	var decoded []BlockTemplate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal round-trip error: %v", err)
	}

	if len(decoded) != 11 {
		t.Errorf("round-trip produced %d templates, want 11", len(decoded))
	}
}

// ---------------------------------------------------------------------------
// ListBlockTypes handler
// ---------------------------------------------------------------------------

func TestListBlockTypesHandler(t *testing.T) {
	t.Parallel()

	h := &Handlers{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/page-builder/block-types", nil)
	rr := httptest.NewRecorder()

	h.ListBlockTypes(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body struct {
		Data struct {
			BlockTypes []BlockTemplate `json:"block_types"`
		} `json:"data"`
	}

	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(body.Data.BlockTypes) != 11 {
		t.Errorf("response contained %d block types, want 11", len(body.Data.BlockTypes))
	}
}
