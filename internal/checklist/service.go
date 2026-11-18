// Package checklist — service.go orchestrates the checklist business logic:
// selector option extraction from published visa guides, item extraction from
// the required_documents field, per-user progress CRUD, and the optional
// completion-points reward (wallet:award_points task, Section 28). It reads
// checklist.* system_settings via the hot-reloadable config.Settings.
// // Ref: japaconnect-engineering/sections/70-visa-checklist.md
// //	japaconnect-engineering/adr/ADR-070-visa-document-checklist-generator.md
package checklist

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
)

// Field keys on the visa guide custom field group (Section 14 amendment).
const (
	FieldDestinationCountry = "destination_country"
	FieldVisaType           = "visa_type"
	FieldRequiredDocuments  = "required_documents"
)

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks. The
// jobqueue enqueuer satisfies this.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// ChecklistItem is a single document requirement rendered as a checkable
// item. ItemKey is a stable slug — taken from the authored `key`/`item_key`
// in the field value when present (ADR-070 §3), otherwise derived from the
// label. Origins optionally scopes the item to origin countries (slugs or
// ISO codes) — empty means "applies to all origins" (ADR-070 §2).
// //nolint:revive // exported: stutter naming is intentional.
type ChecklistItem struct {
	ItemKey     string   `json:"item_key"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Category    string   `json:"category,omitempty"`
	Origins     []string `json:"origins,omitempty"`
	Completed   bool     `json:"completed"`
	Notes       string   `json:"notes"`
}

// ChecklistGuideResult is the payload for GET /checklist/:guide_id — the
// guide's identity plus its checklist items. Matches the frontend
// ChecklistGuide type (§70 contract). DestinationCountry/VisaType let
// deep-linked clients (e.g. visa-routes ?guide_id= links) backfill the
// 3-step selector without a second lookup.
// //nolint:revive // exported: stutter naming is intentional.
type ChecklistGuideResult struct {
	GuideID            string          `json:"guide_id"`
	Title              string          `json:"title"`
	DestinationCountry string          `json:"destination_country,omitempty"`
	VisaType           string          `json:"visa_type,omitempty"`
	Items              []ChecklistItem `json:"items"`
}

// ProgressItem is a single persisted progress row, serialized for the API.
type ProgressItem struct {
	ItemKey     string `json:"item_key"`
	Completed   bool   `json:"completed"`
	CompletedAt string `json:"completed_at,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// ProgressResult is the payload for GET /checklist/:guide_id/progress — the
// user's per-item progress merged with the guide's checklist items. Matches
// the frontend ChecklistProgress type (§70 contract).
type ProgressResult struct {
	GuideID        string         `json:"guide_id"`
	Items          []ProgressItem `json:"items"`
	CompletedCount int            `json:"completed_count"`
	TotalCount     int            `json:"total_count"`
}

// CountryOption is a selector option for either an origin country (Geo Space)
// or a destination country (derived from published visa guides). Destinations
// carry nested visa_types so the frontend can render the 3-step selector
// without a second round-trip.
type CountryOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	// Code is the Geo Space ISO code (origins only) — clients send it back
	// as ?origin_code= so origins lists authored with ISO codes match.
	Code string `json:"code,omitempty"`
	// Visa types available for this destination (destinations only).
	VisaTypes []VisaTypeOption `json:"visa_types,omitempty"`
}

// VisaTypeOption is a visa-type selector option. GuideID is the matching
// published visa guide so the frontend can jump straight to its checklist.
type VisaTypeOption struct {
	// Value is the visa_type custom field value, e.g. "student", "work".
	Value   string `json:"value"`
	Label   string `json:"label"`
	GuideID string `json:"guide_id"`
}

// SelectorResult is the payload for GET /checklist/selector — origin country
// Geo Spaces (§05) plus destination countries with nested visa types from
// published visa guides. Matches the 3-step selector model in §70.
type SelectorResult struct {
	Origins      []CountryOption `json:"origins"`
	Destinations []CountryOption `json:"destinations"`
}

// AdminProgressRow is a single admin-list progress row with the username and
// guide-title joins (guide_title is empty when the backing guide was deleted
// or is no longer a published visa guide — the row is retained for audit).
type AdminProgressRow struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	GuideID     string `json:"guide_id"`
	GuideTitle  string `json:"guide_title"`
	ItemKey     string `json:"item_key"`
	Completed   bool   `json:"completed"`
	CompletedAt string `json:"completed_at"`
	Notes       string `json:"notes"`
	UpdatedAt   string `json:"updated_at"`
	Username    string `json:"username"`
}

