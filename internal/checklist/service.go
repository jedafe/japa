// Package checklist — service.go orchestrates the checklist business logic:
// selector option extraction from published visa guides, item extraction from
// the required_documents field, per-user progress CRUD, and the optional
// completion-points reward (wallet:award_points task, Section 28). It reads
// checklist.* system_settings via the hot-reloadable config.Settings.
//
// Ref: japaconnect-engineering/sections/70-visa-checklist.md
//      japaconnect-engineering/adr/ADR-070-visa-document-checklist-generator.md
package checklist

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db/gen"
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
type ChecklistItem struct {
	ItemKey   string `json:"item_key"`
	Label     string `json:"label"`
	Completed bool   `json:"completed"`
	Notes     string `json:"notes"`
}

// CountryOption is a destination-country selector option derived from
// published visa guides.
type CountryOption struct {
	Country string `json:"country"`
}

// VisaTypeOption is a visa-type selector option. GuideID is the matching
// published visa guide so the frontend can jump straight to its checklist.
type VisaTypeOption struct {
	VisaType string `json:"visa_type"`
	GuideID  string `json:"guide_id"`
}

// SelectorResult is the payload for GET /checklist/selector — the distinct
// destination countries and visa types available across published visa
// guides.
type SelectorResult struct {
	Countries []CountryOption   `json:"countries"`
	VisaTypes []VisaTypeOption  `json:"visa_types"`
}

// ProgressItem is a single persisted progress row, serialized for the API.
type ProgressItem struct {
	ID          string `json:"id"`
	GuideID     string `json:"guide_id"`
	ItemKey     string `json:"item_key"`
	Completed   bool   `json:"completed"`
	CompletedAt string `json:"completed_at"`
	Notes       string `json:"notes"`
	UpdatedAt   string `json:"updated_at"`
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
	GuideID              string `json:"guide_id"`
	TotalUsers           int64  `json:"total_users"`
	UsersWithCompletions int64  `json:"users_with_completions"`
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
	return s.settings.GetBoolOr(ctx, tenantID, "checklist.enabled", true)
}

// GetSelector returns the distinct destination countries and visa types from
// published visa guides. Anonymous-accessible (no auth required).
func (s *Service) GetSelector(ctx context.Context, tenantID pgtype.UUID) (SelectorResult, error) {
	fields, err := s.store.ListPublishedVisaGuideFields(ctx, tenantID, []string{
		FieldDestinationCountry,
		FieldVisaType,
	})
	if err != nil {
		return SelectorResult{}, fmt.Errorf("checklist: selector: %w", err)
	}

	countrySet := make(map[string]struct{})
	visaTypeSet := make(map[string]VisaTypeOption)
	for _, f := range fields {
		switch f.FieldKey {
		case FieldDestinationCountry:
			if v := strings.TrimSpace(f.ValueText); v != "" {
				countrySet[v] = struct{}{}
			}
		case FieldVisaType:
			v := strings.TrimSpace(f.ValueText)
			if v == "" {
				continue
			}
			// Keep the first guide encountered for each visa type so the
			// frontend has a default guide to load.
			if _, ok := visaTypeSet[v]; !ok {
				visaTypeSet[v] = VisaTypeOption{
					VisaType: v,
					GuideID:  f.GuideID.String(),
				}
			}
		}
	}

	countries := make([]CountryOption, 0, len(countrySet))
	for c := range countrySet {
		countries = append(countries, CountryOption{Country: c})
	}
	sort.Slice(countries, func(i, j int) bool { return countries[i].Country < countries[j].Country })

	visaTypes := make([]VisaTypeOption, 0, len(visaTypeSet))
	for _, vt := range visaTypeSet {
		visaTypes = append(visaTypes, vt)
	}
	sort.Slice(visaTypes, func(i, j int) bool { return visaTypes[i].VisaType < visaTypes[j].VisaType })

	return SelectorResult{Countries: countries, VisaTypes: visaTypes}, nil
}

