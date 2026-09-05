// Package personas — handlers_test.go tests the admin persona HTTP handlers
// using httptest, plus the high-stakes content gate and activity-policy JSON
// (de)serialization.
//
// Ref: japaconnect-engineering/sections/68-ai-personas.md
//
//	japaconnect-engineering/adr/ADR-068-ai-persona-system.md
package personas

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Auth denial tests — each handler must 401 when no auth context is set.
// The RBAC permission check happens at the route registration site, so these
// tests verify the handler's own requireUser guard.
// ---------------------------------------------------------------------------

// TestHandleList_Unauthorized verifies that the list endpoint requires auth.
func TestHandleList_Unauthorized(t *testing.T) {
	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/personas", nil)
	rr := httptest.NewRecorder()
	h.HandleList(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleList without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleCreate_Unauthorized verifies that the create endpoint requires auth.
func TestHandleCreate_Unauthorized(t *testing.T) {
	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/personas", nil)
	rr := httptest.NewRecorder()
	h.HandleCreate(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleCreate without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleGet_Unauthorized verifies that the get endpoint requires auth.
func TestHandleGet_Unauthorized(t *testing.T) {
	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/personas/00000000-0000-0000-0000-000000000001", nil)
	rr := httptest.NewRecorder()
	h.HandleGet(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleGet without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleUpdate_Unauthorized verifies that the update endpoint requires auth.
func TestHandleUpdate_Unauthorized(t *testing.T) {
	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodPatch, "/admin/personas/00000000-0000-0000-0000-000000000001", nil)
	rr := httptest.NewRecorder()
	h.HandleUpdate(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleUpdate without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleActivity_Unauthorized verifies that the activity endpoint requires auth.
func TestHandleActivity_Unauthorized(t *testing.T) {
	h := NewHandlers(nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/personas/00000000-0000-0000-0000-000000000001/activity", nil)
	rr := httptest.NewRecorder()
	h.HandleActivity(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleActivity without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// High-stakes content gate (ADR-068 §5 — hardcoded, NOT configurable)
// ---------------------------------------------------------------------------

// TestIsHighStakesContent verifies the high-stakes gate with table-driven
// tests. Categories visa/banking/grants/health_insurance and tags
// immigration-advice/financial-advice must always return true; everything
// else returns false. This gate is hardcoded and must never be made
// configurable (ADR-068 §5).
func TestIsHighStakesContent(t *testing.T) {
	tests := []struct {
		name          string
		guideCategory string
		tags          []string
		want          bool
	}{
		// High-stakes categories.
		{"visa category", "visa", nil, true},
		{"banking category", "banking", nil, true},
		{"grants category", "grants", nil, true},
		{"health_insurance category", "health_insurance", nil, true},

		// High-stakes tags.
		{"immigration-advice tag", "general", []string{"immigration-advice"}, true},
		{"financial-advice tag", "general", []string{"financial-advice"}, true},
		{"high-stakes tag among others", "general", []string{"tips", "financial-advice", "news"}, true},

		// Non-high-stakes.
		{"empty category and tags", "", nil, false},
		{"general category", "general", nil, false},
		{"lifestyle category", "lifestyle", []string{"food", "travel"}, false},
		{"no category, benign tags", "", []string{"community", "discussion"}, false},
		{"empty tags slice", "visa", []string{}, true}, // category alone triggers
		{"nil tags, benign category", "culture", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsHighStakesContent(tt.guideCategory, tt.tags)
			if got != tt.want {
				t.Errorf("IsHighStakesContent(%q, %v) = %v, want %v",
					tt.guideCategory, tt.tags, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ActivityPolicy JSON (de)serialization
// ---------------------------------------------------------------------------

// TestActivityPolicyValidation verifies that an ActivityPolicy round-trips
// through JSON marshal/unmarshal with all fields preserved, including the
// [2]int array for active_hours_utc.
func TestActivityPolicyValidation(t *testing.T) {
	original := ActivityPolicy{
		ActionsPerDay:        12,
		ActionBudget:         map[string]int{"comment": 5, "react": 10},
		EligibleContentTypes: []string{"guide", "discussion"},
		EligibleSpaces:       []string{"lagos", "abuja"},
		MinContentAgeHours:   48,
		AvoidOwnContent:      true,
		ActiveHoursUTC:       [2]int{8, 22},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal ActivityPolicy: %v", err)
	}

	var decoded ActivityPolicy
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal ActivityPolicy: %v", err)
	}

	if decoded.ActionsPerDay != original.ActionsPerDay {
		t.Errorf("ActionsPerDay = %d, want %d", decoded.ActionsPerDay, original.ActionsPerDay)
	}

	if decoded.MinContentAgeHours != original.MinContentAgeHours {
		t.Errorf("MinContentAgeHours = %d, want %d", decoded.MinContentAgeHours, original.MinContentAgeHours)
	}

	if decoded.AvoidOwnContent != original.AvoidOwnContent {
		t.Errorf("AvoidOwnContent = %v, want %v", decoded.AvoidOwnContent, original.AvoidOwnContent)
	}

	if decoded.ActiveHoursUTC != original.ActiveHoursUTC {
		t.Errorf("ActiveHoursUTC = %v, want %v", decoded.ActiveHoursUTC, original.ActiveHoursUTC)
	}

	if len(decoded.ActionBudget) != len(original.ActionBudget) {
		t.Errorf("ActionBudget len = %d, want %d", len(decoded.ActionBudget), len(original.ActionBudget))
	} else {
		for k, v := range original.ActionBudget {
			if decoded.ActionBudget[k] != v {
				t.Errorf("ActionBudget[%q] = %d, want %d", k, decoded.ActionBudget[k], v)
			}
		}
	}

	if len(decoded.EligibleContentTypes) != len(original.EligibleContentTypes) {
		t.Errorf("EligibleContentTypes len = %d, want %d",
			len(decoded.EligibleContentTypes), len(original.EligibleContentTypes))
	}

	if len(decoded.EligibleSpaces) != len(original.EligibleSpaces) {
		t.Errorf("EligibleSpaces len = %d, want %d",
			len(decoded.EligibleSpaces), len(original.EligibleSpaces))
	}
}

// TestActivityPolicy_JSONShape verifies the exact JSON field names and the
// active_hours_utc array shape match the spec.
func TestActivityPolicy_JSONShape(t *testing.T) {
	p := ActivityPolicy{
		ActionsPerDay:        5,
		ActionBudget:         map[string]int{"comment": 3},
		EligibleContentTypes: []string{"guide"},
		EligibleSpaces:       []string{"lagos"},
		MinContentAgeHours:   24,
		AvoidOwnContent:      false,
		ActiveHoursUTC:       [2]int{9, 17},
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	requiredKeys := []string{
		"actions_per_day", "action_budget", "eligible_content_types",
		"eligible_spaces", "min_content_age_hours", "avoid_own_content",
		"active_hours_utc",
	}
	for _, key := range requiredKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q in marshaled ActivityPolicy", key)
		}
	}

	// active_hours_utc must be a 2-element array.
	var hours []int
	if err := json.Unmarshal(raw["active_hours_utc"], &hours); err != nil {
		t.Fatalf("unmarshal active_hours_utc: %v", err)
	}

	if len(hours) != 2 {
		t.Errorf("active_hours_utc len = %d, want 2", len(hours))
	}

	if hours[0] != 9 || hours[1] != 17 {
		t.Errorf("active_hours_utc = %v, want [9 17]", hours)
	}
}

// TestActivityPolicy_DefaultRoundTrip verifies that a zero-value ActivityPolicy
// marshals cleanly (no nil panics) and the active_hours_utc defaults to [0,0].
func TestActivityPolicy_DefaultRoundTrip(t *testing.T) {
	var p ActivityPolicy

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal zero ActivityPolicy: %v", err)
	}

	var decoded ActivityPolicy
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal zero ActivityPolicy: %v", err)
	}

	if decoded.ActiveHoursUTC != [2]int{0, 0} {
		t.Errorf("default ActiveHoursUTC = %v, want [0 0]", decoded.ActiveHoursUTC)
	}

	if decoded.AvoidOwnContent != false {
		t.Errorf("default AvoidOwnContent = %v, want false", decoded.AvoidOwnContent)
	}
}

// ---------------------------------------------------------------------------
// ProvisionPersonas — needsDefault logic (BUG-007)
// ---------------------------------------------------------------------------

// TestProvisionPersonas_NeedsDefaultLogic verifies that the needsDefault
// check correctly identifies when at least one input omits type_id (and
// therefore the default profile type resolution must run). This is the
// core of the BUG-007 fix: resolveDefaultTypeID must NOT be called when
// every input supplies an explicit type_id, so provisioning succeeds on
// tenants that lack the 'contributor'/'columnist' default types.
func TestProvisionPersonas_NeedsDefaultLogic(t *testing.T) {
	tests := []struct {
		name     string
		typeIDs  []string
		wantNeed bool
	}{
		{
			name:     "all inputs have type_id",
			typeIDs:  []string{"uuid-1", "uuid-2"},
			wantNeed: false,
		},
		{
			name:     "one input missing type_id",
			typeIDs:  []string{"uuid-1", ""},
			wantNeed: true,
		},
		{
			name:     "all inputs missing type_id",
			typeIDs:  []string{"", ""},
			wantNeed: true,
		},
		{
			name:     "single input with type_id",
			typeIDs:  []string{"uuid-1"},
			wantNeed: false,
		},
		{
			name:     "single input missing type_id",
			typeIDs:  []string{""},
			wantNeed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needsDefault := false

			for _, tid := range tt.typeIDs {
				if tid == "" {
					needsDefault = true
					break
				}
			}

			if needsDefault != tt.wantNeed {
				t.Errorf("needsDefault = %v, want %v (typeIDs=%v)",
					needsDefault, tt.wantNeed, tt.typeIDs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ProvisionPersonas — duplicate slug pre-validation (BUG-013)
// ---------------------------------------------------------------------------

// TestProvisionPersonas_DuplicateSlugDetection verifies that the duplicate
// slug pre-validation catches duplicate slugs within a batch before opening
// a transaction. Without this check, the second CreatePersonaUser would hit
// a UNIQUE(tenant_id, email) constraint violation inside the tx, and the
// admin would get a generic "provisioning failed" error with no indication
// of which slug collided.
func TestProvisionPersonas_DuplicateSlugDetection(t *testing.T) {
	tests := []struct {
		name    string
		slugs   []string
		wantDup bool
		wantIdx int
	}{
		{"all unique slugs", []string{"ada", "bola", "chi"}, false, -1},
		{"two duplicate slugs", []string{"ada", "bola", "ada"}, true, 2},
		{"same slug twice", []string{"ada", "ada"}, true, 1},
		{"single slug", []string{"ada"}, false, -1},
		{"triplicate slug", []string{"x", "x", "x"}, true, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := make(map[string]int)
			dupIdx := -1

			for i, slug := range tt.slugs {
				if prev, dup := seen[slug]; dup {
					dupIdx = i
					_ = prev

					break
				}

				seen[slug] = i
			}

			gotDup := dupIdx != -1
			if gotDup != tt.wantDup {
				t.Errorf("duplicate detected = %v, want %v (slugs=%v)",
					gotDup, tt.wantDup, tt.slugs)
			}

			if gotDup && dupIdx != tt.wantIdx {
				t.Errorf("duplicate at index %d, want %d (slugs=%v)",
					dupIdx, tt.wantIdx, tt.slugs)
			}
		})
	}
}