// CompletionStat is a per-guide completion aggregate for the admin stats
// route — real completion rates (§70): users fully completing the live item
// set and the average completion percentage across users with progress.
type CompletionStat struct {
	GuideID                string  `json:"guide_id"`
	Title                  string  `json:"title"`
	GuideDeleted           bool    `json:"guide_deleted,omitempty"`
	TotalItems             int     `json:"total_items"`
	TotalUsers             int64   `json:"total_users"`
	UsersWithAnyCompletion int64   `json:"users_with_any_completion"`
	UsersFullyCompleted    int64   `json:"users_fully_completed"`
	AvgCompletionPct       float64 `json:"avg_completion_pct"`
}

// ErrItemNotFound is returned when a PATCH targets an item_key that is not
// part of the guide's live required_documents set (after origin filtering).
var ErrItemNotFound = errors.New("checklist: item key not in guide")

// Service holds the business logic for the visa document checklist.
type Service struct {
	store    *Store
	settings *config.Settings
	enqueuer TaskEnqueuer
	checker  FeatureChecker
	cache    *valkey.Client
	cacheTTL time.Duration
}

// CacheTTLEnv overrides the selector/guide-item cache TTL (Go duration
// string). Defaults to 24h to match the jc:render catalog entry the ADR-070
// §8 cache reuses — keys embed a content version hash, so TTL only bounds
// stale-key memory, never serves stale data.
const CacheTTLEnv = "JC_CHECKLIST_CACHE_TTL"

const defaultCacheTTL = 24 * time.Hour

// NewService creates a checklist service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
		cacheTTL: defaultCacheTTL,
	}
}

// SetEnqueuer injects the Asynq task enqueuer for completion-points rewards.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetCache injects the Valkey client used for the ADR-070 §8 render-cache
// pattern (jc:render::{content_id}:{hash}). Nil disables caching — every read
// falls through to Postgres.
func (s *Service) SetCache(c *valkey.Client) {
	if c == nil {
		return
	}

	s.cache = c

	if env := strings.TrimSpace(os.Getenv(CacheTTLEnv)); env != "" {
		if parsed, err := time.ParseDuration(env); err == nil && parsed > 0 {
			s.cacheTTL = parsed
		} else {
			slog.Warn("invalid JC_CHECKLIST_CACHE_TTL, using default", "value", env, "default", s.cacheTTL)
		}
	}
}

// cacheGet unmarshals a cached payload into out. Returns false on miss,
// malformed payload, or cache outage (all treated identically — the DB is
// the source of truth).
func (s *Service) cacheGet(ctx context.Context, key string, out any) bool {
	if s.cache == nil {
		return false
	}

	raw, err := s.cache.Redis().Get(ctx, key).Result()
	if err != nil {
		return false
	}

	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return false
	}

	return true
}

// cacheSet stores a payload. Best-effort — a Valkey failure never breaks
// the request path.
func (s *Service) cacheSet(ctx context.Context, key string, v any) {
	if s.cache == nil {
		return
	}

	raw, err := json.Marshal(v)
	if err != nil {
		return
	}

	_ = s.cache.Redis().Set(ctx, key, raw, s.cacheTTL).Err()
}

// guideCacheKey produces the ADR-070 §8 render-cache key for a guide's
// checklist payload — jc:render::{guide_id}:{hash(updated_at)}. updated_at
// changes on every guide edit, so edited guides get a fresh key and stale
// entries simply expire.
func guideCacheKey(guideID pgtype.UUID, updatedAt pgtype.Timestamptz) string {
	hash := sha256.Sum256([]byte(strconv.FormatInt(updatedAt.Time.UnixMilli(), 10)))
	return valkey.MustKey("render::%s:%s", guideID.String(), hex.EncodeToString(hash[:]))
}

// selectorCacheKey produces the render-cache key for the selector payload —
// the version hash covers the visa-guide corpus AND the country spaces that
// feed the origin step, so publishing a new guide or renaming a country
// mints a new key.
func selectorCacheKey(version string) string {
	hash := sha256.Sum256([]byte(version))
	return valkey.MustKey("render::checklist-selector:%s", hex.EncodeToString(hash[:]))
}

// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for reachability (Ref: internal/features/features.go).
// system_settings is consulted only as a fallback when no checker is injected.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// SetFeatureChecker injects the feature flag checker so Enabled reads the
// feature_flags table instead of drifting against system_settings.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// Enabled reads the checklist.enabled flag. Prefers the injected
// FeatureChecker; falls back to system_settings for backward compatibility.
func (s *Service) Enabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "checklist.enabled")
	}

	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, "checklist.enabled", true)
}