// GetChecklistItems extracts the required_documents field from a visa guide
// and returns the document labels as checklist items with stable item_key
// slugs. Anonymous-accessible (no auth required) — progress is left empty
// for anonymous users.
func (s *Service) GetChecklistItems(ctx context.Context, tenantID, guideID pgtype.UUID) ([]ChecklistItem, error) {
	field, err := s.store.GetVisaGuideField(ctx, tenantID, guideID, FieldRequiredDocuments)
	if err != nil {
		if err == pgx.ErrNoRows {
			return []ChecklistItem{}, nil
		}
		return nil, fmt.Errorf("checklist: get items: %w", err)
	}

	labels := extractDocumentLabels(field)
	items := make([]ChecklistItem, 0, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		items = append(items, ChecklistItem{
			ItemKey: slugifyItemKey(label),
			Label:   label,
		})
	}
	return items, nil
}

// GetProgress returns the user's saved progress for a guide, merged with the
// guide's checklist items so every item is present (unchecked items without
// a progress row still appear).
func (s *Service) GetProgress(ctx context.Context, tenantID, userID, guideID pgtype.UUID) ([]ChecklistItem, error) {
	items, err := s.GetChecklistItems(ctx, tenantID, guideID)
	if err != nil {
		return nil, err
	}

	rows, err := s.store.ListChecklistProgress(ctx, tenantID, userID, guideID)
	if err != nil {
		return nil, fmt.Errorf("checklist: get progress: %w", err)
	}

	progMap := make(map[string]gen.ChecklistProgress, len(rows))
	for _, r := range rows {
		progMap[r.ItemKey] = r
	}

	for i := range items {
		if p, ok := progMap[items[i].ItemKey]; ok {
			items[i].Completed = p.Completed
			if p.Notes != nil {
				items[i].Notes = *p.Notes
			}
		}
	}
	return items, nil
}

// UpsertProgress marks a single item complete/incomplete (with an optional
// note) for the authenticated user. When the item is the final one needed
// for full completion and checklist.completion_points > 0, a
// wallet:award_points task is enqueued fire-and-forget (Rule 8).
func (s *Service) UpsertProgress(ctx context.Context, tenantID, userID, guideID pgtype.UUID, itemKey string, completed bool, notes *string) (gen.ChecklistProgress, error) {
	var completedAt pgtype.Timestamptz
	if completed {
		completedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}

	row, err := s.store.UpsertChecklistProgress(ctx, gen.UpsertChecklistProgressParams{
		TenantID:    tenantID,
		UserID:      userID,
		GuideID:     guideID,
		ItemKey:     itemKey,
		Completed:   completed,
		CompletedAt: completedAt,
		Notes:       notes,
	})
	if err != nil {
		return gen.ChecklistProgress{}, fmt.Errorf("checklist: upsert progress: %w", err)
	}

	// Full-completion reward (Section 28 integration). Only when the item was
	// just marked complete — avoids re-awarding on no-op updates.
	if completed && s.enqueuer != nil && s.settings != nil {
		points := s.settings.GetIntOr(ctx, tenantID, "checklist.completion_points", 0)
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

	if err := s.enqueuer.Enqueue("wallet:award_points", payload); err != nil {
		slog.Warn("checklist: completion award: enqueue failed", "error", err)
		return
	}
	slog.Info("checklist: completion award enqueued",
		"user_id", userID.String(), "guide_id", guideID.String(), "points", points)
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
		TenantID:   tenantID,
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
			GuideID:              r.GuideID.String(),
			TotalUsers:           r.TotalUsers,
			UsersWithCompletions: r.UsersWithCompletions,
		})
	}
	return out, nil
}

// extractDocumentLabels parses the required_documents field value into a list
// of document labels. The field may be stored as a JSON array (multi_select)
// or as a comma/newline-separated text value.
func extractDocumentLabels(field VisaGuideField) []string {
	if len(field.ValueJSON) > 0 {
		var arr []string
		if err := json.Unmarshal(field.ValueJSON, &arr); err == nil && len(arr) > 0 {
			return arr
		}
		// Some multi_select values are stored as a JSON object with an
		// "items" / "values" key — try the common shapes.
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(field.ValueJSON, &obj); err == nil {
			for _, key := range []string{"items", "values", "options"} {
				if raw, ok := obj[key]; ok {
					if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
						return arr
					}
				}
			}
		}
	}
	if field.ValueText != "" {
		parts := strings.FieldsFunc(field.ValueText, func(r rune) bool {
			return r == ',' || r == '\n' || r == ';'
		})
		return parts
	}
	return nil
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
