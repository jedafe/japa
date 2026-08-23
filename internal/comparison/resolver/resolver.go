// Package resolver resolves item_ref references into normalized items that can
// be compared in the Japa Comparison Engine. It keeps the comparison package
// decoupled from the concrete services that own each entity type.
package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ResolvedItem is the normalized representation of an entity that can be
// selected as a comparison item.
type ResolvedItem struct {
	Type        string         `json:"type"`
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	ImageURL    string         `json:"image_url,omitempty"`
	URL         string         `json:"url,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// Resolver knows how to resolve and search one entity type.
type Resolver interface {
	Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, bool, error)
	Search(ctx context.Context, query string, filters map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error)
}

// Registry maps entity types to resolvers.
type Registry struct {
	resolvers map[string]Resolver
}

// NewRegistry creates an empty resolver registry.
func NewRegistry() *Registry {
	return &Registry{resolvers: make(map[string]Resolver)}
}

// Register adds a resolver for an entity type.
func (r *Registry) Register(entityType string, resolver Resolver) {
	if r == nil {
		return
	}
	if r.resolvers == nil {
		r.resolvers = make(map[string]Resolver)
	}
	if resolver == nil {
		return
	}
	r.resolvers[entityType] = resolver
}

// Resolve resolves an item_ref for the type declared in ref["type"].
func (r *Registry) Resolve(ctx context.Context, ref map[string]any, tenantID pgtype.UUID) (ResolvedItem, error) {
	if r == nil || r.resolvers == nil {
		return ResolvedItem{}, errors.New("resolver registry not initialized")
	}
	typ, ok := ref["type"].(string)
	if !ok || typ == "" {
		return ResolvedItem{}, errors.New("item_ref missing type")
	}
	res, ok := r.resolvers[typ]
	if !ok {
		return ResolvedItem{}, fmt.Errorf("unsupported item_ref type: %s", typ)
	}
	item, found, err := res.Resolve(ctx, ref, tenantID)
	if err != nil {
		return ResolvedItem{}, fmt.Errorf("resolving %s: %w", typ, err)
	}
	if !found {
		return ResolvedItem{}, fmt.Errorf("%s item not found", typ)
	}
	item.Type = typ
	return item, nil
}

// Search searches for items of a given entity type.
func (r *Registry) Search(ctx context.Context, entityType, query string, filters map[string]any, tenantID pgtype.UUID, limit int32) ([]ResolvedItem, error) {
	if r == nil || r.resolvers == nil {
		return nil, errors.New("resolver registry not initialized")
	}
	res, ok := r.resolvers[entityType]
	if !ok {
		return nil, fmt.Errorf("unsupported entity type: %s", entityType)
	}
	items, err := res.Search(ctx, query, filters, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("searching %s: %w", entityType, err)
	}
	for i := range items {
		items[i].Type = entityType
	}
	return items, nil
}

// ParseUUID parses a UUID string into a pgtype.UUID.
func ParseUUID(s string) (pgtype.UUID, error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid uuid: %w", err)
	}
	return pgtype.UUID{Bytes: u, Valid: true}, nil
}

// IsNotFound reports whether an error chain indicates a missing entity.
func IsNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// CoerceMap coerces a JSON-encoded item_ref into a map[string]any.
func CoerceMap(data []byte) (map[string]any, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing item_ref: %w", err)
	}
	return m, nil
}

// Excerpt returns a short plain-text excerpt from a possibly HTML or JSON body.
func Excerpt(body string, maxLen int) string {
	plain := stripTagsAndJSON(body)
	plain = strings.Join(strings.Fields(plain), " ")
	if maxLen <= 0 {
		maxLen = 200
	}
	if len(plain) <= maxLen {
		return plain
	}
	return plain[:maxLen] + "…"
}

func stripTagsAndJSON(s string) string {
	var out strings.Builder
	inTag := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '<' {
			inTag = true
			continue
		}
		if c == '>' {
			inTag = false
			continue
		}
		if inTag {
			continue
		}
		out.WriteByte(c)
	}
	text := out.String()
	if len(text) > 0 && text[0] == '{' {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err == nil {
			var sb strings.Builder
			flattenJSONMap(m, &sb)
			return sb.String()
		}
	}
	return text
}

func flattenJSONMap(m map[string]any, sb *strings.Builder) {
	for _, v := range m {
		switch val := v.(type) {
		case string:
			if sb.Len() > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(val)
		case []any:
			for _, item := range val {
				if m2, ok := item.(map[string]any); ok {
					flattenJSONMap(m2, sb)
				}
			}
		}
	}
}