// GetSelector returns origin country Geo Spaces (§05) and destination
// countries with nested visa types from published visa guides. Follows the
// 3-step selector model in §70: origin → destination → visa type.
// Anonymous-accessible (no auth required). The result is cached under the
// ADR-070 §8 jc:render pattern keyed on a corpus version hash.
func (s *Service) GetSelector(ctx context.Context) (SelectorResult, error) {
	version, err := s.store.SelectorVersion(ctx)
	if err != nil {
		return SelectorResult{}, fmt.Errorf("checklist: selector version: %w", err)
	}

	if s.cache != nil {
		var cached SelectorResult
		if s.cacheGet(ctx, selectorCacheKey(version), &cached) {
			return cached, nil
		}
	}

	// Step 1: Origins — country-level Geo Spaces.
	countrySpaces, err := s.store.ListCountrySpaces(ctx)
	if err != nil {
		return SelectorResult{}, fmt.Errorf("checklist: selector origins: %w", err)
	}

	origins := make([]CountryOption, 0, len(countrySpaces))
	// Index country spaces by slug, lowercased name, and lowercased ISO code
	// so destination options derived from guides' free-text destination_country
	// field resolve to a real Geo Space id (previously emitted as "" —
	// produced duplicate option keys once more than one destination existed).
	spaceByKey := make(map[string]CountrySpace, len(countrySpaces)*3)

	for _, c := range countrySpaces {
		origins = append(origins, CountryOption{
			ID:   c.ID.String(),
			Name: c.Name,
			Slug: c.Slug,
			Code: c.GeoCode,
		})
		spaceByKey[c.Slug] = c
		spaceByKey[strings.ToLower(c.Name)] = c
		if c.GeoCode != "" {
			spaceByKey[strings.ToLower(c.GeoCode)] = c
		}
	}

	// Steps 2+3: Destinations with nested visa types — from published visa
	// guides' destination_country + visa_type custom fields.
	fields, err := s.store.ListPublishedVisaGuideFields(ctx, []string{
		FieldDestinationCountry,
		FieldVisaType,
	})
	if err != nil {
		return SelectorResult{}, fmt.Errorf("checklist: selector destinations: %w", err)
	}

	// Index each guide's destination once (O(n)) — the visa-type pass used to
	// rescan the whole list per row (O(n²) over the fields result).
	destByGuide := make(map[[16]byte]string, len(fields))
	for _, f := range fields {
		if f.FieldKey == FieldDestinationCountry {
			if v := strings.TrimSpace(f.ValueText); v != "" {
				destByGuide[f.GuideID.Bytes] = v
			}
		}
	}

	// Group by destination country (case-insensitive key — free-text guide
	// fields can carry "United Kingdom" and "united kingdom" variants that
	// must merge into one option); each destination gets its visa types.
	destMap := make(map[string]*CountryOption)
	destKeyOf := make(map[string]string) // raw value → normalized key
	destOrder := make([]string, 0)
	guideVisaTypes := make(map[string]map[string]VisaTypeOption) // dest key → visa_type → option

	for _, f := range fields {
		switch f.FieldKey {
		case FieldDestinationCountry:
			v := strings.TrimSpace(f.ValueText)
			if v == "" {
				continue
			}

			key := strings.ToLower(v)
			destKeyOf[v] = key

			if _, ok := destMap[key]; !ok {
				opt := &CountryOption{Name: v, Slug: slugify(v)}
				if sp, found := spaceByKey[opt.Slug]; !found {
					if sp2, found2 := spaceByKey[key]; found2 {
						opt.ID = sp2.ID.String()
					}
				} else {
					opt.ID = sp.ID.String()
				}
				// Fall back to the slug when no Geo Space matches the free-text
				// value so the option still carries a stable non-empty id.
				if opt.ID == "" {
					opt.ID = opt.Slug
				}

				destMap[key] = opt

				destOrder = append(destOrder, key)
				guideVisaTypes[key] = make(map[string]VisaTypeOption)
			}
		case FieldVisaType:
			v := strings.TrimSpace(f.ValueText)
			if v == "" {
				continue
			}

			dest := destKeyOf[destByGuide[f.GuideID.Bytes]]
			if dest == "" {
				continue
			}

			if _, ok := guideVisaTypes[dest]; !ok {
				guideVisaTypes[dest] = make(map[string]VisaTypeOption)
			}

			if _, exists := guideVisaTypes[dest][v]; !exists {
				guideVisaTypes[dest][v] = VisaTypeOption{
					Value:   v,
					Label:   prettifyVisaType(v),
					GuideID: f.GuideID.String(),
				}
			}
		}
	}

	destinations := make([]CountryOption, 0, len(destOrder))

	for _, destName := range destOrder {
		dest := destMap[destName]
		vtMap := guideVisaTypes[destName]
		vts := make([]VisaTypeOption, 0, len(vtMap))

		for _, vt := range vtMap {
			vts = append(vts, vt)
		}

		sort.Slice(vts, func(i, j int) bool { return vts[i].Value < vts[j].Value })
		dest.VisaTypes = vts
		destinations = append(destinations, *dest)
	}

	sort.Slice(destinations, func(i, j int) bool { return destinations[i].Name < destinations[j].Name })

	result := SelectorResult{Origins: origins, Destinations: destinations}
	s.cacheSet(ctx, selectorCacheKey(version), result)

	return result, nil
}

