package customfields

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jedafe/japa/internal/httpx"
)

// requestWithURLParam builds a request whose chi route context carries the
// given URL parameter, mimicking what the chi router injects during routing.
// The body may be empty for handlers that do not read the request body.
func requestWithURLParam(method, target, param, value, body string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(param, value)

	r := httptest.NewRequest(method, target, strings.NewReader(body))

	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// requestWithURLParams is the two-param variant of requestWithURLParam, for
// nested admin routes like /admin/field-groups/{id}/fields/{fid} that carry
// two distinct IDs in the same path.
func requestWithURLParams(method, target string, params map[string]string, body string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}

	r := httptest.NewRequest(method, target, strings.NewReader(body))

	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// decodeErrorResponse unmarshals an error envelope from the response body,
// failing the test when the body is not valid JSON.
func decodeErrorResponse(t *testing.T, body []byte) errorResponse {
	t.Helper()

	var got errorResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}

	return got
}

func TestIsValidFieldType(t *testing.T) {
	t.Parallel()

	valid := []string{
		FieldTypeText, FieldTypeTextarea, FieldTypeRichText,
		FieldTypeNumber, FieldTypeCurrency, FieldTypePercentage,
		FieldTypeBoolean,
		FieldTypeDate, FieldTypeDatetime, FieldTypeTime, FieldTypeDuration,
		FieldTypeSelect, FieldTypeMultiselect,
		FieldTypeURL, FieldTypeEmail, FieldTypePhone,
		FieldTypeColor, FieldTypeRating,
		FieldTypeImageSingle, FieldTypeImageMulti, FieldTypeMedia,
		FieldTypePlatformEmbed, FieldTypeReference,
	}
	for _, ft := range valid {
		if !IsValidFieldType(ft) {
			t.Errorf("IsValidFieldType(%q) = false, want true", ft)
		}
	}

	for _, ft := range []string{"foo", "", "invalid", "checkbox", "radio"} {
		if IsValidFieldType(ft) {
			t.Errorf("IsValidFieldType(%q) = true, want false", ft)
		}
	}
}

func TestFieldTypeValueColumn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fieldType string
		wantCol   string
		wantOk    bool
	}{
		{FieldTypeText, "value_text", true},
		{FieldTypeTextarea, "value_text", true},
		{FieldTypeURL, "value_text", true},
		{FieldTypeEmail, "value_text", true},
		{FieldTypePhone, "value_text", true},
		{FieldTypeColor, "value_text", true},
		{FieldTypePlatformEmbed, "value_text", true},
		{FieldTypeRichText, "value_json", true},
		{FieldTypeSelect, "value_json", true},
		{FieldTypeMultiselect, "value_json", true},
		{FieldTypeMedia, "value_json", true},
		{FieldTypeImageSingle, "value_json", true},
		{FieldTypeImageMulti, "value_json", true},
		{FieldTypeReference, "value_json", true},
		{FieldTypeNumber, "value_number", true},
		{FieldTypeCurrency, "value_number", true},
		{FieldTypePercentage, "value_number", true},
		{FieldTypeRating, "value_number", true},
		{FieldTypeBoolean, "value_bool", true},
		{FieldTypeDate, "value_date", true},
		{FieldTypeDatetime, "value_date", true},
		{FieldTypeTime, "value_date", true},
		{FieldTypeDuration, "value_date", true},
		{"invalid", "", false},
	}

	for _, tc := range tests {
		gotCol, gotOk := FieldTypeValueColumn(tc.fieldType)
		if gotCol != tc.wantCol || gotOk != tc.wantOk {
			t.Errorf("FieldTypeValueColumn(%q) = (%q, %v), want (%q, %v)",
				tc.fieldType, gotCol, gotOk, tc.wantCol, tc.wantOk)
		}
	}
}

func TestValidOptionsShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fieldType string
		opts      []FieldOption
		wantOk    bool
		wantMsg   string
	}{
		{
			name:      "select with valid options",
			fieldType: FieldTypeSelect,
			opts:      []FieldOption{{Value: "opt1", Label: "Option 1"}, {Value: "opt2", Label: "Option 2"}},
			wantOk:    true,
		},
		{
			name:      "multiselect with valid options",
			fieldType: FieldTypeMultiselect,
			opts:      []FieldOption{{Value: "a", Label: "A"}},
			wantOk:    true,
		},
		{
			name:      "select with empty options",
			fieldType: FieldTypeSelect,
			opts:      nil,
			wantOk:    false,
			wantMsg:   "Select and multiselect fields require at least one option",
		},
		{
			name:      "select with missing value",
			fieldType: FieldTypeSelect,
			opts:      []FieldOption{{Value: "", Label: "Label"}},
			wantOk:    false,
			wantMsg:   "Each option must have both value and label",
		},
		{
			name:      "select with missing label",
			fieldType: FieldTypeSelect,
			opts:      []FieldOption{{Value: "v", Label: ""}},
			wantOk:    false,
			wantMsg:   "Each option must have both value and label",
		},
		{
			name:      "number with no options is ok",
			fieldType: FieldTypeNumber,
			opts:      nil,
			wantOk:    true,
		},
		{
			name:      "text with no options is ok",
			fieldType: FieldTypeText,
			opts:      nil,
			wantOk:    true,
		},
		{
			name:      "image_single with no options is ok",
			fieldType: FieldTypeImageSingle,
			opts:      nil,
			wantOk:    true,
		},
		{
			name:      "reference with no options is ok",
			fieldType: FieldTypeReference,
			opts:      nil,
			wantOk:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg, ok := ValidOptionsShape(tc.fieldType, tc.opts)
			if ok != tc.wantOk {
				t.Errorf("ValidOptionsShape() ok = %v, want %v", ok, tc.wantOk)
			}
			if msg != tc.wantMsg {
				t.Errorf("ValidOptionsShape() msg = %q, want %q", msg, tc.wantMsg)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	httpx.WriteJSON(rr, http.StatusCreated, map[string]string{"status": "ok"})

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusCreated)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if got["status"] != "ok" {
		t.Errorf(`body["status"] = %q, want "ok"`, got["status"])
	}
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	httpx.WriteError(rr, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	got := decodeErrorResponse(t, rr.Body.Bytes())
	if got.Error.Code != "invalid_body" {
		t.Errorf("error.code = %q, want invalid_body", got.Error.Code)
	}

	if got.Error.Message != "Request body is not valid JSON" {
		t.Errorf("error.message = %q, want Request body is not valid JSON", got.Error.Message)
	}
}

func TestParseUUIDParam(t *testing.T) {
	t.Parallel()

	const valid = "550e8400-e29b-41d4-a716-446655440000"
	r := requestWithURLParam(http.MethodGet, "/", "id", valid, "")

	id, err := httpx.ParseUUIDParam(r, "id")
	if err != nil {
		t.Fatalf("parseUUIDParam valid UUID: %v", err)
	}

	if !id.Valid {
		t.Fatal("expected valid pgtype.UUID, got invalid zero value")
	}

	r2 := requestWithURLParam(http.MethodGet, "/", "id", "not-a-uuid", "")
	if _, err := httpx.ParseUUIDParam(r2, "id"); err == nil {
		t.Fatal("expected error for invalid UUID, got nil")
	}
}

func TestCreateFieldGroup_MissingFields(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/custom-field-groups", strings.NewReader(`{}`))
	h.CreateFieldGroup(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "missing_fields" {
		t.Errorf("error.code = %q, want missing_fields", got.Error.Code)
	}
}

func TestCreateFieldGroup_InvalidBody(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/custom-field-groups", strings.NewReader(`{not json`))
	h.CreateFieldGroup(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "invalid_body" {
		t.Errorf("error.code = %q, want invalid_body", got.Error.Code)
	}
}

func TestCreateField_InvalidFieldType(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)

	const groupID = "550e8400-e29b-41d4-a716-446655440000"

	body := `{"label":"Test","key":"test","field_type":"invalid"}`
	r := requestWithURLParam(
		http.MethodPost,
		"/api/v1/custom-field-groups/"+groupID+"/fields",
		"id", groupID, body,
	)
	rr := httptest.NewRecorder()
	h.CreateField(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "invalid_field_type" {
		t.Errorf("error.code = %q, want invalid_field_type", got.Error.Code)
	}
}

func TestSubmitForm_InvalidBody(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)

	const formID = "550e8400-e29b-41d4-a716-446655440000"
	r := requestWithURLParam(
		http.MethodPost,
		"/api/v1/forms/"+formID+"/submissions",
		"id", formID, `{not json`,
	)
	rr := httptest.NewRecorder()
	h.SubmitForm(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "invalid_body" {
		t.Errorf("error.code = %q, want invalid_body", got.Error.Code)
	}
}

func TestUpdateFieldInGroup_InvalidFieldID(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)

	const groupID = "550e8400-e29b-41d4-a716-446655440000"

	body := `{"label":"Test","field_type":"text"}`
	r := requestWithURLParams(
		http.MethodPatch,
		"/api/v1/admin/field-groups/"+groupID+"/fields/not-a-uuid",
		map[string]string{"id": groupID, "fid": "not-a-uuid"},
		body,
	)
	rr := httptest.NewRecorder()
	h.UpdateFieldInGroup(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "invalid_id" {
		t.Errorf("error.code = %q, want invalid_id", got.Error.Code)
	}
}

func TestUpdateFieldInGroup_InvalidFieldType(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)

	const groupID = "550e8400-e29b-41d4-a716-446655440000"
	const fieldID = "660e8400-e29b-41d4-a716-446655440000"

	body := `{"label":"Test","field_type":"invalid"}`
	r := requestWithURLParams(
		http.MethodPatch,
		"/api/v1/admin/field-groups/"+groupID+"/fields/"+fieldID,
		map[string]string{"id": groupID, "fid": fieldID},
		body,
	)
	rr := httptest.NewRecorder()
	h.UpdateFieldInGroup(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "invalid_field_type" {
		t.Errorf("error.code = %q, want invalid_field_type", got.Error.Code)
	}
}

func TestDeleteFieldInGroup_InvalidFieldID(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)

	const groupID = "550e8400-e29b-41d4-a716-446655440000"

	r := requestWithURLParams(
		http.MethodDelete,
		"/api/v1/admin/field-groups/"+groupID+"/fields/not-a-uuid",
		map[string]string{"id": groupID, "fid": "not-a-uuid"},
		"",
	)
	rr := httptest.NewRecorder()
	h.DeleteFieldInGroup(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "invalid_id" {
		t.Errorf("error.code = %q, want invalid_id", got.Error.Code)
	}
}

func TestReorderFields_InvalidGroupID(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)

	r := requestWithURLParam(
		http.MethodPost,
		"/api/v1/admin/field-groups/not-a-uuid/fields/reorder",
		"id", "not-a-uuid", `{"fields":[]}`,
	)
	rr := httptest.NewRecorder()
	h.ReorderFields(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "invalid_id" {
		t.Errorf("error.code = %q, want invalid_id", got.Error.Code)
	}
}

func TestReorderFields_InvalidBody(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)

	const groupID = "550e8400-e29b-41d4-a716-446655440000"

	r := requestWithURLParam(
		http.MethodPost,
		"/api/v1/admin/field-groups/"+groupID+"/fields/reorder",
		"id", groupID, `{not json`,
	)
	rr := httptest.NewRecorder()
	h.ReorderFields(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "invalid_body" {
		t.Errorf("error.code = %q, want invalid_body", got.Error.Code)
	}
}

func TestGetFormSubmission_InvalidFormID(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)

	const subID = "550e8400-e29b-41d4-a716-446655440000"

	r := requestWithURLParams(
		http.MethodGet,
		"/api/v1/admin/forms/not-a-uuid/submissions/"+subID,
		map[string]string{"id": "not-a-uuid", "sub_id": subID},
		"",
	)
	rr := httptest.NewRecorder()
	h.GetFormSubmission(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "invalid_id" {
		t.Errorf("error.code = %q, want invalid_id", got.Error.Code)
	}
}

func TestGetFormSubmission_InvalidSubmissionID(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)

	const formID = "550e8400-e29b-41d4-a716-446655440000"

	r := requestWithURLParams(
		http.MethodGet,
		"/api/v1/admin/forms/"+formID+"/submissions/not-a-uuid",
		map[string]string{"id": formID, "sub_id": "not-a-uuid"},
		"",
	)
	rr := httptest.NewRecorder()
	h.GetFormSubmission(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "invalid_id" {
		t.Errorf("error.code = %q, want invalid_id", got.Error.Code)
	}
}

func TestUpdateFormSubmission_InvalidBody(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)

	const formID = "550e8400-e29b-41d4-a716-446655440000"
	const subID = "660e8400-e29b-41d4-a716-446655440000"

	r := requestWithURLParams(
		http.MethodPatch,
		"/api/v1/admin/forms/"+formID+"/submissions/"+subID,
		map[string]string{"id": formID, "sub_id": subID},
		`{not json`,
	)
	rr := httptest.NewRecorder()
	h.UpdateFormSubmission(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "invalid_body" {
		t.Errorf("error.code = %q, want invalid_body", got.Error.Code)
	}
}

func TestUpdateFormSubmission_MissingStatus(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)

	const formID = "550e8400-e29b-41d4-a716-446655440000"
	const subID = "660e8400-e29b-41d4-a716-446655440000"

	r := requestWithURLParams(
		http.MethodPatch,
		"/api/v1/admin/forms/"+formID+"/submissions/"+subID,
		map[string]string{"id": formID, "sub_id": subID},
		`{}`,
	)
	rr := httptest.NewRecorder()
	h.UpdateFormSubmission(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "missing_fields" {
		t.Errorf("error.code = %q, want missing_fields", got.Error.Code)
	}
}

func TestDeleteFormSubmission_InvalidSubmissionID(t *testing.T) {
	t.Parallel()

	h := NewHandlers(nil, nil)

	const formID = "550e8400-e29b-41d4-a716-446655440000"

	r := requestWithURLParams(
		http.MethodDelete,
		"/api/v1/admin/forms/"+formID+"/submissions/not-a-uuid",
		map[string]string{"id": formID, "sub_id": "not-a-uuid"},
		"",
	)
	rr := httptest.NewRecorder()
	h.DeleteFormSubmission(rr, r)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	if got := decodeErrorResponse(t, rr.Body.Bytes()); got.Error.Code != "invalid_id" {
		t.Errorf("error.code = %q, want invalid_id", got.Error.Code)
	}
}

func TestSupportedEmbedPlatforms(t *testing.T) {
	t.Parallel()

	want := map[string]bool{
		"youtube": true, "github": true, "twitter": true, "vimeo": true,
		"spotify": true, "figma": true, "codepen": true, "google_maps": true,
	}
	if len(SupportedEmbedPlatforms) != len(want) {
		t.Fatalf("len(SupportedEmbedPlatforms) = %d, want %d", len(SupportedEmbedPlatforms), len(want))
	}

	for _, p := range SupportedEmbedPlatforms {
		if !want[p] {
			t.Errorf("unexpected platform %q in SupportedEmbedPlatforms", p)
		}
	}
}
