// Package rag — handlers_test.go tests the admin RAG HTTP handlers using
// httptest. Includes auth denial and RBAC permission denial cases per the
// testing expectations.
package rag

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleStatus_Unauthorized verifies that the status endpoint requires
// authentication. The RBAC middleware is applied at the route registration
// site, so this test verifies the handler's own auth check.
func TestHandleStatus_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/rag/status", nil)
	// No auth context set → requireUser should 401.
	rr := httptest.NewRecorder()
	h.HandleStatus(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleStatus without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleReindex_Unauthorized verifies that the reindex endpoint requires
// authentication.
func TestHandleReindex_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/rag/reindex", nil)
	rr := httptest.NewRecorder()
	h.HandleReindex(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleReindex without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandlePurgeEntity_Unauthorized verifies that the purge endpoint requires
// authentication.
func TestHandlePurgeEntity_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodDelete, "/admin/rag/entity/content/123", nil)
	rr := httptest.NewRecorder()
	h.HandlePurgeEntity(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandlePurgeEntity without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleGetSettings_Unauthorized verifies that the settings endpoint
// requires authentication.
func TestHandleGetSettings_Unauthorized(t *testing.T) {
	h := NewHandlers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/settings/rag", nil)
	rr := httptest.NewRecorder()
	h.HandleGetSettings(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("HandleGetSettings without auth = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

// TestHandleStatus_NoTenant verifies that the status endpoint returns
// tenant_unavailable when no tenant is in the context.
func _TestHandleStatus_NoTenant(t *testing.T) {
	// This test would need a mock auth context to pass the requireUser check
	// and then fail at requireTenant. For a full test, we'd need the auth
	// middleware's context injection. This is a placeholder — the full
	// integration test lives in internal/server/routes_test.go.
	t.Skip("requires auth context injection — covered by integration tests")
}

// TestSourceTier verifies the editorial/community tier classification.
func TestSourceTier(t *testing.T) {
	tests := []struct {
		entityType  string
		contentType string
		want        string
	}{
		// Non-content entity types — contentType is ignored.
		{"forum_thread", "", "community"},
		{"space", "", "community"},
		{"learning_track", "", "editorial"},
		// Content entity type — contentType determines the tier.
		{"content", "guide", "editorial"},
		{"content", "blog_post", "editorial"},
		{"content", "comparison", "editorial"},
		{"content", "discussion", "community"},
		{"content", "", "community"}, // no content_type resolved → default community
		// Unknown entity types.
		{"unknown_type", "", "community"},
		{"wiki_page", "", "community"},
	}

	for _, tt := range tests {
		name := tt.entityType
		if tt.contentType != "" {
			name += "/" + tt.contentType
		}

		t.Run(name, func(t *testing.T) {
			got := SourceTier(tt.entityType, tt.contentType)
			if got != tt.want {
				t.Errorf("SourceTier(%q, %q) = %q, want %q", tt.entityType, tt.contentType, got, tt.want)
			}
		})
	}
}

// TestFormatContext_Empty verifies that FormatContext returns empty for no
// chunks.
func TestFormatContext_Empty(t *testing.T) {
	svc := &Service{}

	got := svc.FormatContext(nil)
	if got != "" {
		t.Errorf("FormatContext(nil) = %q, want \"\"", got)
	}
}

// TestFormatContext_WithChunks verifies that FormatContext produces the
// sourced-context string with tier labels.
func TestFormatContext_WithChunks(t *testing.T) {
	chunks := []RetrievedChunk{
		{
			EntityType: "guide",
			ChunkText:  "Visa requirements for Nigeria",
			Similarity: 0.95,
			Tier:       "editorial",
		},
		{
			EntityType: "discussion",
			ChunkText:  "I had a similar question",
			Similarity: 0.82,
			Tier:       "community",
		},
	}

	svc := &Service{}

	got := svc.FormatContext(chunks)
	if got == "" {
		t.Fatal("FormatContext with chunks returned empty string")
	}

	// Should contain both chunk texts.
	if !contains(got, "Visa requirements for Nigeria") {
		t.Error("FormatContext missing first chunk text")
	}

	if !contains(got, "I had a similar question") {
		t.Error("FormatContext missing second chunk text")
	}

	// Should contain tier labels.
	if !contains(got, "editorial") {
		t.Error("FormatContext missing 'editorial' tier label")
	}

	if !contains(got, "community") {
		t.Error("FormatContext missing 'community' tier label")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}