// prettifyVisaType converts a raw visa_type field value (e.g. "student",
// "work") into a human-readable label for the selector.
func prettifyVisaType(v string) string {
	switch v {
	case "student":
		return "Student"
	case "work":
		return "Work"
	case "tourist":
		return "Tourist"
	case "business":
		return "Business"
	case "family":
		return "Family Reunion"
	case "transit":
		return "Transit"
	default:
		// Fallback: replace separators and title-case each word so values
		// like "skilled_worker" render as "Skilled Worker".
		words := strings.FieldsFunc(v, func(r rune) bool { return r == '_' || r == '-' })
		for i, w := range words {
			if len(w) > 0 {
				words[i] = strings.ToUpper(w[:1]) + w[1:]
			}
		}

		return strings.Join(words, " ")
	}
}

// slugify converts a country name into a URL-safe slug (e.g. "United Kingdom"
// → "united-kingdom"). Used for destination options that don't have a Geo
// Space ID.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "'", "")

	return s
}

// GetGuide returns the checklist guide result for GET /checklist/:guide_id.
// Anonymous-accessible. origin/originCode optionally filter items to those
// applicable to that origin country (ADR-070 §2 — origin-specific
// requirements); an empty pair returns the full item set. Returns
// pgx.ErrNoRows when the guide is not a published visa guide so the handler
// can answer 404 instead of 500.
func (s *Service) GetGuide(ctx context.Context, guideID pgtype.UUID, origin, originCode string) (ChecklistGuideResult, error) {
	guide, err := s.getGuideCached(ctx, guideID)
	if err != nil {
		return ChecklistGuideResult{}, err
	}

	guide.Items = filterItemsByOrigin(guide.Items, origin, originCode)

	return guide, nil
}

// getGuideCached resolves a guide's canonical (unfiltered) checklist payload,
// serving from the jc:render cache when possible. The cache stores the full
// item set — origin filtering always happens after retrieval so a cached
// value is never mutated.
func (s *Service) getGuideCached(ctx context.Context, guideID pgtype.UUID) (ChecklistGuideResult, error) {
	title, updatedAt, err := s.store.GetVisaGuideMeta(ctx, guideID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ChecklistGuideResult{}, pgx.ErrNoRows
		}

		return ChecklistGuideResult{}, fmt.Errorf("checklist: get guide title: %w", err)
	}

	if s.cache != nil {
		key := guideCacheKey(guideID, updatedAt)

		var cached ChecklistGuideResult
		if s.cacheGet(ctx, key, &cached) {
			return cached, nil
		}

		guide, err := s.getGuideUncached(ctx, guideID, title)
		if err != nil {
			return ChecklistGuideResult{}, err
		}

		s.cacheSet(ctx, key, guide)

		return guide, nil
	}

	return s.getGuideUncached(ctx, guideID, title)
}

// getGuideUncached builds the checklist payload from the guide's custom
// fields: required_documents (the item list), destination_country and
// visa_type (selector backfill for deep-linked clients).
func (s *Service) getGuideUncached(ctx context.Context, guideID pgtype.UUID, title string) (ChecklistGuideResult, error) {
	fields, err := s.store.GetVisaGuideFields(ctx, guideID, []string{
		FieldRequiredDocuments,
		FieldDestinationCountry,
		FieldVisaType,
	})
	if err != nil {
		return ChecklistGuideResult{}, fmt.Errorf("checklist: get guide fields: %w", err)
	}

	result := ChecklistGuideResult{
		GuideID: guideID.String(),
		Title:   title,
		Items:   []ChecklistItem{},
	}

	for _, f := range fields {
		switch f.FieldKey {
		case FieldRequiredDocuments:
			result.Items = buildChecklistItems(f)
		case FieldDestinationCountry:
			result.DestinationCountry = strings.TrimSpace(f.ValueText)
		case FieldVisaType:
			result.VisaType = strings.TrimSpace(f.ValueText)
		}
	}

	return result, nil
}

