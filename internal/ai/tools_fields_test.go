// Package ai — tools_fields_test.go covers the Phase 4 custom field tool
// handlers (Section 103, ADR-111).
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
// Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 4)
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// custom_fields.list (task 4.05)
// ---------------------------------------------------------------------------

func TestCustomFieldsListHandler_Success(t *testing.T) {
	t.Parallel()

	groupID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	deps := &ToolDeps{
		CustomFields: &stubCustomFieldsService{
			fields: []gen.ListCustomFieldsByGroupRow{
				{ID: mustUUID(t, "00000000-0000-0000-0000-000000000010"), Label: "Scholarship", Key: "scholarship", FieldType: "text", IsRequired: true},
				{ID: mustUUID(t, "00000000-0000-0000-0000-000000000011"), Label: "Ranking", Key: "ranking", FieldType: "number"},
			},
		},
	}

	handler := CustomFieldsListHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"group_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	m := parseToolResult(t, tr)
	fields, _ := m["fields"].([]any)
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	// Suppress unused var warning for groupID (used for clarity).
	_ = groupID
}

func TestCustomFieldsListHandler_InvalidGroupID(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{CustomFields: &stubCustomFieldsService{}}
	handler := CustomFieldsListHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"group_id":"not-a-uuid"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestCustomFieldsListHandler_NoDeps(t *testing.T) {
	t.Parallel()

	handler := CustomFieldsListHandler(nil)
	tr, _ := handler(context.Background(), json.RawMessage(`{"group_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

// ---------------------------------------------------------------------------
// custom_field_values.read (task 4.06)
// ---------------------------------------------------------------------------

func TestCFVReadHandler_Success(t *testing.T) {
	t.Parallel()

	entityID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	deps := &ToolDeps{
		CustomFields: &stubCustomFieldsService{
			cfvs: []gen.CustomFieldValue{
				{FieldID: mustUUID(t, "00000000-0000-0000-0000-000000000010"), EntityType: "content", EntityID: entityID, ValueText: strPtr("val1"), AiExempt: false},
				{FieldID: mustUUID(t, "00000000-0000-0000-0000-000000000011"), EntityType: "content", EntityID: entityID, ValueText: strPtr("exempt"), AiExempt: true},
			},
		},
	}

	handler := CFVReadHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"entity_type":"content","entity_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	m := parseToolResult(t, tr)
	fvs, _ := m["field_values"].([]any)
	if len(fvs) != 1 {
		t.Fatalf("expected 1 non-exempt value, got %d", len(fvs))
	}
}

func TestCFVReadHandler_MissingEntityType(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{CustomFields: &stubCustomFieldsService{}}
	handler := CFVReadHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{"entity_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestCFVReadPrivacyTarget_Content(t *testing.T) {
	t.Parallel()

	et, id, err := CFVReadPrivacyTarget(context.Background(), json.RawMessage(`{"entity_type":"content","entity_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if et != PrivacyEntityContent {
		t.Errorf("entity type = %q, want %q", et, PrivacyEntityContent)
	}
	if !id.Valid {
		t.Error("expected valid UUID")
	}
}

func TestCFVReadPrivacyTarget_User(t *testing.T) {
	t.Parallel()

	et, _, err := CFVReadPrivacyTarget(context.Background(), json.RawMessage(`{"entity_type":"user","entity_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if et != PrivacyEntityUser {
		t.Errorf("entity type = %q, want %q", et, PrivacyEntityUser)
	}
}

func TestCFVReadPrivacyTarget_UnknownType_Skips(t *testing.T) {
	t.Parallel()

	et, id, err := CFVReadPrivacyTarget(context.Background(), json.RawMessage(`{"entity_type":"organization","entity_id":"00000000-0000-0000-0000-000000000001"}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if et != "" {
		t.Errorf("entity type = %q, want empty (skip)", et)
	}
	if id.Valid {
		t.Error("expected invalid UUID for skip")
	}
}

// ---------------------------------------------------------------------------
// custom_field_values.set (task 4.07)
// ---------------------------------------------------------------------------

func TestCFVSetHandler_Success(t *testing.T) {
	t.Parallel()

	cfs := &stubCustomFieldsService{}
	deps := &ToolDeps{CustomFields: cfs}

	handler := CFVSetHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{
		"field_id":"00000000-0000-0000-0000-000000000010",
		"entity_type":"content",
		"entity_id":"00000000-0000-0000-0000-000000000001",
		"value_text":"test value"
	}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	if len(cfs.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(cfs.upserted))
	}
	if cfs.upserted[0].ValueText == nil || *cfs.upserted[0].ValueText != "test value" {
		t.Error("value_text not passed correctly")
	}
}

func TestCFVSetHandler_NumberValue(t *testing.T) {
	t.Parallel()

	cfs := &stubCustomFieldsService{}
	deps := &ToolDeps{CustomFields: cfs}

	handler := CFVSetHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{
		"field_id":"00000000-0000-0000-0000-000000000010",
		"entity_type":"content",
		"entity_id":"00000000-0000-0000-0000-000000000001",
		"value_number":100000
	}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	if len(cfs.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(cfs.upserted))
	}
	if !cfs.upserted[0].ValueNumber.Valid {
		t.Fatal("value_number not set")
	}
	if cfs.upserted[0].ValueText != nil {
		t.Error("value_text should be nil for number value")
	}
}

func TestCFVSetHandler_UpsertError(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		CustomFields: &stubCustomFieldsService{upsertErr: errors.New("db error")},
	}
	handler := CFVSetHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{
		"field_id":"00000000-0000-0000-0000-000000000010",
		"entity_type":"content",
		"entity_id":"00000000-0000-0000-0000-000000000001",
		"value_text":"x"
	}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestCFVSetHandler_InvalidFieldID(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{CustomFields: &stubCustomFieldsService{}}
	handler := CFVSetHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{
		"field_id":"bad",
		"entity_type":"content",
		"entity_id":"00000000-0000-0000-0000-000000000001"
	}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestCFVSetPrivacyTarget(t *testing.T) {
	t.Parallel()

	et, _, err := CFVSetPrivacyTarget(context.Background(), json.RawMessage(`{
		"field_id":"00000000-0000-0000-0000-000000000010",
		"entity_type":"content",
		"entity_id":"00000000-0000-0000-0000-000000000001"
	}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if et != PrivacyEntityContent {
		t.Errorf("entity type = %q, want %q", et, PrivacyEntityContent)
	}
}

// ---------------------------------------------------------------------------
// custom_field_values.bulk_set (task 4.08)
// ---------------------------------------------------------------------------

func TestCFVBulkSetHandler_Success(t *testing.T) {
	t.Parallel()

	cfs := &stubCustomFieldsService{}
	deps := &ToolDeps{CustomFields: cfs}

	handler := CFVBulkSetHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{
		"entity_type":"content",
		"entity_id":"00000000-0000-0000-0000-000000000001",
		"values":[
			{"field_id":"00000000-0000-0000-0000-000000000010","value_text":"v1"},
			{"field_id":"00000000-0000-0000-0000-000000000011","value_text":"v2"}
		]
	}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	m := parseToolResult(t, tr)
	if m["succeeded"].(float64) != 2 {
		t.Errorf("expected 2 succeeded, got %v", m["succeeded"])
	}
	if len(cfs.upserted) != 2 {
		t.Fatalf("expected 2 upserts, got %d", len(cfs.upserted))
	}
}

func TestCFVBulkSetHandler_NumberValue(t *testing.T) {
	t.Parallel()

	cfs := &stubCustomFieldsService{}
	deps := &ToolDeps{CustomFields: cfs}

	handler := CFVBulkSetHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{
		"entity_type":"content",
		"entity_id":"00000000-0000-0000-0000-000000000001",
		"values":[
			{"field_id":"00000000-0000-0000-0000-000000000010","value_number":50000},
			{"field_id":"00000000-0000-0000-0000-000000000011","value_number":120000}
		]
	}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	m := parseToolResult(t, tr)
	if m["succeeded"].(float64) != 2 {
		t.Errorf("expected 2 succeeded, got %v", m["succeeded"])
	}
	if len(cfs.upserted) != 2 {
		t.Fatalf("expected 2 upserts, got %d", len(cfs.upserted))
	}
	for _, p := range cfs.upserted {
		if !p.ValueNumber.Valid {
			t.Error("value_number not set for a bulk entry")
		}
	}
}

func TestCFVBulkSetHandler_PartialFailure(t *testing.T) {
	t.Parallel()

	cfs := &stubCustomFieldsService{
		upsertErr: errors.New("db error"),
	}
	deps := &ToolDeps{CustomFields: cfs}

	handler := CFVBulkSetHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{
		"entity_type":"content",
		"entity_id":"00000000-0000-0000-0000-000000000001",
		"values":[
			{"field_id":"00000000-0000-0000-0000-000000000010","value_text":"v1"},
			{"field_id":"00000000-0000-0000-0000-000000000011","value_text":"v2"}
		]
	}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Bulk set reports per-field status, not a global error.
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success (per-field errors reported)", tr.Status)
	}
	m := parseToolResult(t, tr)
	if m["succeeded"].(float64) != 0 {
		t.Errorf("expected 0 succeeded, got %v", m["succeeded"])
	}
	if m["failed"].(float64) != 2 {
		t.Errorf("expected 2 failed, got %v", m["failed"])
	}
}

func TestCFVBulkSetHandler_PrivacyBlocked(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{
		CustomFields: &stubCustomFieldsService{},
		Privacy:      &stubPrivacyChecker{exempt: true},
	}

	handler := CFVBulkSetHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{
		"entity_type":"content",
		"entity_id":"00000000-0000-0000-0000-000000000001",
		"values":[{"field_id":"00000000-0000-0000-0000-000000000010","value_text":"v1"}]
	}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusPrivacyBlocked {
		t.Fatalf("status = %q, want privacy_blocked", tr.Status)
	}
}

func TestCFVBulkSetHandler_EmptyValues(t *testing.T) {
	t.Parallel()

	deps := &ToolDeps{CustomFields: &stubCustomFieldsService{}}
	handler := CFVBulkSetHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{
		"entity_type":"content",
		"entity_id":"00000000-0000-0000-0000-000000000001",
		"values":[]
	}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestCFVBulkSetHandler_InvalidFieldIDInBatch(t *testing.T) {
	t.Parallel()

	cfs := &stubCustomFieldsService{}
	deps := &ToolDeps{CustomFields: cfs}

	handler := CFVBulkSetHandler(deps)
	tr, _ := handler(context.Background(), json.RawMessage(`{
		"entity_type":"content",
		"entity_id":"00000000-0000-0000-0000-000000000001",
		"values":[
			{"field_id":"bad-uuid","value_text":"v1"},
			{"field_id":"00000000-0000-0000-0000-000000000011","value_text":"v2"}
		]
	}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success (per-field errors)", tr.Status)
	}
	m := parseToolResult(t, tr)
	if m["succeeded"].(float64) != 1 {
		t.Errorf("expected 1 succeeded, got %v", m["succeeded"])
	}
	if m["failed"].(float64) != 1 {
		t.Errorf("expected 1 failed, got %v", m["failed"])
	}
}

func TestCFVBulkSetPrivacyTarget(t *testing.T) {
	t.Parallel()

	et, _, err := CFVBulkSetPrivacyTarget(context.Background(), json.RawMessage(`{
		"entity_type":"content",
		"entity_id":"00000000-0000-0000-0000-000000000001",
		"values":[]
	}`), pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if et != PrivacyEntityContent {
		t.Errorf("entity type = %q, want %q", et, PrivacyEntityContent)
	}
}

// ---------------------------------------------------------------------------
// privacyEntityTypeFor
// ---------------------------------------------------------------------------

// stubCustomFieldDefsService implements CustomFieldDefsService for testing.
type stubCustomFieldDefsService struct {
	groups         []gen.CustomFieldGroup
	groupsByClass  map[string][]gen.CustomFieldGroup
	groupsErr      error
	createFieldErr error
	updateFieldErr error
	createGroupErr error
}

func (s *stubCustomFieldDefsService) ListFieldGroups(ctx context.Context, tenantID pgtype.UUID) ([]gen.CustomFieldGroup, error) {
	return s.groups, s.groupsErr
}

func (s *stubCustomFieldDefsService) ListFieldGroupsByEntityClass(ctx context.Context, tenantID pgtype.UUID, entityClass string) ([]gen.CustomFieldGroup, error) {
	if s.groupsByClass != nil {
		return s.groupsByClass[entityClass], s.groupsErr
	}
	return s.groups, s.groupsErr
}

func (s *stubCustomFieldDefsService) CreateField(ctx context.Context, params gen.CreateCustomFieldParams) (gen.CreateCustomFieldRow, error) {
	return gen.CreateCustomFieldRow{}, s.createFieldErr
}

func (s *stubCustomFieldDefsService) UpdateField(ctx context.Context, params gen.UpdateCustomFieldParams) (gen.UpdateCustomFieldRow, error) {
	return gen.UpdateCustomFieldRow{}, s.updateFieldErr
}

func (s *stubCustomFieldDefsService) CreateFieldGroup(ctx context.Context, tenantID pgtype.UUID, name, slug string, description *string, isSystem bool) (gen.CustomFieldGroup, error) {
	return gen.CustomFieldGroup{}, s.createGroupErr
}

// ---------------------------------------------------------------------------
// custom_field_groups.list
// ---------------------------------------------------------------------------

func TestCustomFieldGroupsListHandler_Success(t *testing.T) {
	t.Parallel()

	groupID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	deps := &ToolDeps{
		CustomFieldDefs: &stubCustomFieldDefsService{
			groups: []gen.CustomFieldGroup{
				{ID: groupID, Name: "Blog Fields", Slug: "blog_post_fields", IsSystem: true},
			},
		},
	}

	handler := CustomFieldGroupsListHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	m := parseToolResult(t, tr)
	groups, _ := m["groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
}

func TestCustomFieldGroupsListHandler_EntityClassFilter(t *testing.T) {
	t.Parallel()

	groupID := mustUUID(t, "00000000-0000-0000-0000-000000000001")
	otherID := mustUUID(t, "00000000-0000-0000-0000-000000000002")
	deps := &ToolDeps{
		CustomFieldDefs: &stubCustomFieldDefsService{
			groupsByClass: map[string][]gen.CustomFieldGroup{
				"content_type:blog_post": {
					{ID: groupID, Name: "Blog Fields", Slug: "blog_post_fields", IsSystem: true},
				},
			},
			groups: []gen.CustomFieldGroup{
				{ID: otherID, Name: "Other", Slug: "other_fields", IsSystem: false},
			},
		},
	}

	handler := CustomFieldGroupsListHandler(deps)
	tr, err := handler(context.Background(), json.RawMessage(`{"entity_class":"content_type:blog_post"}`), pgtype.UUID{}, pgtype.UUID{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != ToolCallStatusSuccess {
		t.Fatalf("status = %q, want success", tr.Status)
	}
	m := parseToolResult(t, tr)
	groups, _ := m["groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("expected 1 filtered group, got %d", len(groups))
	}
}

func TestCustomFieldGroupsListHandler_NoDeps(t *testing.T) {
	t.Parallel()

	handler := CustomFieldGroupsListHandler(nil)
	tr, _ := handler(context.Background(), json.RawMessage(`{}`), pgtype.UUID{}, pgtype.UUID{})
	if tr.Status != ToolCallStatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
}

func TestPrivacyEntityTypeFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want PrivacyEntityType
		ok   bool
	}{
		{"content", PrivacyEntityContent, true},
		{"user", PrivacyEntityUser, true},
		{"organization", "", false},
		{"space", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		got, ok := privacyEntityTypeFor(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("privacyEntityTypeFor(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
