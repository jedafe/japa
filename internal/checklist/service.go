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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
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
// item. ItemKey is a stable slug derived from the label so that rewording a
// guide's document label does not orphan saved progress.
// //nolint:revive // exported: stutter naming is intentional
type ChecklistItem struct {
	ItemKey     string `json:"item_key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Category    string `json:"category,omitempty"`
	Completed   bool   `json:"completed"`
	Notes       string `json:"notes"`
}

// ChecklistGuideResult is the payload for GET /checklist/:guide_id — the
// guide's identity plus its checklist items. Matches the frontend
// ChecklistGuide type (§70 contract).
// //nolint:revive // exported: stutter naming is intentional
type ChecklistGuideResult struct {
	GuideID string          `json:"guide_id"`
	Title   string          `json:"title"`
	Items   []ChecklistItem `json:"items"`
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

// AdminProgressRow is a single admin-list progress row with the username join.
type AdminProgressRow struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	GuideID     string `json:"guide_id"`
	ItemKey     string `json:"item_key"`
	Completed   bool   `json:"completed"`
	CompletedAt string `json:"completed_at"`
	Notes       string `json:"notes"`
	UpdatedAt   string `json:"updated_at"`
	Username    string `json:"username"`
}

// CompletionStat is a per-guide completion aggregate for the admin stats route.
type CompletionStat struct {
	GuideID                string `json:"guide_id"`
	TotalUsers             int64  `json:"total_users"`
	UsersWithAnyCompletion int64  `json:"users_with_any_completion"`
}

// Service holds the business logic for the visa document checklist.
type Service struct {
	store    *Store
	settings *config.Settings
	enqueuer TaskEnqueuer
}

// NewService creates a checklist service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// SetEnqueuer injects the Asynq task enqueuer for completion-points rewards.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// Enabled reads the checklist.enabled system_settings toggle.
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, pgtype.UUID{}, "checklist.enabled", true)
}

// GetSelector returns origin country Geo Spaces (§05) and destination
// countries with nested visa types from published visa guides. Follows the
// 3-step selector model in §70: origin → destination → visa type.
// Anonymous-accessible (no auth required).
func (s *Service) GetSelector(ctx context.Context, tenantID pgtype.UUID) (SelectorResult, error) {
	// Step 1: Origins — country-level Geo Spaces.
	countrySpaces, err := s.store.ListCountrySpaces(ctx, tenantID)
	if err != nil {
		return SelectorResult{}, fmt.Errorf("checklist: selector origins: %w", err)
	}

	origins := make([]CountryOption, 0, len(countrySpaces))
	for _, c := range countrySpaces {
		origins = append(origins, CountryOption{
			ID:   c.ID.String(),
			Name: c.Name,
			Slug: c.Slug,
		})
	}

	// Steps 2+3: Destinations with nested visa types — from published visa
	// guides' destination_country + visa_type custom fields.
	fields, err := s.store.ListPublishedVisaGuideFields(ctx, tenantID, []string{
		FieldDestinationCountry,
		FieldVisaType,
	})
	if err != nil {
		return SelectorResult{}, fmt.Errorf("checklist: selector destinations: %w", err)
	}

	// Group by destination country; each destination gets its visa types.
	destMap := make(map[string]*CountryOption) // key = destination country name
	destOrder := make([]string, 0)
	guideVisaTypes := make(map[string]map[string]VisaTypeOption) // dest → visa_type → option

	for _, f := range fields {
		switch f.FieldKey {
		case FieldDestinationCountry:
			v := strings.TrimSpace(f.ValueText)
			if v == "" {
				continue
			}

			if _, ok := destMap[v]; !ok {
				destMap[v] = &CountryOption{Name: v, Slug: slugify(v)}
				destOrder = append(destOrder, v)
				guideVisaTypes[v] = make(map[string]VisaTypeOption)
			}
		case FieldVisaType:
			v := strings.TrimSpace(f.ValueText)
			if v == "" {
				continue
			}
			// Find the destination this guide belongs to by scanning the
			// fields list for the same guide_id's destination_country.
			dest := findGuideDestination(fields, f.GuideID)
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

	return SelectorResult{Origins: origins, Destinations: destinations}, nil
}

// findGuideDestination scans the flat field list for the destination_country
// value associated with the given guide_id.
func findGuideDestination(fields []VisaGuideField, guideID pgtype.UUID) string {
	for _, f := range fields {
		if f.FieldKey == FieldDestinationCountry && f.GuideID == guideID {
			return strings.TrimSpace(f.ValueText)
		}
	}

	return ""
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
		// Title-case the first letter as a fallback.
		if len(v) > 0 {
			return strings.ToUpper(v[:1]) + v[1:]
		}

		return v
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

// GetGuide returns the checklist guide result (guide_id + title + items) for
// GET /checklist/:guide_id. Anonymous-accessible (no auth required).
func (s *Service) GetGuide(ctx context.Context, tenantID, guideID pgtype.UUID) (ChecklistGuideResult, error) {
	title, err := s.store.GetVisaGuideTitle(ctx, tenantID, guideID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ChecklistGuideResult{}, pgx.ErrNoRows
		}

		return ChecklistGuideResult{}, fmt.Errorf("checklist: get guide title: %w", err)
	}

	items, err := s.GetChecklistItems(ctx, tenantID, guideID)
	if err != nil {
		return ChecklistGuideResult{}, err
	}

	return ChecklistGuideResult{
		GuideID: guideID.String(),
		Title:   title,
		Items:   items,
	}, nil
}

// GetChecklistItems extracts the required_documents field from a visa guide
// and returns the document items with stable item_key slugs plus optional
// required/category/description metadata. Anonymous-accessible (no auth
// required) — progress is left empty for anonymous users.
func (s *Service) GetChecklistItems(ctx context.Context, tenantID, guideID pgtype.UUID) ([]ChecklistItem, error) {
	field, err := s.store.GetVisaGuideField(ctx, tenantID, guideID, FieldRequiredDocuments)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []ChecklistItem{}, nil
		}

		return nil, fmt.Errorf("checklist: get items: %w", err)
	}

	rawItems := extractDocumentItems(field)
	items := make([]ChecklistItem, 0, len(rawItems))

	for _, ri := range rawItems {
		label := strings.TrimSpace(ri.Label)
		if label == "" {
			continue
		}

		items = append(items, ChecklistItem{
			ItemKey:     slugifyItemKey(label),
			Label:       label,
			Description: ri.Description,
			Required:    ri.Required,
			Category:    ri.Category,
		})
	}

	return items, nil
}

// GetProgress returns the user's saved progress for a guide as a
// ProgressResult — per-item completed/completed_at/notes merged with the
// guide's checklist items so every item is present (unchecked items without
// a progress row still appear). Matches the frontend ChecklistProgress type.
func (s *Service) GetProgress(ctx context.Context, tenantID, userID, guideID pgtype.UUID) (ProgressResult, error) {
	items, err := s.GetChecklistItems(ctx, tenantID, guideID)
	if err != nil {
		return ProgressResult{}, err
	}

	rows, err := s.store.ListChecklistProgress(ctx, tenantID, userID, guideID)
	if err != nil {
		return ProgressResult{}, fmt.Errorf("checklist: get progress: %w", err)
	}

	progMap := make(map[string]gen.ChecklistProgress, len(rows))
	for _, r := range rows {
		progMap[r.ItemKey] = r
	}

	progressItems := make([]ProgressItem, 0, len(items))
	completedCount := 0

	for _, item := range items {
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
		TotalCount:     len(items),
	}, nil
}

// UpsertProgress marks a single item complete/incomplete (with an optional
// note) for the authenticated user. A nil completed preserves the existing
// completed state (note-only update) by fetching the current row first; a
// nil notes preserves the existing notes via SQL COALESCE. When the item is
// the final one needed for full completion and checklist.completion_points >
// 0, a wallet:award_points task is enqueued fire-and-forget (Rule 8).
func (s *Service) UpsertProgress(ctx context.Context, tenantID, userID, guideID pgtype.UUID, itemKey string, completed *bool, notes *string) (gen.ChecklistProgress, error) {
	// Resolve nil completed: a note-only PATCH must preserve the existing
	// completion state. Fetch the current row; if none exists, default to
	// false (new item, not yet checked).
	resolvedCompleted := false
	if completed != nil {
		resolvedCompleted = *completed
	} else {
		existing, err := s.store.GetSingleProgress(ctx, tenantID, userID, guideID, itemKey)
		if err == nil {
			resolvedCompleted = existing.Completed
		}
	}

	var completedAt pgtype.Timestamptz
	if resolvedCompleted {
		completedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
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
		points := s.settings.GetIntOr(ctx, pgtype.UUID{}, "checklist.completion_points", 0)
		if points > 0 {
			s.maybeAwardCompletionPoints(ctx, tenantID, userID, guideID, points)
		}
	}

	return row, nil
}

// maybeAwardCompletionPoints checks whether all checklist items are now
// completed and, if so, enqueues a wallet:award_points task. Fire-and-forget
// per Rule 8 — failures are logged, never returned to the caller.
func (s *Service) maybeAwardCompletionPoints(ctx context.Context, tenantID, userID, guideID pgtype.UUID, points int) {
	items, err := s.GetChecklistItems(ctx, tenantID, guideID)
	if err != nil {
		slog.Warn("checklist: completion check: list items", "error", err)
		return
	}

	if len(items) == 0 {
		return
	}

	completed, err := s.store.CountCompletedChecklistItems(ctx, tenantID, userID, guideID)
	if err != nil {
		slog.Warn("checklist: completion check: count", "error", err)
		return
	}

	if completed < int64(len(items)) {
		return
	}

	payload, err := json.Marshal(map[string]any{
		"tenant_id": tenantID.String(),
		"user_id":   userID.String(),
		"guide_id":  guideID.String(),
		"points":    points,
		"reason":    "checklist_completion",
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

// ResetProgress deletes all progress rows for a user + guide.
func (s *Service) ResetProgress(ctx context.Context, tenantID, userID, guideID pgtype.UUID) error {
	if err := s.store.DeleteChecklistProgress(ctx, tenantID, userID, guideID); err != nil {
		return fmt.Errorf("checklist: reset progress: %w", err)
	}

	return nil
}

// AdminListProgress lists all users' progress rows (paginated, filterable by
// guide_id and user_id). Admin-only (checklist.manage permission).
func (s *Service) AdminListProgress(ctx context.Context, tenantID, guideID, userID pgtype.UUID, page, limit int32) ([]AdminProgressRow, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, err := s.store.AdminListChecklistProgress(ctx, gen.AdminListChecklistProgressParams{

		GuideID:    guideID,
		UserID:     userID,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("checklist: admin list progress: %w", err)
	}

	total, err := s.store.AdminCountChecklistProgress(ctx, tenantID, guideID, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("checklist: admin count progress: %w", err)
	}

	out := make([]AdminProgressRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, AdminProgressRow{
			ID:          r.ID.String(),
			UserID:      r.UserID.String(),
			GuideID:     r.GuideID.String(),
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

// AdminCompletionStats returns per-guide completion aggregates (admin).
func (s *Service) AdminCompletionStats(ctx context.Context, tenantID pgtype.UUID, page, limit int32) ([]CompletionStat, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, err := s.store.AdminCompletionStats(ctx, tenantID, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("checklist: admin completion stats: %w", err)
	}

	out := make([]CompletionStat, 0, len(rows))
	for _, r := range rows {
		out = append(out, CompletionStat{
			GuideID:                r.GuideID.String(),
			TotalUsers:             r.TotalUsers,
			UsersWithAnyCompletion: r.UsersWithAnyCompletion,
		})
	}

	return out, nil
}

// rawDocumentItem is the intermediate representation of a single document
// requirement parsed from the required_documents field. Only Label is
// required; the other fields are optional metadata.
type rawDocumentItem struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Category    string `json:"category"`
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