// buildChecklistItems converts a required_documents field into ChecklistItem
// values with stable keys. An authored `key`/`item_key` in the field value
// wins (ADR-070 §3 — key generated at authoring time); the label slug is the
// fallback for legacy values. Duplicate resulting keys get a positional -N
// suffix so each item keeps a unique key.
func buildChecklistItems(field VisaGuideField) []ChecklistItem {
	rawItems := extractDocumentItems(field)
	items := make([]ChecklistItem, 0, len(rawItems))

	seen := make(map[string]bool, len(rawItems))
	for _, ri := range rawItems {
		label := strings.TrimSpace(ri.Label)
		if label == "" {
			continue
		}

		base := slugifyItemKey(label)
		if k := strings.TrimSpace(ri.effectiveKey()); k != "" {
			base = slugifyItemKey(k)
		}

		key := base
		for i := 2; seen[key]; i++ {
			key = fmt.Sprintf("%s-%d", base, i)
		}
		seen[key] = true

		items = append(items, ChecklistItem{
			ItemKey:     key,
			Label:       label,
			Description: ri.Description,
			Required:    ri.Required,
			Category:    ri.Category,
			Origins:     ri.effectiveOrigins(),
		})
	}

	return items
}

// filterItemsByOrigin narrows an item list to entries applicable to the
// given origin. Items carry an optional origins list (country slugs or ISO
// codes, ADR-070 §2); empty origins means "all origins". Empty origin input
// returns the full set unchanged — the returned slice may share backing
// storage with the input, callers must not mutate it.
func filterItemsByOrigin(items []ChecklistItem, origin, originCode string) []ChecklistItem {
	oKey := slugify(origin)
	cKey := slugify(originCode)
	if oKey == "" && cKey == "" {
		return items
	}

	out := make([]ChecklistItem, 0, len(items))
	for _, it := range items {
		if len(it.Origins) == 0 || originMatches(it.Origins, oKey, cKey) {
			out = append(out, it)
		}
	}

	return out
}

// originMatches reports whether any origins entry resolves to the requested
// origin slug or ISO code (both compared after slugification).
func originMatches(origins []string, slug, code string) bool {
	for _, o := range origins {
		so := slugify(o)
		if (slug != "" && so == slug) || (code != "" && so == code) {
			return true
		}
	}

	return false
}

// GetProgress returns the user's saved progress for a guide as a
// ProgressResult — per-item completed/completed_at/notes merged with the
// guide's checklist items so every item is present (unchecked items without
// a progress row still appear). origin/originCode apply the same item
// filtering as GetGuide so the progress denominator matches what the client
// displays. Returns pgx.ErrNoRows when the guide is not a published visa
// guide (handler → 404).
func (s *Service) GetProgress(ctx context.Context, userID, guideID pgtype.UUID, origin, originCode string) (ProgressResult, error) {
	guide, err := s.GetGuide(ctx, guideID, origin, originCode)
	if err != nil {
		return ProgressResult{}, err
	}

	rows, err := s.store.ListChecklistProgress(ctx, userID, guideID)
	if err != nil {
		return ProgressResult{}, fmt.Errorf("checklist: get progress: %w", err)
	}

	progMap := make(map[string]gen.ChecklistProgress, len(rows))
	for _, r := range rows {
		progMap[r.ItemKey] = r
	}

	progressItems := make([]ProgressItem, 0, len(guide.Items))
	completedCount := 0

	for _, item := range guide.Items {
		pi := ProgressItem{ItemKey: item.ItemKey}
		if p, ok := progMap[item.ItemKey]; ok {
			pi.Completed = p.Completed
			pi.CompletedAt = formatTs(p.CompletedAt)

			if p.Notes != nil {
				pi.Notes = *p.Notes
			}

			if p.Completed {
				completedCount++
			}
		}

		progressItems = append(progressItems, pi)
	}

	return ProgressResult{
		GuideID:        guideID.String(),
		Items:          progressItems,
		CompletedCount: completedCount,
		TotalCount:     len(guide.Items),
	}, nil
}

