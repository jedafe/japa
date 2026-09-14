package literals

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers exposes read-only template-literal endpoints for non-admin
// surfaces — the composer `{{` token picker and any client-side renderers
// that need the active token list. Expressions/values are never exposed:
// only the key, label, description, category, and kind are returned.
type Handlers struct {
	catalog *Catalog
}

// NewHandlers creates Handlers backed by the shared literal catalog.
func NewHandlers(catalog *Catalog) *Handlers {
	return &Handlers{catalog: catalog}
}

// literalListItem is the public shape returned by GET /api/v1/literals.
// The raw `value` (static text or admin-authored expression) is deliberately
// omitted — the picker only needs identity + display metadata.
type literalListItem struct {
	Key         string  `json:"key"`
	Kind        string  `json:"kind"`
	Label       *string `json:"label,omitempty"`
	Description *string `json:"description,omitempty"`
	Category    *string `json:"category,omitempty"`
	IsSystem    bool    `json:"is_system"`
}

// List handles GET /api/v1/literals?q=.
//
// OptionalAuth: token keys/labels are display metadata, not sensitive data —
// the endpoint is public so guest-facing renderers and the composer picker can
// share it. Results come from the Valkey-cached catalog (jc:literals:{tenant})
// so no per-request DB query runs. ?q= is a case-insensitive substring match
// on key and label.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	// Nil catalog (unwired in tests) — degrade to an empty list.
	if h.catalog == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []literalListItem{}})
		return
	}

	lits, err := h.catalog.GetActive(r.Context())
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list literals")
		return
	}

	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

	out := make([]literalListItem, 0, len(lits))
	for _, lit := range lits {
		if !lit.IsActive {
			continue
		}
		if q != "" && !literalMatches(lit, q) {
			continue
		}
		out = append(out, literalListItem{
			Key:         lit.Key,
			Kind:        lit.Kind,
			Label:       lit.Label,
			Description: lit.Description,
			Category:    lit.Category,
			IsSystem:    lit.IsSystem,
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

func literalMatches(lit TemplateLiteral, q string) bool {
	if strings.Contains(strings.ToLower(lit.Key), q) {
		return true
	}
	if lit.Label != nil && strings.Contains(strings.ToLower(*lit.Label), q) {
		return true
	}
	return false
}
