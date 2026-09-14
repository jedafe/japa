package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// TestRelocationGuideBulkFlow exercises the end-to-end agent workflow for
// generating multiple guide content rows (one per West African country) and
// publishing them in bulk.
//
// Flow simulated:
//  1. content_types.list to discover the guide type.
//  2. guide_categories.list to discover the relocation category.
//  3. content.bulk_create to create 5 draft relocation guides.
//  4. custom_field_values.bulk_set to populate a structured value per guide.
//  5. content.bulk_publish to publish all 5 guides.
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content Creation Flow)
// Ref: japaconnect-engineering/sections/14-content-type-system.md (Guide Content Type).
func TestRelocationGuideBulkFlow(t *testing.T) {
	t.Parallel()

	userID := mustUUID(t, "00000000-0000-0000-0000-000000000099")

	guideGroupID := mustUUID(t, "00000000-0000-0000-0000-000000000010")
	fieldID := mustUUID(t, "00000000-0000-0000-0000-000000000030")

	// Step 1: content_types.list returns guide as an allowed type.
	contentTypesSvc := &stubContentTypesService{
		types: []ContentType{
			{
				ContentType:           "guide",
				Label:                 "Guide",
				HasCategory:           true,
				CategoryListTool:      "guide_categories.list",
				FieldGroupEntityClass: "content_type:guide:relocation",
				FieldGroupID:          guideGroupID.String(),
				FieldGroupSlug:        "relocation_guide_fields",
			},
		},
	}

	// Step 2: guide_categories.list returns the relocation category.
	guideCatSvc := &stubGuideCategoryService{
		cats: []GuideCategory{
			{
				Slug:      "relocation",
				Label:     "Relocation",
				IsSystem:  true,
				GroupID:   guideGroupID.String(),
				GroupSlug: "relocation_guide_fields",
			},
		},
	}

	// Step 3: content.bulk_create creates the rows.
	createdRows := []ContentCreateRow{}
	createdOrder := 0
	contentCreator := func(ctx context.Context, tid pgtype.UUID, input ContentCreateInput) (ContentCreateRow, error) {
		createdOrder++
		id := mustUUID(t, fmt.Sprintf("00000000-0000-0000-0000-%012d", createdOrder))
		row := ContentCreateRow{
			ID:            id,
			Title:         input.Title,
			Slug:          fmt.Sprintf("relocation-from-%d", createdOrder),
			Status:        input.Status,
			AuthorID:      input.AuthorID,
			ContentType:   input.ContentType,
			GuideCategory: input.GuideCategory,
		}
		createdRows = append(createdRows, row)

		return row, nil
	}

	// Step 4: custom_field_values.bulk_set records the values.
	customFieldsSvc := &stubCustomFieldsService{}

	// Step 5: content.bulk_publish transitions status.
	publishedIDs := []pgtype.UUID{}
	contentLifecycle := &stubContentLifecycleService{
		updateStatusFunc: func(ctx context.Context, id pgtype.UUID, status string) (gen.UpdateContentStatusRow, error) {
			publishedIDs = append(publishedIDs, id)
			return gen.UpdateContentStatusRow{
				ID:     id,
				Title:  "Guide",
				Slug:   "relocation-" + id.String(),
				Status: status,
			}, nil
		},
	}

	permSvc := &stubPerms{granted: map[string]bool{
		"content.create":           true,
		"content.publish":          true,
		"agent.content_type.guide": true,
		"content.assign_author":    false,
	}}

	deps := &ToolDeps{
		ContentTypes:     contentTypesSvc,
		GuideCategories:  guideCatSvc,
		ContentCreator:   contentCreator,
		CustomFields:     customFieldsSvc,
		ContentLifecycle: contentLifecycle,
		Permission:       permSvc,
	}

	// 1. content_types.list
	typesHandler := ContentTypesListHandler(deps)

	typesResult, err := typesHandler(t.Context(), json.RawMessage(`{}`), userID)
	if err != nil {
		t.Fatalf("content_types.list failed: %v", err)
	}

	if typesResult.Status != ToolCallStatusSuccess {
		t.Fatalf("content_types.list returned status %q", typesResult.Status)
	}

	typesMap := parseToolResult(t, typesResult)
	if typesMap["total"].(float64) != 1 {
		t.Fatalf("expected 1 content type, got %v", typesMap["total"])
	}

	// 2. guide_categories.list
	catsHandler := GuideCategoriesListHandler(deps)

	catsResult, err := catsHandler(t.Context(), json.RawMessage(`{}`), userID)
	if err != nil {
		t.Fatalf("guide_categories.list failed: %v", err)
	}

	if catsResult.Status != ToolCallStatusSuccess {
		t.Fatalf("guide_categories.list returned status %q", catsResult.Status)
	}

	catsMap := parseToolResult(t, catsResult)
	if catsMap["total"].(float64) != 1 {
		t.Fatalf("expected 1 guide category, got %v", catsMap["total"])
	}

	// 3. content.bulk_create for 5 West African countries.
	countries := []string{"Nigeria", "Ghana", "Senegal", "Cote d'Ivoire", "Cameroon"}
	items := make([]map[string]any, 0, len(countries))

	for _, country := range countries {
		items = append(items, map[string]any{
			"title":          "How to Relocate to Canada from " + country,
			"content_type":   "guide",
			"body":           fmt.Sprintf("A practical guide for %s citizens relocating to Canada.", country),
			"guide_category": "relocation",
			"field_values": map[string]any{
				"_ai_provenance": map[string]any{"ai_generated": true, "source": "relocation_bulk_flow_test"},
			},
		})
	}

	bulkCreateArgs, _ := json.Marshal(map[string]any{"items": items})

	bulkCreateHandler := ContentBulkCreateHandler(deps)

	createResult, err := bulkCreateHandler(t.Context(), json.RawMessage(bulkCreateArgs), userID)
	if err != nil {
		t.Fatalf("content.bulk_create failed: %v", err)
	}

	if createResult.Status != ToolCallStatusSuccess {
		t.Fatalf("content.bulk_create returned status %q", createResult.Status)
	}

	createMap := parseToolResult(t, createResult)
	if createMap["succeeded"].(float64) != float64(len(countries)) {
		t.Fatalf("expected %d created, got %v", len(countries), createMap["succeeded"])
	}

	createdResultRows, ok := createMap["results"].([]any)
	if !ok || len(createdResultRows) != len(countries) {
		t.Fatalf("expected %d result rows, got %v", len(countries), createdResultRows)
	}

	contentIDs := make([]string, 0, len(countries))

	for i, r := range createdResultRows {
		row := r.(map[string]any)
		if row["content_id"] == "" {
			t.Fatalf("missing content_id in result %d: %v", i, row)
		}

		contentIDs = append(contentIDs, row["content_id"].(string))
	}

	// 4. custom_field_values.bulk_set for each created guide (one field per guide).
	bulkSetHandler := CFVBulkSetHandler(deps)

	for _, contentID := range contentIDs {
		setArgs, _ := json.Marshal(map[string]any{
			"entity_type": "content",
			"entity_id":   contentID,
			"values": []map[string]any{
				{"field_id": fieldID.String(), "value_text": "Canada"},
			},
		})

		setResult, err := bulkSetHandler(t.Context(), json.RawMessage(setArgs), userID)
		if err != nil {
			t.Fatalf("custom_field_values.bulk_set for %s failed: %v", contentID, err)
		}

		if setResult.Status != ToolCallStatusSuccess {
			t.Fatalf("custom_field_values.bulk_set for %s returned status %q", contentID, setResult.Status)
		}
	}

	// 5. content.bulk_publish
	publishArgs, _ := json.Marshal(map[string]any{
		"content_ids": contentIDs,
		"status":      "published",
	})
	bulkPublishHandler := ContentBulkPublishHandler(deps)

	publishResult, err := bulkPublishHandler(t.Context(), json.RawMessage(publishArgs), userID)
	if err != nil {
		t.Fatalf("content.bulk_publish failed: %v", err)
	}

	if publishResult.Status != ToolCallStatusSuccess {
		t.Fatalf("content.bulk_publish returned status %q", publishResult.Status)
	}

	publishMap := parseToolResult(t, publishResult)
	if publishMap["succeeded"].(float64) != float64(len(countries)) {
		t.Fatalf("expected %d published, got %v", len(countries), publishMap["succeeded"])
	}

	if len(publishedIDs) != len(countries) {
		t.Fatalf("expected %d update calls, got %d", len(countries), len(publishedIDs))
	}
}