// UpsertProgress marks a single item complete/incomplete (with an optional
// note) for the authenticated user. The guide must be a published visa guide
// (pgx.ErrNoRows otherwise) and item_key must belong to its live
// required_documents set (ErrItemNotFound otherwise) — the guide_id FK was
// dropped by migration 000453, so the service is the integrity layer.
// origin/originCode apply the same item filtering the client sees: the
// completion reward counts only items in that set. A nil completed preserves
// the existing completed state (note-only update) by fetching the current
// row first — on the Writer so a same-connection read can't resurrect stale
// state under replica lag; a nil notes preserves the existing notes via SQL
// COALESCE.
func (s *Service) UpsertProgress(ctx context.Context, userID, guideID pgtype.UUID, itemKey string, completed *bool, notes *string, origin, originCode string) (gen.ChecklistProgress, error) {
	guide, err := s.GetGuide(ctx, guideID, origin, originCode)
	if err != nil {
		return gen.ChecklistProgress{}, err
	}

	itemInGuide := false
	for _, it := range guide.Items {
		if it.ItemKey == itemKey {
			itemInGuide = true

			break
		}
	}

	if !itemInGuide {
		return gen.ChecklistProgress{}, ErrItemNotFound
	}

	// Resolve nil completed: a note-only PATCH must preserve the existing
	// completion state AND its timestamp. Fetch the current row on the Writer
	// (read-your-writes); if none exists, default to false (new item, not yet
	// checked).
	resolvedCompleted := false
	var completedAt pgtype.Timestamptz
	if completed != nil {
		resolvedCompleted = *completed
		if resolvedCompleted {
			completedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		}
	} else {
		// Note-only update: keep the original completed_at rather than
		// stamping now() — refreshing it on every note save would rewrite
		// when the user actually finished the item. Only a missing row
		// falls through to defaults — a transient read error must NOT
		// silently write completed=false over an existing tick.
		existing, err := s.store.GetSingleProgressWriter(ctx, userID, guideID, itemKey)
		switch {
		case err == nil:
			resolvedCompleted = existing.Completed
			completedAt = existing.CompletedAt
		case errors.Is(err, pgx.ErrNoRows):
			// First write for this item — defaults stand.
		default:
			return gen.ChecklistProgress{}, fmt.Errorf("checklist: read progress for note update: %w", err)
		}
	}

	row, err := s.store.UpsertChecklistProgress(ctx, gen.UpsertChecklistProgressParams{
		UserID:      userID,
		GuideID:     guideID,
		ItemKey:     itemKey,
		Completed:   resolvedCompleted,
		CompletedAt: completedAt,
		Notes:       notes,
	})
	if err != nil {
		return gen.ChecklistProgress{}, fmt.Errorf("checklist: upsert progress: %w", err)
	}

	// Full-completion reward (Section 28 integration). Only when the item was
	// just marked complete — avoids re-awarding on no-op or note-only updates.
	if completed != nil && *completed && s.enqueuer != nil && s.settings != nil {
		points := s.settings.GetIntOr(ctx, "checklist.completion_points", 0)
		if points > 0 {
			s.maybeAwardCompletionPoints(ctx, userID, guideID, guide.Items, points)
		}
	}

	return row, nil
}

// maybeAwardCompletionPoints checks whether all live checklist items are now
// completed and, if so, enqueues a wallet:award_points task. Completed rows
// are counted on the Writer (the upsert just landed there) and only against
// keys present in the current item set — orphaned or never-valid keys can't
// inflate the count toward the threshold. Fire-and-forget per Rule 8 —
// failures are logged, never returned to the caller.
func (s *Service) maybeAwardCompletionPoints(ctx context.Context, userID, guideID pgtype.UUID, items []ChecklistItem, points int) {
	if len(items) == 0 {
		return
	}

	liveKeys := make(map[string]bool, len(items))
	for _, it := range items {
		liveKeys[it.ItemKey] = true
	}

	rows, err := s.store.ListChecklistProgressWriter(ctx, userID, guideID)
	if err != nil {
		slog.Warn("checklist: completion check: list progress", "error", err)
		return
	}

	completed := 0
	for _, r := range rows {
		if r.Completed && liveKeys[r.ItemKey] {
			completed++
		}
	}

	if completed < len(items) {
		return
	}

	payload, err := json.Marshal(map[string]any{
		"user_id":  userID.String(),
		"guide_id": guideID.String(),
		"points":   points,
		"reason":   "checklist_completion",
	})
	if err != nil {
		slog.Warn("checklist: completion award: marshal payload", "error", err)
		return
	}

	if err := s.enqueuer.Enqueue(worker.TypeWalletAwardPoints, payload); err != nil {
		slog.Warn("checklist: completion award: enqueue failed", "error", err)
		return
	}

	slog.Info("checklist: completion award enqueued", "user_id", userID.String(), "guide_id", guideID.String(), "points", points)
}

