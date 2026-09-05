package widgets

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Service orchestrates widget CRUD and rendering with Valkey caching.
type Service struct {
	store    *Store
	renderer *Renderer
	redis    valkeycompat.Cmdable
}

// NewService creates a Service.
func NewService(store *Store, renderer *Renderer, redis valkeycompat.Cmdable) *Service {
	return &Service{store: store, renderer: renderer, redis: redis}
}

// SetRedis allows late binding of the Redis client (matching the pattern used
// by other services that are constructed before the Valkey client is ready).
func (s *Service) SetRedis(r valkeycompat.Cmdable) {
	s.redis = r
}

// List returns a paginated list of widgets for a tenant.
func (s *Service) List(ctx context.Context, tenantID pgtype.UUID, q string, page, limit int32) ([]Widget, int64, error) {
	return s.store.List(ctx, tenantID, q, page, limit)
}

// Get returns a single widget by ID, scoped to the tenant.
func (s *Service) Get(ctx context.Context, id, tenantID pgtype.UUID) (Widget, error) {
	return s.store.Get(ctx, id, tenantID)
}

// Create creates a new widget configuration.
func (s *Service) Create(ctx context.Context, tenantID pgtype.UUID, input CreateWidgetInput) (Widget, error) {
	if input.Name == "" {
		return Widget{}, fmt.Errorf("%w: name is required", errpkg.ErrInvalid)
	}

	if input.WidgetType == "" {
		return Widget{}, fmt.Errorf("%w: widget_type is required", errpkg.ErrInvalid)
	}

	if !IsValidWidgetType(input.WidgetType) {
		return Widget{}, fmt.Errorf("%w: widget_type must be one of: %v", errpkg.ErrInvalid, ValidWidgetTypes)
	}

	return s.store.Create(ctx, tenantID, input)
}

// Update modifies a widget and invalidates its Valkey cache.
func (s *Service) Update(ctx context.Context, id, tenantID pgtype.UUID, input UpdateWidgetInput) (Widget, error) {
	if input.Name != nil && *input.Name == "" {
		return Widget{}, fmt.Errorf("%w: name must not be empty", errpkg.ErrInvalid)
	}

	if input.WidgetType != nil && !IsValidWidgetType(*input.WidgetType) {
		return Widget{}, fmt.Errorf("%w: widget_type must be one of: %v", errpkg.ErrInvalid, ValidWidgetTypes)
	}

	w, err := s.store.Update(ctx, id, tenantID, input)
	if err != nil {
		return Widget{}, err
	}

	s.invalidateCache(ctx, w.ID)

	return w, nil
}

// Delete removes a widget and invalidates its Valkey cache.
func (s *Service) Delete(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.store.Delete(ctx, id, tenantID); err != nil {
		return err
	}

	s.invalidateCache(ctx, id)

	return nil
}

// RenderWidget returns rendered widget data, Valkey-cached by the widget's
// cache_ttl_secs. Ref: Section 16 Widget Rendering Pattern.
func (s *Service) RenderWidget(ctx context.Context, id pgtype.UUID) (RenderedWidget, bool, error) {
	cacheKey := widgetCacheKey(id)

	// Try Valkey cache first.
	if s.redis != nil {
		if cached, err := s.redis.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var rw RenderedWidget
			if err := json.Unmarshal([]byte(cached), &rw); err == nil {
				return rw, true, nil
			}
		}
	}

	// Cache miss — fetch widget config and render fresh.
	w, err := s.store.GetPublic(ctx, id)
	if err != nil {
		return RenderedWidget{}, false, err
	}

	data, err := s.renderer.Render(ctx, w)
	if err != nil {
		return RenderedWidget{}, false, err
	}

	rw := RenderedWidget{
		WidgetID:   id.String(),
		WidgetType: w.WidgetType,
		CardType:   extractCardType(w.Config),
		Data:       data,
		CachedAt:   time.Now(),
	}

	// Cache with the widget's TTL.
	if s.redis != nil {
		serialized, err := json.Marshal(rw)
		if err == nil {
			ttl := time.Duration(w.CacheTTLSecs) * time.Second
			if ttl <= 0 {
				ttl = 300 * time.Second
			}

			_ = s.redis.Set(ctx, cacheKey, string(serialized), ttl)
		}
	}

	return rw, false, nil
}

// RenderWidgetByID resolves a widget by ID and returns (data, widget_type, card_type, error).
// This satisfies the pagebuilder.WidgetResolver interface for server-side
// widget block resolution in GetPublishedPage.
func (s *Service) RenderWidgetByID(ctx context.Context, id pgtype.UUID) (json.RawMessage, string, string, error) {
	rw, _, err := s.RenderWidget(ctx, id)
	if err != nil {
		return nil, "", "", err
	}

	return rw.Data, rw.WidgetType, rw.CardType, nil
}

// extractCardType reads the "card_type" key from a widget's config JSON.
// Returns "" if not present or if the config is invalid.
func extractCardType(config json.RawMessage) string {
	if len(config) == 0 {
		return ""
	}

	var cfg map[string]any
	if err := json.Unmarshal(config, &cfg); err != nil {
		return ""
	}

	if ct, ok := cfg["card_type"].(string); ok {
		return ct
	}

	return ""
}

// invalidateCache deletes the widget's Valkey cache entry.
func (s *Service) invalidateCache(ctx context.Context, id pgtype.UUID) {
	if s.redis == nil {
		return
	}

	_ = s.redis.Del(ctx, widgetCacheKey(id)).Err()
}

// widgetCacheKey returns the Valkey key for a widget's rendered data.
// Ref: conventions/valkey-keys-catalog.md — jc:widget:{widget_id}.
func widgetCacheKey(id pgtype.UUID) string {
	return "jc:widget:" + id.String()
}
