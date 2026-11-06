package wiki

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	errpkg "github.com/jedafe/japa/internal/errors"

	"context"
	"fmt"
	"strings"
)

// Wiki entity directory (§80 Entity Directory amendment): /wiki/e and
// /wiki/e/{type} — the index + per-kind listing surfaces. The kind→table
// binding is wiki_entity_kinds (source_table/id_column/slug_column);
// name/discriminator columns are fixed per kind mirroring ResolveEntityRef.
// All identifiers interpolated into dynamic SQL come from the kinds row —
// never from request input.

// kindListSource describes each entity kind's listing source: the FROM
// clause (joins allowed), the name/slug expressions, and the base WHERE —
// all fixed constants, never request-derived. Mirrors ResolveEntityRef.
type kindListSource struct {
	from     string
	idExpr   string // qualified — joins make bare "id" ambiguous
	nameExpr string
	slugExpr string
	where    string
}

var kindListSources = map[string]kindListSource{
	"visa_route": {
		"visa_routes", "id", "name", "slug", "is_active = true",
	},
	"organization": {
		"organizations", "id", "name", "slug", "deleted_at IS NULL",
	},
	"bank": {
		"banks", "id", "name", "slug", "is_active = true",
	},
	"embassy": {
		"embassies", "id", "name", "slug", "is_active = true",
	},
	"course": {
		"university_course_offerings o JOIN university_course_catalog c ON c.id = o.course_id",
		"o.id", "c.title", "o.slug", "o.deleted_at IS NULL AND o.is_published = true",
	},
	"program": {
		"university_course_catalog", "id", "title", "slug", "deleted_at IS NULL AND is_published = true",
	},
	"scholarship": {
		"scholarships", "id", "title", "slug", "",
	},
}

// kindDiscriminatorColumn maps kind → its discriminator column, "" = none.
var kindDiscriminatorColumn = map[string]string{
	"visa_route":   "visa_category",
	"organization": "org_type",
}

// EntityKindDTO is one entry on the /wiki/e index.
type EntityKindDTO struct {
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	EntityCount int64  `json:"entity_count"`
}

// EntityDirectoryItem is one row on /wiki/e/{type}.
type EntityDirectoryItem struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Discriminator string `json:"discriminator,omitempty"`
	Href          string `json:"href"` // canonical /wiki/e/{type_slug}/{slug}/overview
}

// ListEntityKinds serves the /wiki/e index — every kind with a live count
// from its source table.
func (s *Service) ListEntityKinds(ctx context.Context) ([]EntityKindDTO, error) {
	kinds, err := s.store.read.ListWikiEntityKinds(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing entity kinds: %w", err)
	}

	out := make([]EntityKindDTO, 0, len(kinds))
	for _, k := range kinds {
		var n int64
		// source_table is the trusted kinds-row identifier.
		if err := s.store.pool.Reader.QueryRow(ctx,
			fmt.Sprintf("SELECT count(*) FROM %s", k.SourceTable)).Scan(&n); err != nil {
			n = 0
		}
		out = append(out, EntityKindDTO{Kind: k.EntityKind, Label: k.Label, EntityCount: n})
	}
	return out, nil
}

// ListEntitiesByKind pages one kind's entities. kind is the entity_kind
// (e.g. 'program'), not the wiki type key — hrefs canonicalize through
// the kind's discriminator-matching type.
func (s *Service) ListEntitiesByKind(ctx context.Context, kind, q string, limit, offset int) ([]EntityDirectoryItem, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 24
	}
	if offset < 0 {
		offset = 0
	}

	if _, err := s.store.read.GetWikiEntityKind(ctx, kind); err != nil {
		return nil, 0, err
	}

	src, ok := kindListSources[kind]
	if !ok {
		return nil, 0, fmt.Errorf("wiki: kind %q has no listing source", kind)
	}
	discCol := kindDiscriminatorColumn[kind]

	selectDisc := "NULL::text AS discriminator"
	orderBy := src.slugExpr
	if discCol != "" {
		selectDisc = discCol + " AS discriminator"
		orderBy = src.nameExpr
	}

	where := src.where
	args := []any{}
	if q != "" {
		clause := fmt.Sprintf("%s ILIKE '%%' || $1 || '%%'", src.nameExpr)
		if where == "" {
			where = clause
		} else {
			where += " AND " + clause
		}
		args = append(args, q)
	}
	if where != "" {
		where = " WHERE " + where
	}

	var total int64
	if err := s.store.pool.Reader.QueryRow(ctx,
		fmt.Sprintf("SELECT count(*) FROM %s%s", src.from, where), args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting %s entities: %w", kind, err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(
		"SELECT %s AS id, %s AS slug, %s AS name, %s FROM %s%s ORDER BY %s LIMIT %d OFFSET %d",
		src.idExpr, src.slugExpr, src.nameExpr, selectDisc, src.from, where, orderBy, limit, offset))

	rows, err := s.store.pool.Reader.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing %s entities: %w", kind, err)
	}
	defer rows.Close()

	out := []EntityDirectoryItem{}
	for rows.Next() {
		var it EntityDirectoryItem
		var disc *string
		if err := rows.Scan(&it.ID, &it.Slug, &it.Name, &disc); err != nil {
			return nil, 0, err
		}
		if disc != nil {
			it.Discriminator = *disc
		}
		typeSlug := s.canonicalEntityTypeSlug(ctx, kind, it.Discriminator)
		if typeSlug == "" {
			typeSlug = kind
		}
		it.Href = fmt.Sprintf("/wiki/e/%s/%s/overview", typeSlug, it.Slug)
		out = append(out, it)
	}
	return out, total, rows.Err()
}

// HandleListEntityKinds serves GET /wiki/entity-kinds — the /wiki/e index.
func (h *Handlers) HandleListEntityKinds(w http.ResponseWriter, r *http.Request) {
	kinds, err := h.service.ListEntityKinds(r.Context())
	if err != nil {
		slog.Error("wiki: list entity kinds failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to list entity kinds")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": kinds})
}

// HandleListEntitiesByKind serves GET /wiki/entity-kinds/{kind}/entities —
// the /wiki/e/{type} listing.
func (h *Handlers) HandleListEntitiesByKind(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	if kind == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_kind", "Entity kind is required")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	q := r.URL.Query().Get("q")

	items, total, err := h.service.ListEntitiesByKind(r.Context(), kind, q, limit, offset)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "kind_not_found", "Entity kind not found")
			return
		}
		slog.Error("wiki: list entities failed", "kind", kind, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "get_failed", "Failed to list entities")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"items": items, "total": total},
	})
}