// ResetProgress deletes all progress rows for a user + guide. The guide must
// be a published visa guide (pgx.ErrNoRows otherwise → handler 404).
func (s *Service) ResetProgress(ctx context.Context, userID, guideID pgtype.UUID) error {
	if _, err := s.getGuideCached(ctx, guideID); err != nil {
		return err
	}

	if err := s.store.DeleteChecklistProgress(ctx, userID, guideID); err != nil {
		return fmt.Errorf("checklist: reset progress: %w", err)
	}

	return nil
}

// AdminListProgress lists all users' progress rows (paginated, filterable by
// guide_id and user_id, free-text q across username/email/item key/guide
// title, sortable). Admin-only (checklist.manage permission).
func (s *Service) AdminListProgress(ctx context.Context, guideID, userID pgtype.UUID, q, sortCol, sortDir string, page, limit int32) ([]AdminProgressRow, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, err := s.store.AdminListProgressFiltered(ctx, AdminProgressParams{
		GuideID:    guideID,
		UserID:     userID,
		Q:          q,
		Sort:       sortCol,
		Dir:        sortDir,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("checklist: admin list progress: %w", err)
	}

	total, err := s.store.AdminCountProgressFiltered(ctx, guideID, userID, q)
	if err != nil {
		return nil, 0, fmt.Errorf("checklist: admin count progress: %w", err)
	}

	out := make([]AdminProgressRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, AdminProgressRow{
			ID:          r.ID.String(),
			UserID:      r.UserID.String(),
			GuideID:     r.GuideID.String(),
			GuideTitle:  r.GuideTitle,
			ItemKey:     r.ItemKey,
			Completed:   r.Completed,
			CompletedAt: formatTs(r.CompletedAt),
			Notes:       ptrString(r.Notes),
			UpdatedAt:   formatTs(r.UpdatedAt),
			Username:    r.Username,
		})
	}

	return out, total, nil
}

// AdminCompletionStats returns per-guide completion aggregates (admin):
// total users with progress, users with any completion, users who completed
// every live item, and the average completion percentage — the "completion
// rates by guide" promised by the §70 route. Guides are paginated; per-user
// completion counts are computed only for the requested page.
func (s *Service) AdminCompletionStats(ctx context.Context, page, limit int32) ([]CompletionStat, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	guideIDs, total, err := s.store.AdminProgressGuideIDs(ctx, offset, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("checklist: admin stats guide ids: %w", err)
	}

	if len(guideIDs) == 0 {
		return []CompletionStat{}, total, nil
	}

	pairs, err := s.store.AdminProgressPairs(ctx, guideIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("checklist: admin stats pairs: %w", err)
	}

	titles, err := s.store.GuideTitles(ctx, guideIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("checklist: admin stats titles: %w", err)
	}

	type agg struct {
		gid      pgtype.UUID
		stat     *CompletionStat
		userDone [][]string
	}

	statsByGuide := make(map[pgtype.UUID]*agg, len(guideIDs))
	ordered := make([]*agg, 0, len(guideIDs))

	for _, gid := range guideIDs {
		t, found := titles[gid]
		a := &agg{gid: gid, stat: &CompletionStat{
			GuideID:      gid.String(),
			Title:        t.Title,
			GuideDeleted: !found || t.Deleted,
		}}
		statsByGuide[gid] = a
		ordered = append(ordered, a)
	}

	for _, p := range pairs {
		a, ok := statsByGuide[p.GuideID]
		if !ok {
			continue
		}

		a.stat.TotalUsers++
		a.userDone = append(a.userDone, p.DoneKeys)
	}

	// Live item count per guide — the denominator for full-completion and
	// average-percentage math, intersected with each user's completed keys
	// so stale/orphaned keys can't inflate the rate. Item sets come from the
	// guide cache (canonical, unfiltered).
	for _, a := range ordered {
		guide, gerr := s.getGuideCached(ctx, a.gid)
		if gerr != nil {
			// Guide no longer published/exists — rows retained, rates unknown.
			a.stat.GuideDeleted = true
			continue
		}

		a.stat.TotalItems = len(guide.Items)
		if a.stat.TotalItems == 0 {
			continue
		}

		liveKeys := make(map[string]bool, len(guide.Items))
		for _, it := range guide.Items {
			liveKeys[it.ItemKey] = true
		}

		var pctSum float64
		for _, doneKeys := range a.userDone {
			liveDone := int64(0)
			for _, k := range doneKeys {
				if liveKeys[k] {
					liveDone++
				}
			}

			// "Any completion" is measured against the live set like the
			// rates — a user whose only ticks were on since-removed items
			// has effectively zero completion.
			if liveDone > 0 {
				a.stat.UsersWithAnyCompletion++
			}

			if liveDone >= int64(a.stat.TotalItems) {
				a.stat.UsersFullyCompleted++
			}

			pctSum += float64(liveDone) / float64(a.stat.TotalItems)
		}

		if a.stat.TotalUsers > 0 {
			a.stat.AvgCompletionPct = pctSum / float64(a.stat.TotalUsers) * 100
		}
	}

	out := make([]CompletionStat, 0, len(ordered))
	for _, a := range ordered {
		out = append(out, *a.stat)
	}

	return out, total, nil
}

// rawDocumentItem is the intermediate representation of a single document
// requirement parsed from the required_documents field. Only Label is
// required; the other fields are optional metadata. Key/ItemKey carry the
// authored stable key (ADR-070 §3); Origins/OriginCountries scope the item
// to specific origin countries (ADR-070 §2) — slugs or ISO codes.
type rawDocumentItem struct {
	Label           string   `json:"label"`
	Description     string   `json:"description"`
	Required        bool     `json:"required"`
	Category        string   `json:"category"`
	Key             string   `json:"key"`
	ItemKey         string   `json:"item_key"`
	Origins         []string `json:"origins"`
	OriginCountries []string `json:"origin_countries"`
}

// effectiveKey returns the authored stable key when present (key, then
// item_key aliases), else "" — the caller falls back to the label slug.
func (r rawDocumentItem) effectiveKey() string {
	if r.Key != "" {
		return r.Key
	}

	return r.ItemKey
}

// effectiveOrigins merges the two accepted origins aliases into one list.
func (r rawDocumentItem) effectiveOrigins() []string {
	out := make([]string, 0, len(r.Origins)+len(r.OriginCountries))
	for _, o := range r.Origins {
		if s := strings.TrimSpace(o); s != "" {
			out = append(out, s)
		}
	}
	for _, o := range r.OriginCountries {
		if s := strings.TrimSpace(o); s != "" {
			out = append(out, s)
		}
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

// extractDocumentItems parses the required_documents field value into a list
// of rawDocumentItems. The field supports three storage formats:
// 1. JSON array of objects: [{"label":"Passport","required":true,"category":"Identity"}, ...]
// 2. JSON array of strings: ["Bank Statement","Passport Copy", ...] (metadata defaults)
// 3. Comma/newline/semicolon-separated text: "Bank Statement, Passport Copy"
// // Objects with an "items"/"values"/"options" key wrapping an array are also
// handled (some multi_select custom fields use this shape).
func extractDocumentItems(field VisaGuideField) []rawDocumentItem {
	if len(field.ValueJSON) > 0 {
		// Try array of objects first (richest format).
		var objArr []rawDocumentItem
		if err := json.Unmarshal(field.ValueJSON, &objArr); err == nil && len(objArr) > 0 {
			return objArr
		}
		// Try array of strings (plain labels).
		var strArr []string
		if err := json.Unmarshal(field.ValueJSON, &strArr); err == nil && len(strArr) > 0 {
			return labelsToRawItems(strArr)
		}
		// Try object with items/values/options key.
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(field.ValueJSON, &obj); err == nil {
			for _, key := range []string{"items", "values", "options"} {
				if raw, ok := obj[key]; ok {
					if err := json.Unmarshal(raw, &objArr); err == nil && len(objArr) > 0 {
						return objArr
					}

					if err := json.Unmarshal(raw, &strArr); err == nil && len(strArr) > 0 {
						return labelsToRawItems(strArr)
					}
				}
			}
		}
	}

	if field.ValueText != "" {
		parts := strings.FieldsFunc(field.ValueText, func(r rune) bool {
			return r == ',' || r == '\n' || r == ';'
		})

		return labelsToRawItems(parts)
	}

	return nil
}

// labelsToRawItems converts a slice of plain label strings into rawDocumentItems
// with default metadata (not required, no category, no description).
func labelsToRawItems(labels []string) []rawDocumentItem {
	out := make([]rawDocumentItem, 0, len(labels))
	for _, l := range labels {
		out = append(out, rawDocumentItem{Label: l})
	}

	return out
}

// slugifyItemKey converts a document label to a stable lowercase slug:
// lowercase, hyphens, strip non-alphanumeric. e.g. "Bank Statement" →
// "bank-statement", "Passport Copy" → "passport-copy".
func slugifyItemKey(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))

	var sb strings.Builder

	prevDash := true

	for _, r := range label {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)

			prevDash = false
		} else if !prevDash {
			sb.WriteRune('-')

			prevDash = true
		}
	}

	out := strings.Trim(sb.String(), "-")
	if out == "" {
		return "item"
	}

	return out
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}

	return t.Time.UTC().Format(time.RFC3339)
}

// ptrString dereferences a *string, returning "" for nil.
func ptrString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}
